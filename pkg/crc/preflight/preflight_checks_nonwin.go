// +build !windows

package preflight

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/code-ready/crc/pkg/crc/adminhelper"
	"github.com/code-ready/crc/pkg/crc/logging"
	crcos "github.com/code-ready/crc/pkg/os"
)

const (
	rootCheck = "check-root-user"
	rootError = "crc should not be ran as root. Turn off this check with 'crc config set skip-" + rootCheck + " true'"
)

var nonWinPreflightChecks = [...]Check{
	{
		configKeySuffix:  rootCheck,
		checkDescription: "Checking if running as root",
		check:            checkIfRunningAsNormalUser,
		fixDescription:   rootError,
		flags:            NoFix,
	},
	{
		cleanupDescription: "Removing hosts file records added by CRC",
		cleanup:            adminhelper.CleanHostsFile,
		flags:              CleanUpOnly,
	},
}

func checkIfRunningAsNormalUser() error {
	if os.Geteuid() != 0 {
		return nil
	}
	logging.Debug("Ran as root")
	return errors.New(rootError)
}

func setSuid(path string) error {
	logging.Debugf("Making %s suid", path)

	stdOut, stdErr, err := crcos.RunPrivileged(fmt.Sprintf("Changing ownership of %s", path), "chown", "root", path)
	if err != nil {
		return fmt.Errorf("Unable to set ownership of %s to root: %s %v: %s",
			path, stdOut, err, stdErr)
	}

	/* Can't do this before the chown as the chown will reset the suid bit */
	stdOut, stdErr, err = crcos.RunPrivileged(fmt.Sprintf("Setting suid for %s", path), "chmod", "u+s,g+x", path)
	if err != nil {
		return fmt.Errorf("Unable to set suid bit on %s: %s %v: %s", path, stdOut, err, stdErr)
	}
	return nil
}

func checkSuid(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSetuid == 0 {
		return fmt.Errorf("%s does not have the SUID bit set (%s)", path, fi.Mode().String())
	}
	if fi.Sys().(*syscall.Stat_t).Uid != 0 {
		return fmt.Errorf("%s is not owned by root", path)
	}

	return nil
}
