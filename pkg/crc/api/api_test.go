// +build !windows

package api

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cmdConfig "github.com/code-ready/crc/cmd/crc/cmd/config"
	"github.com/code-ready/crc/pkg/crc/config"
	"github.com/code-ready/crc/pkg/crc/machine/fakemachine"
	"github.com/code-ready/crc/pkg/crc/preflight"
	"github.com/code-ready/crc/pkg/crc/version"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApi(t *testing.T) {
	dir, err := ioutil.TempDir("", "api")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	socket := filepath.Join(dir, "api.sock")
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	client := fakemachine.NewClient()
	api, err := createServerWithListener(listener, setupNewInMemoryConfig(), client)
	require.NoError(t, err)
	go func() {
		if err := api.Serve(); err != nil {
			log.Error(err)
		}
	}()

	tt := []struct {
		command       string
		clientFailing bool
		args          json.RawMessage
		expected      map[string]interface{}
	}{
		{
			command: "version",
			expected: map[string]interface{}{
				"CrcVersion":       version.GetCRCVersion(),
				"CommitSha":        version.GetCommitSha(),
				"OpenshiftVersion": version.GetBundleVersion(),
				"Success":          true,
			},
		},
		{
			command: "status",
			expected: map[string]interface{}{
				"Name":             "crc",
				"CrcStatus":        "Running",
				"OpenshiftStatus":  "Running",
				"OpenshiftVersion": "4.5.1",
				"DiskUse":          float64(10000000000),
				"DiskSize":         float64(20000000000),
				"Error":            "",
				"Success":          true,
			},
		},
		{
			command:       "status",
			clientFailing: true,
			expected: map[string]interface{}{
				"Name":             "crc",
				"CrcStatus":        "",
				"OpenshiftStatus":  "",
				"OpenshiftVersion": "",
				"DiskUse":          float64(0),
				"DiskSize":         float64(0),
				"Error":            "broken",
				"Success":          false,
			},
		},
		{
			command:       "start",
			clientFailing: true,
			args:          json.RawMessage(`{"pull-secret":"/Users/fake/pull-secret"}`),
			expected: map[string]interface{}{
				"Name":           "crc",
				"Status":         "",
				"Error":          "Incorrect arguments given: json: unknown field \"pull-secret\"",
				"KubeletStarted": false,
				"ClusterConfig": map[string]interface{}{
					"KubeConfig":    "",
					"KubeAdminPass": "",
					"ClusterAPI":    "",
					"WebConsoleURL": "",
					"ProxyConfig":   nil,
					"ClusterCACert": "",
				},
			},
		},
		{
			command: "start",
			args:    json.RawMessage(`{"pullSecretFile":"/Users/fake/pull-secret"}`),
			expected: map[string]interface{}{
				"Name":           "crc",
				"Status":         "",
				"Error":          "",
				"KubeletStarted": true,
				"ClusterConfig": map[string]interface{}{
					"ClusterCACert": "MIIDODCCAiCgAwIBAgIIRVfCKNUa1wIwDQYJ",
					"KubeConfig":    "/tmp/kubeconfig",
					"KubeAdminPass": "foobar",
					"ClusterAPI":    "https://foo.testing:6443",
					"WebConsoleURL": "https://console.foo.testing:6443",
					"ProxyConfig":   nil,
				},
			},
		},
	}
	for _, test := range tt {
		client.Failing = test.clientFailing
		client, err := net.Dial("unix", socket)
		require.NoError(t, err)

		jsonReq, err := json.Marshal(commandRequest{
			Command: test.command,
			Args:    test.args,
		})
		assert.NoError(t, err)
		_, err = client.Write(jsonReq)
		assert.NoError(t, err)

		payload := make([]byte, 1024)
		n, err := client.Read(payload)
		assert.NoError(t, err)

		var res map[string]interface{}
		assert.NoError(t, json.Unmarshal(payload[0:n], &res))
		assert.Equal(t, test.expected, res)
	}
}

func TestSetconfigApi(t *testing.T) {
	socket, cleanup := setupAPIServer(t)
	client, err := net.Dial("unix", socket)
	require.NoError(t, err)
	defer cleanup()

	jsonReq, err := json.Marshal(commandRequest{
		Command: "setconfig",
		Args:    json.RawMessage(`{"properties":{"cpus":"5"}}`),
	})
	assert.NoError(t, err)
	_, err = client.Write(jsonReq)
	assert.NoError(t, err)

	payload := make([]byte, 1024)
	n, err := client.Read(payload)
	assert.NoError(t, err)

	var setconfigRes setOrUnsetConfigResult
	assert.NoError(t, json.Unmarshal(payload[:n], &setconfigRes))
	assert.Equal(t, setOrUnsetConfigResult{
		Error:      "",
		Properties: []string{"cpus"},
	}, setconfigRes)
}

func TestGetconfigApi(t *testing.T) {
	socket, cleanup := setupAPIServer(t)
	client, err := net.Dial("unix", socket)
	require.NoError(t, err)
	defer cleanup()

	jsonReq, err := json.Marshal(commandRequest{
		Command: "getconfig",
		Args:    json.RawMessage(`{"properties":["cpus"]}`),
	})
	assert.NoError(t, err)
	_, err = client.Write(jsonReq)
	assert.NoError(t, err)

	payload := make([]byte, 1024)
	n, err := client.Read(payload)
	assert.NoError(t, err)

	var getconfigRes getConfigResult
	assert.NoError(t, json.Unmarshal(payload[:n], &getconfigRes))

	configs := make(map[string]interface{})
	configs["cpus"] = 4.0

	assert.Equal(t, getConfigResult{
		Error:   "",
		Configs: configs,
	}, getconfigRes)
}

func setupNewInMemoryConfig() config.Storage {
	storage := config.NewEmptyInMemoryStorage()
	cfg := config.New(&skipPreflights{
		storage: storage,
	})
	cmdConfig.RegisterSettings(cfg)
	preflight.RegisterSettings(cfg)

	return cfg
}

func setupAPIServer(t *testing.T) (string, func()) {
	dir, err := ioutil.TempDir("", "api")
	require.NoError(t, err)

	socket := filepath.Join(dir, "api.sock")
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	client := fakemachine.NewClient()
	api, err := createServerWithListener(listener, setupNewInMemoryConfig(), client)
	require.NoError(t, err)
	go func() {
		if err := api.Serve(); err != nil {
			log.Error(err)
		}
	}()

	return socket, func() { os.RemoveAll(dir) }
}

type skipPreflights struct {
	storage config.RawStorage
}

func (s *skipPreflights) Get(key string) interface{} {
	if strings.HasPrefix(key, "skip-") {
		return "true"
	}
	return s.storage.Get(key)
}

func (s *skipPreflights) Set(key string, value interface{}) error {
	return s.storage.Set(key, value)
}

func (s *skipPreflights) Unset(key string) error {
	return s.storage.Unset(key)
}
