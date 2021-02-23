package libmachine

import (
	"encoding/json"

	"github.com/code-ready/crc/pkg/drivers/hyperv"
	"github.com/code-ready/machine/libmachine/drivers"
)

func hypervDriver(rawDriver []byte) (drivers.Driver, error) {
	driver := hyperv.NewDriver("", "")
	if err := json.Unmarshal(rawDriver, &driver); err != nil {
		return nil, err
	}
	return driver, nil
}
