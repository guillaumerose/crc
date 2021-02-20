package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/google/tcpproxy"

	"github.com/code-ready/crc/pkg/crc/api"
	"github.com/code-ready/crc/pkg/crc/constants"
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

const hostVirtualIP = "192.168.127.254"

var daemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Run the crc daemon",
	Long:   "Run the crc daemon",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var endpoints []string
		if runtime.GOOS == "windows" {
			endpoints = append(endpoints, transport.DefaultURL)
		} else {
			_ = os.Remove(constants.NetworkSocketPath)
			endpoints = append(endpoints, fmt.Sprintf("unix://%s", constants.NetworkSocketPath))
			if runtime.GOOS == "linux" {
				endpoints = append(endpoints, transport.DefaultURL)
			}
		}

		err := run(&types.Configuration{
			Debug:             false, // never log packets
			CaptureFile:       captureFile(),
			MTU:               4000, // Large packets slightly improve the performance. Less small packets.
			Subnet:            "192.168.127.0/24",
			GatewayIP:         constants.VSockGateway,
			GatewayMacAddress: "\x5A\x94\xEF\xE4\x0C\xDD",
			DNS: []types.Zone{
				{
					Name: "crc.testing.",
					Records: []types.Record{
						{
							Name: "gateway",
							IP:   net.ParseIP("192.168.127.1"),
						},
						{
							Name: "host",
							IP:   net.ParseIP(hostVirtualIP),
						},
					},
				},
			},
			Forwards: map[string]string{
				fmt.Sprintf(":%d", constants.VsockSSHPort): "192.168.127.2:22",
				":80":   "192.168.127.2:80",
				":8080": "192.168.127.2:8080",
			},
			NAT: map[string]string{
				hostVirtualIP: "127.0.0.1",
			},
		}, endpoints)
		return err
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
	if isDebugLog() {
		go func() {
			for {
				fmt.Printf("%v sent to the VM, %v received from the VM\n", units.HumanSize(float64(vn.BytesSent())), units.HumanSize(float64(vn.BytesReceived())))
				time.Sleep(5 * time.Second)
			}
		}()
	}

	go func() {
		var p tcpproxy.Proxy
		p.ListenFunc = func(a, b string) (net.Listener, error) {
			_ = os.Remove(filepath.Join(constants.CrcBaseDir, "podman.sock"))
			return net.Listen("unix", filepath.Join(constants.CrcBaseDir, "podman.sock"))
		}
		p.AddRoute(":1234", &tcpproxy.DialProxy{
			DialContext: func(ctx context.Context, network, addr string) (conn net.Conn, e error) {
				return vn.Dial("tcp", "192.168.127.2:1234")
			},
		})
		if err := p.Start(); err != nil {
			log.Fatal(err)
		}
		go func() {
			if err := p.Wait(); err != nil {
				log.Error(err)
			}
		}()
	}()

	go func() {
		if err := runDaemon(); err != nil {
			errCh <- err
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	select {
	case <-c:
		return nil
	case err := <-errCh:
		return err
	}
}

func runDaemon() error {
	// Remove if an old socket is present
	os.Remove(constants.DaemonSocketPath)
	apiServer, err := api.CreateServer(constants.DaemonSocketPath, config, newMachine())
	if err != nil {
		return err
	}
	return apiServer.Serve()
}
