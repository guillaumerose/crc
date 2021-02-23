// +build !windows

package libmachine

import (
	"errors"

	"github.com/code-ready/machine/libmachine/drivers"
)

func hypervDriver(rawDriver []byte) (drivers.Driver, error) {
	return nil, errors.New("driver not supported")
}
