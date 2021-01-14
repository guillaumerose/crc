package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/code-ready/crc/cmd/crc/cmd/config"
	"github.com/code-ready/crc/pkg/crc/cluster"
	crcConfig "github.com/code-ready/crc/pkg/crc/config"
	"github.com/code-ready/crc/pkg/crc/errors"
	"github.com/code-ready/crc/pkg/crc/logging"
	"github.com/code-ready/crc/pkg/crc/machine"
	"github.com/code-ready/crc/pkg/crc/preflight"
	"github.com/code-ready/crc/pkg/crc/version"
)

type Handler struct {
	MachineClient AdaptedClient
	Config        crcConfig.Storage
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	var result string
	cmd := strings.TrimPrefix(request.URL.Path, "/")
	switch cmd {
	case "start":
		result = handler.Start(body)
	case "stop":
		result = handler.Stop()
	case "status":
		result = handler.Status()
	case "delete":
		result = handler.Delete()
	case "version":
		result = handler.GetVersion()
	case "setconfig":
		result = handler.SetConfig(body)
	case "unsetconfig":
		result = handler.UnsetConfig(body)
	case "getconfig":
		result = handler.GetConfig(body)
	case "webconsoleurl":
		result = handler.GetWebconsoleInfo()
	default:
		result = encodeErrorToJSON(fmt.Sprintf("Unknown command supplied: %s", cmd))
	}
	_, _ = writer.Write([]byte(result))
}

func (handler *Handler) Status() string {
	clusterStatus := handler.MachineClient.Status()
	return encodeStructToJSON(clusterStatus)
}

func (handler *Handler) Stop() string {
	commandResult := handler.MachineClient.Stop()
	return encodeStructToJSON(commandResult)
}

func (handler *Handler) Start(args json.RawMessage) string {
	var parsedArgs startArgs
	var err error
	if args != nil {
		parsedArgs, err = parseStartArgs(args)
		if err != nil {
			startErr := &StartResult{
				Name:  handler.MachineClient.GetName(),
				Error: fmt.Sprintf("Incorrect arguments given: %s", err.Error()),
			}
			return encodeStructToJSON(startErr)
		}
	}
	if err := preflight.StartPreflightChecks(handler.Config); err != nil {
		startErr := &StartResult{
			Name:  handler.MachineClient.GetName(),
			Error: err.Error(),
		}
		return encodeStructToJSON(startErr)
	}

	startConfig := getStartConfig(handler.Config, parsedArgs)
	status := handler.MachineClient.Start(startConfig)
	return encodeStructToJSON(status)
}

func parseStartArgs(args json.RawMessage) (startArgs, error) {
	var parsedArgs startArgs
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&parsedArgs); err != nil {
		return startArgs{}, err
	}
	return parsedArgs, nil
}

func getStartConfig(cfg crcConfig.Storage, args startArgs) machine.StartConfig {
	return machine.StartConfig{
		BundlePath: cfg.Get(config.Bundle).AsString(),
		Memory:     cfg.Get(config.Memory).AsInt(),
		CPUs:       cfg.Get(config.CPUs).AsInt(),
		NameServer: cfg.Get(config.NameServer).AsString(),
		PullSecret: cluster.NewNonInteractivePullSecretLoader(cfg, args.PullSecretFile),
	}
}

type VersionResult struct {
	CrcVersion       string
	CommitSha        string
	OpenshiftVersion string
	Success          bool
}

func (handler *Handler) GetVersion() string {
	v := &VersionResult{
		CrcVersion:       version.GetCRCVersion(),
		CommitSha:        version.GetCommitSha(),
		OpenshiftVersion: version.GetBundleVersion(),
		Success:          true,
	}
	return encodeStructToJSON(v)
}

func (handler *Handler) Delete() string {
	r := handler.MachineClient.Delete()
	return encodeStructToJSON(r)
}

func (handler *Handler) GetWebconsoleInfo() string {
	r := handler.MachineClient.GetConsoleURL()
	return encodeStructToJSON(r)
}

func (handler *Handler) SetConfig(args json.RawMessage) string {
	setConfigResult := setOrUnsetConfigResult{}
	if args == nil {
		setConfigResult.Error = "No config keys provided"
		return encodeStructToJSON(setConfigResult)
	}

	var multiError = errors.MultiError{}
	var a = make(map[string]interface{})

	err := json.Unmarshal(args, &a)
	if err != nil {
		setConfigResult.Error = fmt.Sprintf("%v", err)
		return encodeStructToJSON(setConfigResult)
	}

	configs := a["properties"].(map[string]interface{})

	// successProps slice contains the properties that were successfully set
	var successProps []string

	for k, v := range configs {
		_, err := handler.Config.Set(k, v)
		if err != nil {
			multiError.Collect(err)
			continue
		}
		successProps = append(successProps, k)
	}

	if len(multiError.Errors) != 0 {
		setConfigResult.Error = fmt.Sprintf("%v", multiError)
	}

	setConfigResult.Properties = successProps
	return encodeStructToJSON(setConfigResult)
}

func (handler *Handler) UnsetConfig(args json.RawMessage) string {
	unsetConfigResult := setOrUnsetConfigResult{}
	if args == nil {
		unsetConfigResult.Error = "No config keys provided"
		return encodeStructToJSON(unsetConfigResult)
	}

	var multiError = errors.MultiError{}
	var keys = make(map[string][]string)

	err := json.Unmarshal(args, &keys)
	if err != nil {
		unsetConfigResult.Error = fmt.Sprintf("%v", err)
		return encodeStructToJSON(unsetConfigResult)
	}

	// successProps slice contains the properties that were successfully unset
	var successProps []string

	keysToUnset := keys["properties"]
	for _, key := range keysToUnset {
		_, err := handler.Config.Unset(key)
		if err != nil {
			multiError.Collect(err)
			continue
		}
		successProps = append(successProps, key)
	}
	if len(multiError.Errors) != 0 {
		unsetConfigResult.Error = fmt.Sprintf("%v", multiError)
	}
	unsetConfigResult.Properties = successProps
	return encodeStructToJSON(unsetConfigResult)
}

func (handler *Handler) GetConfig(args json.RawMessage) string {
	configResult := getConfigResult{}
	if args == nil {
		allConfigs := handler.Config.AllConfigs()
		configResult.Error = ""
		configResult.Configs = make(map[string]interface{})
		for k, v := range allConfigs {
			configResult.Configs[k] = v.Value
		}
		return encodeStructToJSON(configResult)
	}

	var a = make(map[string][]string)

	err := json.Unmarshal(args, &a)
	if err != nil {
		configResult.Error = fmt.Sprintf("%v", err)
		return encodeStructToJSON(configResult)
	}

	keys := a["properties"]

	var configs = make(map[string]interface{})

	for _, key := range keys {
		v := handler.Config.Get(key)
		if v.Invalid {
			continue
		}
		configs[key] = v.Value
	}
	if len(configs) == 0 {
		configResult.Error = "Unable to get configs"
		configResult.Configs = nil
	} else {
		configResult.Error = ""
		configResult.Configs = configs
	}
	return encodeStructToJSON(configResult)
}

func encodeStructToJSON(v interface{}) string {
	s, err := json.Marshal(v)
	if err != nil {
		logging.Error(err.Error())
		err := commandError{
			Err: "Failed while encoding JSON to string",
		}
		s, _ := json.Marshal(err)
		return string(s)
	}
	return string(s)
}

func encodeErrorToJSON(errMsg string) string {
	err := commandError{
		Err: errMsg,
	}
	return encodeStructToJSON(err)
}
