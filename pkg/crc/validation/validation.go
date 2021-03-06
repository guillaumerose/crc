package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/code-ready/crc/pkg/crc/constants"
	"github.com/code-ready/crc/pkg/crc/logging"
	"github.com/code-ready/crc/pkg/crc/machine/bundle"
	"github.com/docker/go-units"
	"github.com/pbnjay/memory"
)

// ValidateCPUs checks if provided cpus count is valid
func ValidateCPUs(value int) error {
	if value < constants.DefaultCPUs {
		return fmt.Errorf("requires CPUs >= %d", constants.DefaultCPUs)
	}
	return nil
}

// ValidateMemory checks if provided Memory count is valid
func ValidateMemory(value int) error {
	if value < constants.DefaultMemory {
		return fmt.Errorf("requires memory in MiB >= %d", constants.DefaultMemory)
	}
	return ValidateEnoughMemory(value)
}

func ValidateDiskSize(value int) error {
	if value < constants.DefaultDiskSize {
		return fmt.Errorf("requires disk size in GiB >= %d", constants.DefaultDiskSize)
	}
	// https://github.com/code-ready/machine-driver-hyperkit/issues/18
	if runtime.GOOS == "darwin" && value > constants.DefaultDiskSize {
		return fmt.Errorf("Disk resizing is not supported on macOS")
	}

	return nil
}

// ValidateEnoughMemory checks if enough memory is installed on the host
func ValidateEnoughMemory(value int) error {
	totalMemory := memory.TotalMemory()
	logging.Debugf("Total memory of system is %d bytes", totalMemory)
	valueBytes := value * 1024 * 1024
	if totalMemory < uint64(valueBytes) {
		return fmt.Errorf("only %s of memory found (%s required)",
			units.HumanSize(float64(totalMemory)),
			units.HumanSize(float64(valueBytes)))
	}
	return nil
}

// ValidateBundlePath checks if the provided bundle path exist
func ValidateBundlePath(bundle string) error {
	if err := ValidatePath(bundle); err != nil {
		if constants.BundleEmbedded() {
			return fmt.Errorf("Run 'crc setup' to unpack the bundle to disk")
		}
		return fmt.Errorf("%s not found, please provide the path to a valid bundle using the -b option", bundle)
	}

	userProvidedBundle := filepath.Base(bundle)
	if userProvidedBundle != constants.GetDefaultBundle() {
		if !constants.IsRelease() {
			logging.Warnf("Using unsupported bundle %s", userProvidedBundle)
			return nil
		}
		return fmt.Errorf("%s is not supported by this crc executable, please use %s", userProvidedBundle, constants.GetDefaultBundle())
	}
	return nil
}

func ValidateBundle(bundlePath string) error {
	bundleName := filepath.Base(bundlePath)
	_, err := bundle.GetCachedBundleInfo(bundleName)
	if err != nil {
		return ValidateBundlePath(bundlePath)
	}
	/* 'bundle' is already unpacked in ~/.crc/cache */
	return nil
}

// ValidateIPAddress checks if provided IP is valid
func ValidateIPAddress(ipAddress string) error {
	ip := net.ParseIP(ipAddress).To4()
	if ip == nil {
		return fmt.Errorf("'%s' is not a valid IPv4 address", ipAddress)
	}
	return nil
}

// ValidatePath check if provide path is exist
func ValidatePath(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file '%s' does not exist", path)
	}
	return nil
}

type imagePullSecret struct {
	Auths map[string]map[string]interface{} `json:"auths"`
}

// ImagePullSecret checks if the given string is a valid image pull secret and returns an error if not.
func ImagePullSecret(secret string) error {
	if secret == "" {
		return errors.New("empty pull secret")
	}
	var s imagePullSecret
	err := json.Unmarshal([]byte(secret), &s)
	if err != nil {
		return fmt.Errorf("invalid pull secret: %v", err)
	}
	if len(s.Auths) == 0 {
		return fmt.Errorf("invalid pull secret: missing 'auths' JSON-object field")
	}

	for d, a := range s.Auths {
		_, authPresent := a["auth"]
		_, credsStorePresent := a["credsStore"]
		if !authPresent && !credsStorePresent {
			return fmt.Errorf("invalid pull secret, '%q' JSON-object requires either 'auth' or 'credsStore' field", d)
		}
	}
	return nil
}
