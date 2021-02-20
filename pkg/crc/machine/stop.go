package machine

import (
	"github.com/code-ready/crc/pkg/crc/logging"
	"github.com/code-ready/machine/libmachine/state"
	"github.com/pkg/errors"
)

func (client *client) Stop() (state.State, error) {
	libMachineAPIClient, cleanup := createLibMachineClient()
	defer cleanup()
	host, err := libMachineAPIClient.Load(client.name)

	if err != nil {
		return state.None, errors.Wrap(err, "Cannot load machine")
	}

	logging.Info("Stopping the virtual machine, this may take a few minutes...")
	if err := host.Stop(); err != nil {
		status, err := host.Driver.GetState()
		if err != nil {
			logging.Debugf("Cannot get VM status after stopping it: %v", err)
		}
		return status, errors.Wrap(err, "Cannot stop machine")
	}
	return host.Driver.GetState()
}
