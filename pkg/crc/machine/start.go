package machine

import (
	"context"
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
	"github.com/code-ready/crc/pkg/crc/ssh"
	crcssh "github.com/code-ready/crc/pkg/crc/ssh"
	"github.com/code-ready/crc/pkg/crc/telemetry"
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

func (client *client) Start(ctx context.Context, startConfig StartConfig) (*StartResult, error) {
	telemetry.SetCPUs(ctx, startConfig.CPUs)
	telemetry.SetMemory(ctx, uint64(startConfig.Memory)*1024*1024)
	telemetry.SetDiskSize(ctx, uint64(startConfig.DiskSize)*1024*1024*1024)

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
		telemetry.SetStartType(ctx, telemetry.CreationStartType)

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

		logging.Infof("Creating virtual machine...")

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
	} else { // exists
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
			logging.Infof("The virtual machine is already running")
			clusterConfig, err := getClusterConfig(crcBundleMetadata)
			if err != nil {
				return nil, errors.Wrap(err, "Cannot create cluster configuration")
			}

			telemetry.SetStartType(ctx, telemetry.AlreadyRunningStartType)
			return &StartResult{
				Status:         vmState,
				ClusterConfig:  *clusterConfig,
				KubeletStarted: true,
			}, nil
		}

		telemetry.SetStartType(ctx, telemetry.StartStartType)

		logging.Infof("Starting virtual machine...")

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
		return nil, errors.Wrap(err, "Virtual machine is not running")
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
	logging.Info("Virtual machine is running")

	// Post VM start immediately update SSH key and copy kubeconfig to instance
	// dir and VM
	if err := updateSSHKeyAndCopyKubeconfig(sshRunner, client.name, crcBundleMetadata); err != nil {
		return nil, errors.Wrap(err, "Error updating public key")
	}

	// Trigger disk resize, this will be a no-op if no disk size change is needed
	if _, _, err = sshRunner.Run("sudo xfs_growfs / >/dev/null"); err != nil {
		return nil, errors.Wrap(err, "Error updating filesystem size")
	}

	err = sshRunner.CopyData([]byte("nameserver 192.168.127.1\nsearch crc.testing"), "/etc/resolv.conf", 0644)
	if err != nil {
		return nil, fmt.Errorf("Error creating /etc/resolv on instance: %s", err.Error())
	}

	// Start network time synchronization if `CRC_DEBUG_ENABLE_STOP_NTP` is not set
	if stopNtp, _ := strconv.ParseBool(os.Getenv("CRC_DEBUG_ENABLE_STOP_NTP")); !stopNtp {
		logging.Info("Starting network time synchronization in CodeReady Containers VM")
		if _, _, err := sshRunner.Run("sudo timedatectl set-ntp on"); err != nil {
			return nil, errors.Wrap(err, "Failed to start network time synchronization")
		}
	}

	return &StartResult{
		KubeletStarted: true,
		ClusterConfig:  *clusterConfig,
		Status:         vmState,
	}, nil
}

func (client *client) IsRunning() (bool, error) {
	libMachineAPIClient, cleanup := createLibMachineClient()
	defer cleanup()
	host, err := libMachineAPIClient.Load(client.name)

	if err != nil {
		return false, errors.Wrap(err, "Cannot load machine")
	}

	// get the actual state
	vmState, err := host.Driver.GetState()
	if err != nil {
		// but reports not started on error
		return false, errors.Wrap(err, "Error getting the state")
	}
	if vmState != state.Running {
		return false, nil
	}
	return true, nil
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
