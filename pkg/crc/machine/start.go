package machine

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/code-ready/crc/pkg/crc/cluster"
	"github.com/code-ready/crc/pkg/crc/constants"
	crcerrors "github.com/code-ready/crc/pkg/crc/errors"
	"github.com/code-ready/crc/pkg/crc/logging"
	"github.com/code-ready/crc/pkg/crc/machine/bundle"
	"github.com/code-ready/crc/pkg/crc/machine/config"
	"github.com/code-ready/crc/pkg/crc/network"
	"github.com/code-ready/crc/pkg/crc/oc"
	"github.com/code-ready/crc/pkg/crc/services"
	"github.com/code-ready/crc/pkg/crc/services/dns"
	"github.com/code-ready/crc/pkg/crc/ssh"
	crcssh "github.com/code-ready/crc/pkg/crc/ssh"
	"github.com/code-ready/crc/pkg/crc/systemd"
	"github.com/code-ready/crc/pkg/libmachine"
	"github.com/code-ready/crc/pkg/libmachine/host"
	crcos "github.com/code-ready/crc/pkg/os"
	"github.com/code-ready/machine/libmachine/drivers"
	"github.com/code-ready/machine/libmachine/state"
	"github.com/docker/go-units"
	"github.com/pkg/errors"
)

const minimumMemoryForMonitoring = 14336

func getCrcBundleInfo(bundlePath string) (*bundle.CrcBundleInfo, error) {
	bundleName := filepath.Base(bundlePath)
	bundleInfo, err := bundle.GetCachedBundleInfo(bundleName)
	if err == nil {
		logging.Infof("Loading bundle: %s ...", bundleName)
		return bundleInfo, nil
	}
	logging.Debugf("Failed to load bundle %s: %v", bundleName, err)
	logging.Infof("Extracting bundle: %s ...", bundleName)
	return bundle.Extract(bundlePath)
}

func (client *client) updateVMConfig(startConfig StartConfig, api libmachine.API, host *host.Host) error {
	/* Memory */
	logging.Debugf("Updating CRC VM configuration")
	if err := setMemory(host, startConfig.Memory); err != nil {
		logging.Debugf("Failed to update CRC VM configuration: %v", err)
		if err == drivers.ErrNotImplemented {
			logging.Warn("Memory configuration change has been ignored as the machine driver does not support it")
		} else {
			return err
		}
	}
	if err := setVcpus(host, startConfig.CPUs); err != nil {
		logging.Debugf("Failed to update CRC VM configuration: %v", err)
		if err == drivers.ErrNotImplemented {
			logging.Warn("CPU configuration change has been ignored as the machine driver does not support it")
		} else {
			return err
		}
	}
	if err := api.Save(host); err != nil {
		return err
	}

	/* Disk size */
	if startConfig.DiskSize != constants.DefaultDiskSize {
		if err := setDiskSize(host, startConfig.DiskSize); err != nil {
			logging.Debugf("Failed to update CRC disk configuration: %v", err)
			if err == drivers.ErrNotImplemented {
				logging.Warn("Disk size configuration change has been ignored as the machine driver does not support it")
			} else {
				return err
			}
		}
		if err := api.Save(host); err != nil {
			return err
		}
	}

	return nil
}

func (client *client) Start(startConfig StartConfig) (*StartResult, error) {
	if err := client.validateStartConfig(startConfig); err != nil {
		return nil, err
	}

	var crcBundleMetadata *bundle.CrcBundleInfo

	libMachineAPIClient, cleanup := createLibMachineClient()
	defer cleanup()

	// Pre-VM start
	var host *host.Host
	exists, err := client.Exists()
	if err != nil {
		return nil, errors.Wrap(err, "Cannot determine if VM exists")
	}
	if !exists {
		// Ask early for pull secret if it hasn't been requested yet
		_, err = startConfig.PullSecret.Value()
		if err != nil {
			return nil, errors.Wrap(err, "Failed to ask for pull secret")
		}

		machineConfig := config.MachineConfig{
			Name:        client.name,
			BundleName:  filepath.Base(startConfig.BundlePath),
			CPUs:        startConfig.CPUs,
			Memory:      startConfig.Memory,
			DiskSize:    startConfig.DiskSize,
			NetworkMode: client.networkMode(),
		}

		crcBundleMetadata, err = getCrcBundleInfo(startConfig.BundlePath)
		if err != nil {
			return nil, errors.Wrap(err, "Error getting bundle metadata")
		}

		logging.Infof("Verifying bundle %s ...", filepath.Base(startConfig.BundlePath))
		if err := crcBundleMetadata.Verify(); err != nil {
			return nil, errors.Wrapf(err, "Invalid bundle %s", filepath.Base(startConfig.BundlePath))
		}

		logging.Infof("Creating CodeReady Containers VM for OpenShift %s...", crcBundleMetadata.GetOpenshiftVersion())

		// Retrieve metadata info
		machineConfig.ImageSourcePath = crcBundleMetadata.GetDiskImagePath()
		machineConfig.ImageFormat = crcBundleMetadata.Storage.DiskImages[0].Format
		machineConfig.SSHKeyPath = crcBundleMetadata.GetSSHKeyPath()
		machineConfig.KernelCmdLine = crcBundleMetadata.Nodes[0].KernelCmdLine
		machineConfig.Initramfs = crcBundleMetadata.GetInitramfsPath()
		machineConfig.Kernel = crcBundleMetadata.GetKernelPath()

		host, err = createHost(libMachineAPIClient, machineConfig)
		if err != nil {
			return nil, errors.Wrap(err, "Error creating machine")
		}
	} else {
		host, err = libMachineAPIClient.Load(client.name)
		if err != nil {
			return nil, errors.Wrap(err, "Error loading machine")
		}

		var bundleName string
		bundleName, crcBundleMetadata, err = getBundleMetadataFromDriver(host.Driver)
		if err != nil {
			return nil, errors.Wrap(err, "Error loading bundle metadata")
		}
		if bundleName != filepath.Base(startConfig.BundlePath) {
			logging.Debugf("Bundle '%s' was requested, but the existing VM is using '%s'",
				filepath.Base(startConfig.BundlePath), bundleName)
			return nil, fmt.Errorf("Bundle '%s' was requested, but the existing VM is using '%s'",
				filepath.Base(startConfig.BundlePath),
				bundleName)
		}
		vmState, err := host.Driver.GetState()
		if err != nil {
			return nil, errors.Wrap(err, "Error getting the machine state")
		}
		if vmState == state.Running {
			logging.Infof("A CodeReady Containers VM for OpenShift %s is already running", crcBundleMetadata.GetOpenshiftVersion())
			clusterConfig, err := getClusterConfig(crcBundleMetadata)
			if err != nil {
				return nil, errors.Wrap(err, "Cannot create cluster configuration")
			}
			return &StartResult{
				Status:         vmState,
				ClusterConfig:  *clusterConfig,
				KubeletStarted: true,
			}, nil
		}

		logging.Infof("Starting CodeReady Containers VM for OpenShift %s...", crcBundleMetadata.GetOpenshiftVersion())

		if err := client.updateVMConfig(startConfig, libMachineAPIClient, host); err != nil {
			return nil, errors.Wrap(err, "Could not update CRC VM configuration")
		}

		if err := host.Driver.Start(); err != nil {
			return nil, errors.Wrap(err, "Error starting stopped VM")
		}
	}

	if runtime.GOOS == "darwin" && client.useVSock() {
		if err := makeDaemonVisibleToHyperkit(client.name); err != nil {
			return nil, err
		}
	}

	clusterConfig, err := getClusterConfig(crcBundleMetadata)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot create cluster configuration")
	}

	// Post-VM start
	vmState, err := host.Driver.GetState()
	if err != nil {
		return nil, errors.Wrap(err, "Error getting the state")
	}
	if vmState != state.Running {
		return nil, errors.Wrap(err, "CodeReady Containers VM is not running")
	}

	instanceIP, err := getIP(host, client.useVSock())
	if err != nil {
		return nil, errors.Wrap(err, "Error getting the IP")
	}
	sshRunner, err := crcssh.CreateRunner(instanceIP, getSSHPort(client.useVSock()), crcBundleMetadata.GetSSHKeyPath(), constants.GetPrivateKeyPath(), constants.GetRsaPrivateKeyPath())
	if err != nil {
		return nil, errors.Wrap(err, "Error creating the ssh client")
	}
	defer sshRunner.Close()

	logging.Debug("Waiting until ssh is available")
	if err := cluster.WaitForSSH(sshRunner); err != nil {
		return nil, errors.Wrap(err, "Failed to connect to the CRC VM with SSH -- host might be unreachable")
	}
	logging.Info("CodeReady Containers VM is running")

	// Post VM start immediately update SSH key and copy kubeconfig to instance
	// dir and VM
	if err := updateSSHKeyAndCopyKubeconfig(sshRunner, client.name, crcBundleMetadata); err != nil {
		return nil, errors.Wrap(err, "Error updating public key")
	}

	// Trigger disk resize, this will be a no-op if no disk size change is needed
	if _, _, err = sshRunner.Run("sudo xfs_growfs / >/dev/null"); err != nil {
		return nil, errors.Wrap(err, "Error updating filesystem size")
	}

	// Start network time synchronization if `CRC_DEBUG_ENABLE_STOP_NTP` is not set
	if stopNtp, _ := strconv.ParseBool(os.Getenv("CRC_DEBUG_ENABLE_STOP_NTP")); !stopNtp {
		logging.Info("Starting network time synchronization in CodeReady Containers VM")
		if _, _, err := sshRunner.Run("sudo timedatectl set-ntp on"); err != nil {
			return nil, errors.Wrap(err, "Failed to start network time synchronization")
		}
	}

	// Add nameserver to VM if provided by User
	if startConfig.NameServer != "" {
		if err = addNameServerToInstance(sshRunner, startConfig.NameServer); err != nil {
			return nil, errors.Wrap(err, "Failed to add nameserver to the VM")
		}
	}

	proxyConfig, err := getProxyConfig(crcBundleMetadata.ClusterInfo.BaseDomain)
	if err != nil {
		return nil, errors.Wrap(err, "Error getting proxy configuration")
	}
	proxyConfig.ApplyToEnvironment()
	proxyConfig.AddNoProxy(instanceIP)

	// Create servicePostStartConfig for DNS checks and DNS start.
	servicePostStartConfig := services.ServicePostStartConfig{
		Name: client.name,
		// TODO: would prefer passing in a more generic type
		SSHRunner: sshRunner,
		IP:        instanceIP,
		// TODO: should be more finegrained
		BundleMetadata: *crcBundleMetadata,
		NetworkMode:    client.networkMode(),
	}

	// Run the DNS server inside the VM
	if err := dns.RunPostStart(servicePostStartConfig); err != nil {
		return nil, errors.Wrap(err, "Error running post start")
	}

	// Check DNS lookup before starting the kubelet
	if queryOutput, err := dns.CheckCRCLocalDNSReachable(servicePostStartConfig); err != nil {
		if !client.useVSock() {
			return nil, errors.Wrapf(err, "Failed internal DNS query: %s", queryOutput)
		}
		logging.Warn(fmt.Sprintf("Failed internal DNS query: %s: %v", queryOutput, err))
	}
	logging.Info("Check internal and public DNS query ...")

	if queryOutput, err := dns.CheckCRCPublicDNSReachable(servicePostStartConfig); err != nil {
		logging.Warnf("Failed public DNS query from the cluster: %v : %s", err, queryOutput)
	}

	// Check DNS lookup from host to VM
	logging.Info("Check DNS query from host ...")
	if err := network.CheckCRCLocalDNSReachableFromHost(crcBundleMetadata, instanceIP); err != nil {
		if !client.useVSock() {
			return nil, errors.Wrap(err, "Failed to query DNS from host")
		}
		logging.Warn(fmt.Sprintf("Failed to query DNS from host: %v", err))
	}

	if err := cluster.EnsurePullSecretPresentOnInstanceDisk(sshRunner, startConfig.PullSecret); err != nil {
		return nil, errors.Wrap(err, "Failed to update VM pull secret")
	}

	if err := ensureKubeletAndCRIOAreConfiguredForProxy(sshRunner, proxyConfig, instanceIP); err != nil {
		return nil, errors.Wrap(err, "Failed to update proxy configuration of kubelet and crio")
	}

	// Check the certs validity inside the vm
	logging.Info("Verifying validity of the kubelet certificates ...")
	certsExpired, err := cluster.CheckCertsValidity(sshRunner)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to check certificate validity")
	}

	logging.Info("Starting OpenShift kubelet service")
	sd := systemd.NewInstanceSystemdCommander(sshRunner)
	if err := sd.Start("kubelet"); err != nil {
		return nil, errors.Wrap(err, "Error starting kubelet")
	}

	ocConfig := oc.UseOCWithSSH(sshRunner)

	if err := cluster.ApproveCSRAndWaitForCertsRenewal(sshRunner, ocConfig, certsExpired[cluster.KubeletClientCert], certsExpired[cluster.KubeletServerCert]); err != nil {
		logBundleDate(crcBundleMetadata)
		return nil, errors.Wrap(err, "Failed to renew TLS certificates: please check if a newer CodeReady Containers release is available")
	}

	if err := cluster.WaitForAPIServer(ocConfig); err != nil {
		return nil, errors.Wrap(err, "Error waiting for apiserver")
	}

	if err := ensureProxyIsConfiguredInOpenShift(ocConfig, sshRunner, proxyConfig, instanceIP); err != nil {
		return nil, errors.Wrap(err, "Failed to update cluster proxy configuration")
	}

	if err := cluster.EnsurePullSecretPresentInTheCluster(ocConfig, startConfig.PullSecret); err != nil {
		return nil, errors.Wrap(err, "Failed to update cluster pull secret")
	}

	if err := cluster.EnsureClusterIDIsNotEmpty(ocConfig); err != nil {
		return nil, errors.Wrap(err, "Failed to update cluster ID")
	}

	if client.monitoringEnabled() {
		logging.Info("Enabling cluster monitoring operator...")
		if err := cluster.StartMonitoring(ocConfig); err != nil {
			return nil, errors.Wrap(err, "Cannot start monitoring stack")
		}
	}

	// In Openshift 4.3, when cluster comes up, the following happens
	// 1. After the openshift-apiserver pod is started, its log contains multiple occurrences of `certificate has expired or is not yet valid`
	// 2. Initially there is no request-header's client-ca crt available to `extension-apiserver-authentication` configmap
	// 3. In the pod logs `missing content for CA bundle "client-ca::kube-system::extension-apiserver-authentication::requestheader-client-ca-file"`
	// 4. After ~1 min /etc/kubernetes/static-pod-resources/kube-apiserver-certs/configmaps/aggregator-client-ca/ca-bundle.crt is regenerated
	// 5. It is now also appear to `extension-apiserver-authentication` configmap as part of request-header's client-ca content
	// 6. Openshift-apiserver is able to load the CA which was regenerated
	// 7. Now apiserver pod log contains multiple occurrences of `error x509: certificate signed by unknown authority`
	// When the openshift-apiserver is in this state, the cluster is non functional.
	// A restart of the openshift-apiserver pod is enough to clear that error and get a working cluster.
	// This is a work-around while the root cause is being identified.
	// More info: https://bugzilla.redhat.com/show_bug.cgi?id=1795163
	if certsExpired[cluster.AggregatorClientCert] {
		logging.Debug("Waiting for the renewal of the request header client ca...")
		if err := cluster.WaitForRequestHeaderClientCaFile(sshRunner); err != nil {
			return nil, errors.Wrap(err, "Failed to wait for aggregator client ca renewal")
		}

		if err := cluster.DeleteOpenshiftAPIServerPods(ocConfig); err != nil {
			return nil, errors.Wrap(err, "Cannot delete OpenShift API Server pods")
		}
	}

	logging.Info("Starting OpenShift cluster ... [waiting 3m]")

	time.Sleep(time.Minute * 3)

	waitForProxyPropagation(ocConfig, proxyConfig)

	logging.Info("Updating kubeconfig")
	if err := eventuallyWriteKubeconfig(ocConfig, instanceIP, clusterConfig); err != nil {
		logging.Warnf("Cannot update kubeconfig: %v", err)
	}

	logging.Warn("The cluster might report a degraded or error state. This is expected since several operators have been disabled to lower the resource usage. For more information, please consult the documentation")
	return &StartResult{
		KubeletStarted: true,
		ClusterConfig:  *clusterConfig,
		Status:         vmState,
	}, nil
}

func (client *client) validateStartConfig(startConfig StartConfig) error {
	if client.monitoringEnabled() && startConfig.Memory < minimumMemoryForMonitoring {
		return fmt.Errorf("Too little memory (%s) allocated to the virtual machine to start the monitoring stack, %s is the minimum",
			units.BytesSize(float64(startConfig.Memory)*1024*1024),
			units.BytesSize(minimumMemoryForMonitoring*1024*1024))
	}
	return nil
}

// makeDaemonVisibleToHyperkit crc daemon is launched in background and doesn't know where hyperkit is running.
// In order to vsock to work with hyperkit, we need to put the unix socket in the hyperkit working directory with a
// special name. The name is the hex representation of the cid and the vsock port.
// This function adds the unix socket in the hyperkit directory.
func makeDaemonVisibleToHyperkit(name string) error {
	dst := filepath.Join(constants.MachineInstanceDir, name, "00000002.00000400")
	if _, err := os.Stat(dst); err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "VSock listener error")
		}
		if err := os.Symlink(constants.NetworkSocketPath, dst); err != nil {
			return errors.Wrap(err, "VSock listener error")
		}
	}
	return nil
}

func createHost(api libmachine.API, machineConfig config.MachineConfig) (*host.Host, error) {
	vm, err := newHost(api, machineConfig)
	if err != nil {
		return nil, fmt.Errorf("Error creating new host: %s", err)
	}
	if err := api.Create(vm); err != nil {
		return nil, fmt.Errorf("Error creating the VM: %s", err)
	}
	return vm, nil
}

func addNameServerToInstance(sshRunner *crcssh.Runner, ns string) error {
	nameserver := network.NameServer{IPAddress: ns}
	nameservers := []network.NameServer{nameserver}
	exist, err := network.HasGivenNameserversConfigured(sshRunner, nameserver)
	if err != nil {
		return err
	}
	if !exist {
		logging.Infof("Adding %s as nameserver to the instance ...", nameserver.IPAddress)
		return network.AddNameserversToInstance(sshRunner, nameservers)
	}
	return nil
}

func updateSSHKeyPair(sshRunner *crcssh.Runner) error {
	if _, err := os.Stat(constants.GetPrivateKeyPath()); err != nil {
		if !os.IsNotExist(err) {
			return err
		}

		// Generate ssh key pair
		logging.Info("Generating new SSH Key pair ...")
		if err := ssh.GenerateSSHKey(constants.GetPrivateKeyPath()); err != nil {
			return fmt.Errorf("Error generating ssh key pair: %v", err)
		}
	}

	// Read generated public key
	publicKey, err := ioutil.ReadFile(constants.GetPublicKeyPath())
	if err != nil {
		return err
	}

	authorizedKeys, _, err := sshRunner.Run("cat /home/core/.ssh/authorized_keys")
	if err == nil && strings.TrimSpace(authorizedKeys) == strings.TrimSpace(string(publicKey)) {
		return nil
	}

	logging.Info("Updating authorized keys ...")
	cmd := fmt.Sprintf("echo '%s' > /home/core/.ssh/authorized_keys; chmod 644 /home/core/.ssh/authorized_keys", publicKey)
	_, _, err = sshRunner.Run(cmd)
	if err != nil {
		return err
	}
	return err
}

func updateSSHKeyAndCopyKubeconfig(sshRunner *crcssh.Runner, name string, crcBundleMetadata *bundle.CrcBundleInfo) error {
	if err := updateSSHKeyPair(sshRunner); err != nil {
		return fmt.Errorf("Error updating SSH Keys: %v", err)
	}

	kubeConfigFilePath := filepath.Join(constants.MachineInstanceDir, name, "kubeconfig")
	if _, err := os.Stat(kubeConfigFilePath); err == nil {
		return nil
	}

	// Copy Kubeconfig file from bundle extract path to machine directory.
	// In our case it would be ~/.crc/machines/crc/
	logging.Info("Copying kubeconfig file to instance dir ...")
	err := crcos.CopyFileContents(crcBundleMetadata.GetKubeConfigPath(),
		kubeConfigFilePath,
		0644)
	if err != nil {
		return fmt.Errorf("Error copying kubeconfig file to instance dir: %v", err)
	}
	return nil
}

func ensureKubeletAndCRIOAreConfiguredForProxy(sshRunner *crcssh.Runner, proxy *network.ProxyConfig, instanceIP string) (err error) {
	if !proxy.IsEnabled() {
		return nil
	}
	logging.Info("Adding proxy configuration to kubelet and crio service ...")
	return cluster.AddProxyToKubeletAndCriO(sshRunner, proxy)
}

func ensureProxyIsConfiguredInOpenShift(ocConfig oc.Config, sshRunner *crcssh.Runner, proxy *network.ProxyConfig, instanceIP string) (err error) {
	if !proxy.IsEnabled() {
		return nil
	}
	logging.Info("Adding proxy configuration to the cluster ...")
	return cluster.AddProxyConfigToCluster(sshRunner, ocConfig, proxy)
}

func waitForProxyPropagation(ocConfig oc.Config, proxyConfig *network.ProxyConfig) {
	if !proxyConfig.IsEnabled() {
		return
	}
	logging.Info("Waiting for the proxy configuration to be applied ...")
	checkProxySettingsForOperator := func() error {
		proxySet, err := cluster.CheckProxySettingsForOperator(ocConfig, proxyConfig, "marketplace-operator", "openshift-marketplace")
		if err != nil {
			logging.Debugf("Error getting proxy setting for openshift-marketplace operator %v", err)
			return &crcerrors.RetriableError{Err: err}
		}
		if !proxySet {
			logging.Debug("Proxy changes for cluster in progress")
			return &crcerrors.RetriableError{Err: fmt.Errorf("")}
		}
		return nil
	}

	if err := crcerrors.RetryAfter(300*time.Second, checkProxySettingsForOperator, 2*time.Second); err != nil {
		logging.Debug("Failed to propagate proxy settings to cluster")
	}
}

func logBundleDate(crcBundleMetadata *bundle.CrcBundleInfo) {
	if buildTime, err := crcBundleMetadata.GetBundleBuildTime(); err == nil {
		bundleAgeDays := time.Since(buildTime).Hours() / 24
		if bundleAgeDays >= 30 {
			/* Initial bundle certificates are only valid for 30 days */
			logging.Debugf("Bundle has been generated %d days ago", int(bundleAgeDays))
		}
	}
}
