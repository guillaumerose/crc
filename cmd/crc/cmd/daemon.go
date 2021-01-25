package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/code-ready/crc/pkg/crc/adminhelper"
	"github.com/code-ready/crc/pkg/crc/api"
	crcConfig "github.com/code-ready/crc/pkg/crc/config"
	"github.com/code-ready/crc/pkg/crc/constants"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		// setup separate logging for daemon
		logging.CloseLogging()
		logging.InitLogrus(logging.LogLevel, constants.DaemonLogFilePath)

		go runDaemon()

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
					Name:      "apps-crc.testing.",
					DefaultIP: net.ParseIP("192.168.127.2"),
				},
				{
					Name: "crc.testing.",
					Records: []types.Record{
						{
							Name: "gateway",
							IP:   net.ParseIP("192.168.127.1"),
						},
						{
							Name: "api",
							IP:   net.ParseIP("192.168.127.2"),
						},
						{
							Name: "api-int",
							IP:   net.ParseIP("192.168.127.2"),
						},
						{
							Regexp: regexp.MustCompile("crc-(.*?)-master-0"),
							IP:     net.ParseIP("192.168.126.11"),
						},
					},
				},
			},
			Forwards: map[string]string{
				fmt.Sprintf(":%d", constants.VsockSSHPort): "192.168.127.2:22",
				":6443": "192.168.127.2:6443",
				":443":  "192.168.127.2:443",
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

	ln, err := vn.Listen("tcp", fmt.Sprintf("%s:80", configuration.GatewayIP))
	if err != nil {
		return err
	}
	go func() {
		mux := internalAPIMux()
		if err := http.Serve(ln, mux); err != nil {
			errCh <- err
		}
	}()

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
	return <-errCh
}

// This API is only exposed in the virtual network. Any process can reach it by connecting to gateway.crc.testing.
func internalAPIMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/hosts/add", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusBadRequest)
			return
		}
		var req []string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := adminhelper.AddToHostsFile("127.0.0.1", req...); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/hosts/remove", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusBadRequest)
			return
		}
		var req []string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := adminhelper.RemoveFromHostsFile(req...); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func newConfig() (crcConfig.Storage, error) {
	config, _, err := newViperConfig()
	return config, err
}

func runDaemon() {
	// Remove if an old socket is present
	os.Remove(constants.DaemonSocketPath)
	crcAPIServer, err := api.CreateAPIServer(constants.DaemonSocketPath, newConfig, newMachineWithConfig)
	if err != nil {
		logging.Fatal("Failed to launch daemon", err)
	}
	crcAPIServer.Serve()
}
