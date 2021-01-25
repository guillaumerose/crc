package adminhelper

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/code-ready/crc/pkg/crc/constants"
)

var (
	goodhostPath = filepath.Join(constants.CrcBinDir, constants.AdminHelperExecutableName)
)

// UpdateHostsFile updates the host's /etc/hosts file with Instance IP.
func UpdateHostsFile(instanceIP string, hostnames ...string) error {
	if err := RemoveFromHostsFile(hostnames...); err != nil {
		return err
	}
	if err := AddToHostsFile(instanceIP, hostnames...); err != nil {
		return err
	}
	return nil
}

func AddToHostsFile(instanceIP string, hostnames ...string) error {
	if err := validateHostnames(hostnames); err != nil {
		return err
	}
	return execute(append([]string{"add", instanceIP}, hostnames...)...)
}

func RemoveFromHostsFile(hostnames ...string) error {
	if err := validateHostnames(hostnames); err != nil {
		return err
	}
	return execute(append([]string{"rm"}, hostnames...)...)
}

func CleanHostsFile() error {
	return execute([]string{"clean", constants.ClusterDomain, constants.AppsDomain}...)
}

func validateHostnames(hostnames []string) error {
	for _, hostname := range hostnames {
		if !strings.HasSuffix(hostname, constants.ClusterDomain) && !strings.HasSuffix(hostname, constants.AppsDomain) {
			return fmt.Errorf("adding %s to hosts file is denied (outside of %s and %s subdomains)", hostname, constants.ClusterDomain, constants.AppsDomain)
		}
	}
	return nil
}
