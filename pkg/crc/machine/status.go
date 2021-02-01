package machine

import (
	"github.com/code-ready/crc/pkg/crc/cluster"
	"github.com/code-ready/crc/pkg/crc/constants"
	"github.com/code-ready/crc/pkg/crc/logging"
	"github.com/code-ready/crc/pkg/crc/oc"
	crcssh "github.com/code-ready/crc/pkg/crc/ssh"
	"github.com/code-ready/machine/libmachine/state"
	"github.com/pkg/errors"
)

func (client *client) Status() (*ClusterStatusResult, error) {
	libMachineAPIClient, cleanup := createLibMachineClient()
	defer cleanup()

	_, err := libMachineAPIClient.Exists(client.name)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot check if machine exists")
	}

	host, err := libMachineAPIClient.Load(client.name)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot load machine")
	}
	vmStatus, err := host.Driver.GetState()
	if err != nil {
		return nil, errors.Wrap(err, "Cannot get machine state")
	}

	if vmStatus != state.Running {
		return &ClusterStatusResult{
			CrcStatus:       vmStatus,
			OpenshiftStatus: "Stopped",
		}, nil
	}

	_, crcBundleMetadata, err := getBundleMetadataFromDriver(host.Driver)
	if err != nil {
		return nil, errors.Wrap(err, "Error loading bundle metadata")
	}
	proxyConfig, err := getProxyConfig(crcBundleMetadata.ClusterInfo.BaseDomain)
	if err != nil {
		return nil, errors.Wrap(err, "Error getting proxy configuration")
	}
	proxyConfig.ApplyToEnvironment()

	ip, err := getIP(host, client.useVSock())
	if err != nil {
		return nil, errors.Wrap(err, "Error getting ip")
	}
	sshRunner, err := crcssh.CreateRunner(ip, getSSHPort(client.useVSock()), constants.GetPrivateKeyPath(), constants.GetRsaPrivateKeyPath())
	if err != nil {
		return nil, errors.Wrap(err, "Error creating the ssh client")
	}
	defer sshRunner.Close()
	// check if all the clusteroperators are running
	diskSize, diskUse, err := cluster.GetRootPartitionUsage(sshRunner)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot get root partition usage")
	}
	return &ClusterStatusResult{
		CrcStatus:        state.Running,
		OpenshiftStatus:  getOpenShiftStatus(sshRunner, client.monitoringEnabled()),
		OpenshiftVersion: crcBundleMetadata.GetOpenshiftVersion(),
		DiskUse:          diskUse,
		DiskSize:         diskSize,
	}, nil
}

func getOpenShiftStatus(sshRunner *crcssh.Runner, monitoringEnabled bool) string {
	status, err := cluster.GetClusterOperatorsStatus(oc.UseOCWithSSH(sshRunner), monitoringEnabled)
	if err != nil {
		logging.Debugf("cannot get OpenShift status: %v", err)
		return "Not Reachable"
	}
	if status.Progressing {
		return "Starting"
	}
	if status.Degraded {
		return "Degraded"
	}
	if status.Available {
		return "Running"
	}
	return "Stopped"
}
