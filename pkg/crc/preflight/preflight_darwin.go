package preflight

import (
	"fmt"
)

// SetupHost performs the prerequisite checks and setups the host to run the cluster
func hyperkitPreflightChecks(experimentalFeatures bool) []Check {
	return []Check{
		{
			configKeySuffix:  "check-hyperkit-installed",
			checkDescription: "Checking if HyperKit is installed",
			check:            checkHyperKitInstalled(experimentalFeatures),
			fixDescription:   "Setting up virtualization with HyperKit",
			fix:              fixHyperKitInstallation(experimentalFeatures),
		},
		{
			configKeySuffix:  "check-hyperkit-driver",
			checkDescription: "Checking if crc-driver-hyperkit is installed",
			check:            checkMachineDriverHyperKitInstalled(experimentalFeatures),
			fixDescription:   "Installing crc-machine-hyperkit",
			fix:              fixMachineDriverHyperKitInstalled(experimentalFeatures),
		},
	}
}

var dnsPreflightChecks = [...]Check{
	{
		configKeySuffix:  "check-hosts-file-permissions",
		checkDescription: fmt.Sprintf("Checking file permissions for %s", hostsFile),
		check:            checkEtcHostsFilePermissions,
		fixDescription:   fmt.Sprintf("Setting file permissions for %s", hostsFile),
		fix:              fixEtcHostsFilePermissions,
	},
}

var resolverPreflightChecks = [...]Check{
	{
		configKeySuffix:    "check-resolver-file-permissions",
		checkDescription:   fmt.Sprintf("Checking file permissions for %s", resolverFile),
		check:              checkResolverFilePermissions,
		fixDescription:     fmt.Sprintf("Setting file permissions for %s", resolverFile),
		fix:                fixResolverFilePermissions,
		cleanupDescription: fmt.Sprintf("Removing %s file", resolverFile),
		cleanup:            removeResolverFile,
	},
}

var traySetupChecks = [...]Check{
	{
		checkDescription: "Checking if tray binary is installed",
		check:            checkTrayBinaryPresent,
		fixDescription:   "Installing and setting up tray",
		fix:              fixTrayBinaryPresent,
		flags:            SetupOnly,
	},
	{
		checkDescription:   "Checking if launchd configuration for daemon exists",
		check:              checkIfDaemonPlistFileExists,
		fixDescription:     "Creating launchd configuration for daemon",
		fix:                fixDaemonPlistFileExists,
		flags:              SetupOnly,
		cleanupDescription: "Removing launchd configuration for daemon",
		cleanup:            removeDaemonPlistFile,
	},
	{
		checkDescription:   "Checking if launchd configuration for tray exists",
		check:              checkIfTrayPlistFileExists,
		fixDescription:     "Creating launchd configuration for tray",
		fix:                fixTrayPlistFileExists,
		flags:              SetupOnly,
		cleanupDescription: "Removing launchd configuration for tray",
		cleanup:            removeTrayPlistFile,
	},
	{
		checkDescription: "Checking installed tray version",
		check:            checkTrayVersion,
		fixDescription:   "Installing and setting up tray app",
		fix:              fixTrayVersion,
		flags:            SetupOnly,
	},
	{
		checkDescription:   "Checking if CodeReady Containers daemon is running",
		check:              checkIfDaemonAgentRunning,
		fixDescription:     "Starting CodeReady Containers daemon",
		fix:                fixDaemonAgentRunning,
		flags:              SetupOnly,
		cleanupDescription: "Unload CodeReady Containers daemon",
		cleanup:            unLoadDaemonAgent,
	},
	{
		checkDescription:   "Check if CodeReady Containers tray is running",
		check:              checkIfTrayAgentRunning,
		fixDescription:     "Starting CodeReady Containers tray",
		fix:                fixTrayAgentRunning,
		flags:              SetupOnly,
		cleanupDescription: "Unload CodeReady Containers tray",
		cleanup:            unLoadTrayAgent,
	},
}

func getAllPreflightChecks() []Check {
	return append(getPreflightChecks(true), getPreflightChecks(false)...)
}

func getPreflightChecks(experimentalFeatures bool) []Check {
	checks := []Check{}

	checks = append(checks, genericPreflightChecks[:]...)
	checks = append(checks, nonWinPreflightChecks[:]...)
	checks = append(checks, hyperkitPreflightChecks(experimentalFeatures)...)
	checks = append(checks, dnsPreflightChecks[:]...)

	if !experimentalFeatures {
		checks = append(checks, resolverPreflightChecks[:]...)
	}
	// Experimental feature
	if experimentalFeatures {
		checks = append(checks, traySetupChecks[:]...)
	}

	return checks
}
