// +build !windows

package adminhelper

import (
	"github.com/code-ready/admin-helper/pkg/types"
	crcos "github.com/code-ready/crc/pkg/os"
)

func execute(args ...string) error {
	_, _, err := crcos.RunWithDefaultLocale(goodhostPath, args...)
	return err
}

func instance() helper {
	return &cliHelper{}
}

type cliHelper struct {
}

func (c *cliHelper) Add(req *types.AddRequest) error {
	return execute(append([]string{"add", req.IP}, req.Hosts...)...)
}

func (c *cliHelper) Remove(req *types.RemoveRequest) error {
	return execute(append([]string{"rm"}, req.Hosts...)...)
}
