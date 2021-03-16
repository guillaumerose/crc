package api

import (
	"encoding/json"
	"net"
)

type Server struct {
	handler  RequestHandler
	listener net.Listener
}

type RequestHandler interface {
	Start(json.RawMessage) string
	Stop() string
	Status() string
	Delete() string
	GetVersion() string
	SetConfig(json.RawMessage) string
	UnsetConfig(json.RawMessage) string
	GetConfig(json.RawMessage) string
	GetWebconsoleInfo() string
	Logs() string
}

// commandRequest struct is used to decode the json request from tray
type commandRequest struct {
	Command string          `json:"command"`
	Args    json.RawMessage `json:"args,omitempty"`
}

// setOrUnsetConfigResult struct is used to return the result of
// setconfig/unsetconfig command
type setOrUnsetConfigResult struct {
	Error      string
	Properties []string
}

// getConfigResult struct is used to return the result of getconfig command
type getConfigResult struct {
	Error   string
	Configs map[string]interface{}
}

// startArgs is used to get the pull secret file path as argument for start handler
type startArgs struct {
	PullSecretFile string `json:"pullSecretFile"`
}

type loggerResult struct {
	Success  bool
	Messages []string
}
