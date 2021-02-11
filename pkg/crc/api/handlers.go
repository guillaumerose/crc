package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/code-ready/crc/cmd/crc/cmd/config"
	"github.com/code-ready/crc/pkg/crc/cluster"
	crcConfig "github.com/code-ready/crc/pkg/crc/config"
	crcErrors "github.com/code-ready/crc/pkg/crc/errors"
	"github.com/code-ready/crc/pkg/crc/logging"
	"github.com/code-ready/crc/pkg/crc/machine"
	"github.com/code-ready/crc/pkg/crc/machine/bundle"
	"github.com/code-ready/crc/pkg/crc/preflight"
	"github.com/code-ready/crc/pkg/crc/version"
)

type Handler struct {
	MachineClient AdaptedClient
	Config        crcConfig.Storage
}

func (h *Handler) Status() string {
	clusterStatus := h.MachineClient.Status()
	return encodeStructToJSON(clusterStatus)
}

func (h *Handler) Stop() string {
	commandResult := h.MachineClient.Stop()
	return encodeStructToJSON(commandResult)
}

func (h *Handler) Start(args json.RawMessage) string {
	var parsedArgs startArgs
	var err error
	if args != nil {
		parsedArgs, err = parseStartArgs(args)
		if err != nil {
			startErr := &StartResult{
				Name:  h.MachineClient.GetName(),
				Error: fmt.Sprintf("Incorrect arguments given: %s", err.Error()),
			}
			return encodeStructToJSON(startErr)
		}
	}
	if err := preflight.StartPreflightChecks(h.Config); err != nil {
		startErr := &StartResult{
			Name:  h.MachineClient.GetName(),
			Error: err.Error(),
		}
		return encodeStructToJSON(startErr)
	}

	startConfig, err := getStartConfig(h.Config, parsedArgs)
	if err != nil {
		return encodeStructToJSON(&StartResult{
			Name:  h.MachineClient.GetName(),
			Error: fmt.Sprintf("Cannot find a bundle: %s", err.Error()),
		})
	}
	status := h.MachineClient.Start(*startConfig)
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

func getStartConfig(cfg crcConfig.Storage, args startArgs) (*machine.StartConfig, error) {
	bundles, err := bundle.List()
	if err != nil {
		return nil, err
	}
	if len(bundles) == 0 {
		return nil, errors.New("no bundle installed")
	}
	return &machine.StartConfig{
		BundlePath: bundles[0].Name,
		Memory:     cfg.Get(config.Memory).AsInt(),
		CPUs:       cfg.Get(config.CPUs).AsInt(),
		NameServer: cfg.Get(config.NameServer).AsString(),
		PullSecret: cluster.NewNonInteractivePullSecretLoader(cfg, args.PullSecretFile),
	}, nil
}

type VersionResult struct {
	CrcVersion       string
	CommitSha        string
	OpenshiftVersion string
	Success          bool
}

func (h *Handler) GetVersion() string {
	v := &VersionResult{
		CrcVersion:       version.GetCRCVersion(),
		CommitSha:        version.GetCommitSha(),
		OpenshiftVersion: version.GetBundleVersion(),
		Success:          true,
	}
	return encodeStructToJSON(v)
}

func (h *Handler) Delete() string {
	r := h.MachineClient.Delete()
	return encodeStructToJSON(r)
}

func (h *Handler) GetWebconsoleInfo() string {
	r := h.MachineClient.GetConsoleURL()
	return encodeStructToJSON(r)
}

func (h *Handler) SetConfig(args json.RawMessage) string {
	setConfigResult := setOrUnsetConfigResult{}
	if args == nil {
		setConfigResult.Error = "No config keys provided"
		return encodeStructToJSON(setConfigResult)
	}

	var multiError = crcErrors.MultiError{}
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
		_, err := h.Config.Set(k, v)
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

func (h *Handler) UnsetConfig(args json.RawMessage) string {
	unsetConfigResult := setOrUnsetConfigResult{}
	if args == nil {
		unsetConfigResult.Error = "No config keys provided"
		return encodeStructToJSON(unsetConfigResult)
	}

	var multiError = crcErrors.MultiError{}
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
		_, err := h.Config.Unset(key)
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

func (h *Handler) GetConfig(args json.RawMessage) string {
	configResult := getConfigResult{}
	if args == nil {
		allConfigs := h.Config.AllConfigs()
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
		v := h.Config.Get(key)
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
