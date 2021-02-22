package constants

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/YourFin/binappend"
	"github.com/code-ready/crc/pkg/crc/version"
)

const (
	DefaultName     = "crc"
	DefaultCPUs     = 4
	DefaultMemory   = 9216
	DefaultDiskSize = 31

	DefaultSSHUser = "core"
	DefaultSSHPort = 22

	CrcEnvPrefix = "CRC"

	DefaultWebConsoleURL      = "https://console-openshift-console.apps-crc.testing"
	DefaultAPIURL             = "https://api.crc.testing:6443"
	DefaultLogLevel           = "info"
	ConfigFile                = "crc.json"
	LogFile                   = "crc.log"
	DaemonLogFile             = "crcd.log"
	CrcLandingPageURL         = "https://cloud.redhat.com/openshift/create/local" // #nosec G101
	DefaultPodmanURLBase      = "https://storage.googleapis.com/libpod-master-releases"
	DefaultAdminHelperCliBase = "https://github.com/code-ready/admin-helper/releases/download/0.0.2"
	CRCMacTrayDownloadURL     = "https://github.com/code-ready/tray-macos/releases/download/v%s/crc-tray-macos.tar.gz"
	CRCWindowsTrayDownloadURL = "https://github.com/code-ready/tray-windows/releases/download/v%s/crc-tray-windows.zip"
	DefaultContext            = "admin"

	VSockGateway = "192.168.127.1"
	VsockSSHPort = 2222

	OkdPullSecret = `{"auths":{"fake":{"auth": "Zm9vOmJhcgo="}}}` // #nosec G101

	ClusterDomain = ".crc.testing"
	AppsDomain    = ".apps-crc.testing"
)

var podmanURLForOs = map[string]string{
	"darwin":  fmt.Sprintf("%s/%s", DefaultPodmanURLBase, "podman-remote-latest-master-darwin-amd64.zip"),
	"linux":   fmt.Sprintf("%s/%s", DefaultPodmanURLBase, "podman-remote-latest-master-linux---amd64.zip"),
	"windows": fmt.Sprintf("%s/%s", DefaultPodmanURLBase, "podman-remote-latest-master-windows-amd64.zip"),
}

func GetPodmanURLForOs(os string) string {
	return podmanURLForOs[os]
}

func GetPodmanURL() string {
	return podmanURLForOs[runtime.GOOS]
}

var adminHelperURLForOs = map[string]string{
	"darwin":  fmt.Sprintf("%s/%s", DefaultAdminHelperCliBase, "admin-helper-darwin"),
	"linux":   fmt.Sprintf("%s/%s", DefaultAdminHelperCliBase, "admin-helper-linux"),
	"windows": fmt.Sprintf("%s/%s", DefaultAdminHelperCliBase, "admin-helper-windows.exe"),
}

func GetAdminHelperURLForOs(os string) string {
	return adminHelperURLForOs[os]
}

func GetAdminHelperURL() string {
	return adminHelperURLForOs[runtime.GOOS]
}

func defaultBundleForOs(bundleVersion string) map[string]string {
	return map[string]string{
		"darwin":  fmt.Sprintf("crc_hyperkit_%s.crcbundle", bundleVersion),
		"linux":   fmt.Sprintf("crc_libvirt_%s.crcbundle", bundleVersion),
		"windows": fmt.Sprintf("crc_hyperv_%s.crcbundle", bundleVersion),
	}
}

func GetBundleFosOs(os, bundleVersion string) string {
	return defaultBundleForOs(bundleVersion)[os]
}

func GetDefaultBundle() string {
	return GetBundleFosOs(runtime.GOOS, version.GetBundleVersion())
}

var (
	CrcBaseDir         = filepath.Join(GetHomeDir(), ".crc")
	CrcBinDir          = filepath.Join(CrcBaseDir, "bin")
	CrcOcBinDir        = filepath.Join(CrcBinDir, "oc")
	ConfigPath         = filepath.Join(CrcBaseDir, ConfigFile)
	LogFilePath        = filepath.Join(CrcBaseDir, LogFile)
	DaemonLogFilePath  = filepath.Join(CrcBaseDir, DaemonLogFile)
	MachineBaseDir     = CrcBaseDir
	MachineCacheDir    = filepath.Join(MachineBaseDir, "cache")
	MachineInstanceDir = filepath.Join(MachineBaseDir, "machines")
	DefaultBundlePath  = defaultBundlePath()
	DaemonSocketPath   = filepath.Join(CrcBaseDir, "crc.sock")
	NetworkSocketPath  = filepath.Join(CrcBaseDir, "network.sock")
)

func defaultBundlePath() string {
	if runtime.GOOS == "darwin" && version.IsMacosInstallPathSet() {
		path := filepath.Join(version.GetMacosInstallPath(), version.GetCRCVersion(), GetDefaultBundle())
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return filepath.Join(MachineCacheDir, GetDefaultBundle())
}

// GetHomeDir returns the home directory for the current user
func GetHomeDir() string {
	if runtime.GOOS == "windows" {
		if homeDrive, homePath := os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH"); len(homeDrive) > 0 && len(homePath) > 0 {
			homeDir := filepath.Join(homeDrive, homePath)
			if _, err := os.Stat(homeDir); err == nil {
				return homeDir
			}
		}
		if userProfile := os.Getenv("USERPROFILE"); len(userProfile) > 0 {
			if _, err := os.Stat(userProfile); err == nil {
				return userProfile
			}
		}
	}
	return os.Getenv("HOME")
}

// EnsureBaseDirectoryExists create the ~/.crc directory if it is not present
func EnsureBaseDirectoriesExist() error {
	return os.MkdirAll(CrcBaseDir, 0750)
}

// BundleEmbedded returns true if the executable was compiled to contain the bundle
func BundleEmbedded() bool {
	executablePath, err := os.Executable()
	if err != nil {
		return false
	}
	extractor, err := binappend.MakeExtractor(executablePath)
	if err != nil {
		return false
	}
	return contains(extractor.AvalibleData(), GetDefaultBundle())
}

func IsRelease() bool {
	return BundleEmbedded() || version.IsMacosInstallPathSet()
}

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func GetPublicKeyPath() string {
	return filepath.Join(MachineInstanceDir, DefaultName, "id_ecdsa.pub")
}

func GetPrivateKeyPath() string {
	return filepath.Join(MachineInstanceDir, DefaultName, "id_ecdsa")
}

// For backward compatibility to v 1.20.0
func GetRsaPrivateKeyPath() string {
	return filepath.Join(MachineInstanceDir, DefaultName, "id_rsa")
}

// TODO: follow the same pattern as oc and podman above
func GetCRCMacTrayDownloadURL() string {
	return fmt.Sprintf(CRCMacTrayDownloadURL, version.GetCRCMacTrayVersion())
}

func GetCRCWindowsTrayDownloadURL() string {
	return fmt.Sprintf(CRCWindowsTrayDownloadURL, version.GetCRCWindowsTrayVersion())
}
