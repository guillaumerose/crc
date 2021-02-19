package transport

import (
	"net"
	"net/url"

	"github.com/pkg/errors"
)

func Dial(endpoint string) (net.Conn, string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, "", err
	}
	switch parsed.Scheme {
	case "unix":
		conn, err := net.Dial("unix", parsed.Path)
		return conn, "/connect", err
	default:
		return nil, "", errors.New("unexpected scheme")
	}
}
