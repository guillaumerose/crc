package ssh

import (
	"errors"
	"fmt"
	"github.com/code-ready/crc/pkg/crc/constants"
	"github.com/code-ready/crc/pkg/crc/logging"
	"github.com/code-ready/machine/libmachine/drivers"
	"os"
	"yunion.io/x/executor/client"
)

type Runner struct {
	driver        drivers.Driver
	privateSSHKey string
}

func CreateRunner(driver drivers.Driver) *Runner {
	return CreateRunnerWithPrivateKey(driver, constants.GetPrivateKeyPath())
}

func CreateRunnerWithPrivateKey(driver drivers.Driver, privateKey string) *Runner {
	return &Runner{driver: driver, privateSSHKey: privateKey}
}

// Create a host using the driver's config
func (runner *Runner) Run(command string) (string, error) {
	return runner.runSSHCommandFromDriver(command, false)
}

func (runner *Runner) SetTextContentAsRoot(destFilename string, content string, mode os.FileMode) error {
	logging.Debugf("Creating %s with permissions 0%o in the CRC VM", destFilename, mode)
	command := fmt.Sprintf("sudo install -m 0%o /dev/null %s && cat <<EOF | sudo tee %s\n%s\nEOF", mode, destFilename, destFilename, content)
	_, err := runner.RunPrivate(command)
	return err
}

func (runner *Runner) RunPrivate(command string) (string, error) {
	return runner.runSSHCommandFromDriver(command, true)
}

func (runner *Runner) SetPrivateKeyPath(path string) {
	runner.privateSSHKey = path
}

func (runner *Runner) CopyData(data []byte, destFilename string) error {
	return errors.New("unsupported")
}

func (runner *Runner) runSSHCommandFromDriver(command string, runPrivate bool) (string, error) {
	client.Init("")
	cmd := client.Command("/bin/sh", "-c", command)
	stdout, err := cmd.Output()
	return string(stdout), err
}
