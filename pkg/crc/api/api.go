package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"

	cmdConfig "github.com/code-ready/crc/cmd/crc/cmd/config"
	"github.com/code-ready/crc/pkg/crc/config"
	"github.com/code-ready/crc/pkg/crc/logging"
	"github.com/code-ready/crc/pkg/crc/machine"
)

func CreateAPIServer(socketPath string, config config.Storage) (CrcAPIServer, error) {
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		logging.Error("Failed to create socket: ", err.Error())
		return CrcAPIServer{}, err
	}
	return createAPIServerWithListener(listener, machine.NewClient(config.Get(cmdConfig.ExperimentalFeatures).AsBool()), config)
}

func createAPIServerWithListener(listener net.Listener, client machine.Client, config config.Storage) (CrcAPIServer, error) {
	apiServer := CrcAPIServer{
		client:                 client,
		config:                 config,
		listener:               listener,
		clusterOpsRequestsChan: make(chan clusterOpsRequest, 10),
		handlers: map[string]handlerFunc{
			"start":         startHandler,
			"stop":          stopHandler,
			"status":        statusHandler,
			"delete":        deleteHandler,
			"version":       versionHandler,
			"setconfig":     setConfigHandler,
			"unsetconfig":   unsetConfigHandler,
			"getconfig":     getConfigHandler,
			"webconsoleurl": webconsoleURLHandler,
		},
	}
	return apiServer, nil
}

func (api CrcAPIServer) Serve() {
	go api.handleClusterOperations() // go routine that handles start, stop and delete calls
	for {
		conn, err := api.listener.Accept()
		if err != nil {
			logging.Error("Error establishing communication: ", err.Error())
			continue
		}
		api.handleConnections(conn) // handle version, status, webconsole, etc. requests
	}
}

func (api CrcAPIServer) handleClusterOperations() {
	for req := range api.clusterOpsRequestsChan {
		api.handleRequest(req.command, req.socket)
	}
}

func (api CrcAPIServer) handleRequest(req commandRequest, conn net.Conn) {
	defer conn.Close()
	var result string
	if handler, ok := api.handlers[req.Command]; ok {
		result = handler(api.client, api.config, req.Args)
	} else {
		result = encodeErrorToJSON(fmt.Sprintf("Unknown command supplied: %s", req.Command))
	}
	writeStringToSocket(conn, result)
}

func (api CrcAPIServer) handleConnections(conn net.Conn) {
	inBuffer := make([]byte, 1024)
	var req commandRequest
	numBytes, err := conn.Read(inBuffer)
	if err != nil || numBytes == 0 || numBytes == cap(inBuffer) {
		logging.Error("Error reading from socket")
		return
	}
	logging.Debug("Received Request:", string(inBuffer[0:numBytes]))
	err = json.Unmarshal(inBuffer[0:numBytes], &req)
	if err != nil {
		logging.Error("Error decoding request: ", err.Error())
		return
	}
	// start, stop and delete are slow operations, and change the VM state so they have to run sequentially.
	// We don't want other operations querying the status of the VM to be blocked by these,
	// so they are treated by a dedicated go routine
	if req.Command == "start" || req.Command == "stop" || req.Command == "delete" {
		// queue new request to channel
		r := clusterOpsRequest{
			command: req,
			socket:  conn,
		}
		if !addRequestToChannel(r, api.clusterOpsRequestsChan) {
			defer conn.Close()
			logging.Error("Channel capacity reached, unable to add new request")
			errMsg := encodeErrorToJSON("Sockets channel capacity reached, unable to add new request")
			writeStringToSocket(conn, errMsg)
			return
		}
	} else {
		go api.handleRequest(req, conn)
	}
}

func writeStringToSocket(socket net.Conn, msg string) {
	var outBuffer bytes.Buffer
	_, err := outBuffer.WriteString(msg)
	if err != nil {
		logging.Error("Failed writing string to buffer", err.Error())
		return
	}
	_, err = socket.Write(outBuffer.Bytes())
	if err != nil {
		logging.Error("Failed writing string to socket", err.Error())
		return
	}
}

func addRequestToChannel(req clusterOpsRequest, requestsChan chan clusterOpsRequest) bool {
	select {
	case requestsChan <- req:
		return true
	default:
		return false
	}
}
