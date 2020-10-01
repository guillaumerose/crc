package cmd

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/code-ready/crc/pkg/crc/constants"
	"github.com/code-ready/crc/pkg/crc/exit"
	"github.com/code-ready/crc/pkg/crc/logging"
	"github.com/code-ready/gvisor-tap-vsock/pkg/transport"
	"github.com/code-ready/gvisor-tap-vsock/pkg/types"
	"github.com/code-ready/gvisor-tap-vsock/pkg/virtualnetwork"
	"github.com/docker/go-units"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(daemonCmd)
}

var daemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Run the crc daemon",
	Long:   "Run the crc daemon",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		// setup separate logging for daemon
		logging.CloseLogging()
		logging.InitLogrus(logging.LogLevel, constants.DaemonLogFilePath)

		go runDaemon(config)

		var endpoints []string
		if runtime.GOOS == "windows" {
			endpoints = append(endpoints, transport.DefaultURL)
		} else {
			apiSock := filepath.Join(constants.CrcBaseDir, "network.sock")
			_ = os.Remove(apiSock)
			endpoints = append(endpoints, fmt.Sprintf("unix://%s", apiSock))
			if runtime.GOOS == "linux" {
				endpoints = append(endpoints, transport.DefaultURL)
			}
		}

		if err := run(&types.Configuration{
			Debug:             isDebugLog(),
			CaptureFile:       captureFile(),
			MTU:               4000,
			Subnet:            "192.168.127.0/24",
			GatewayIP:         "192.168.127.1",
			GatewayMacAddress: "\x5A\x94\xEF\xE4\x0C\xDD",
			DNSRecords: map[string]net.IP{
				"gateway.crc.testing.":              net.ParseIP("192.168.127.1"),
				"apps-crc.testing.":                 net.ParseIP("192.168.127.2"),
				"etcd-0.crc.testing.":               net.ParseIP("192.168.127.2"),
				"api.crc.testing.":                  net.ParseIP("192.168.127.2"),
				"api-int.crc.testing.":              net.ParseIP("192.168.127.2"),
				"oauth-openshift.apps-crc.testing.": net.ParseIP("192.168.127.2"),
				"crc-zqfk6-master-0.crc.testing.":   net.ParseIP("192.168.126.11"),
			},
			Forwards: map[string]string{
				":2222": "192.168.127.2:22",
				":6443": "192.168.127.2:6443",
				":443":  "192.168.127.2:443",
			},
		}, endpoints); err != nil {
			exit.WithMessage(1, err.Error())
		}
	},
}

func captureFile() string {
	if !isDebugLog() {
		return ""
	}
	return filepath.Join(constants.CrcBaseDir, "capture.pcap")
}

func run(configuration *types.Configuration, endpoints []string) error {
	vn, err := virtualnetwork.New(configuration)
	if err != nil {
		return err
	}
	log.Info("waiting for clients...")
	errCh := make(chan error)

	for _, endpoint := range endpoints {
		log.Infof("listening %s", endpoint)
		ln, err := transport.Listen(endpoint)
		if err != nil {
			return errors.Wrap(err, "cannot listen")
		}

		go func() {
			if err := http.Serve(ln, vn.Mux()); err != nil {
				errCh <- err
			}
		}()
	}
	go func() {
		for {
			fmt.Printf("%v sent to the VM, %v received from the VM\n", units.HumanSize(float64(vn.BytesSent())), units.HumanSize(float64(vn.BytesReceived())))
			time.Sleep(5 * time.Second)
		}
	}()
	return <-errCh
}
