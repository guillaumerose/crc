package api

import (
	"bytes"
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"sync"
	"time"

	"github.com/code-ready/crc/cmd/crc/cmd/config"
	"github.com/code-ready/crc/pkg/crc/cluster"
	crcConfig "github.com/code-ready/crc/pkg/crc/config"
	"github.com/code-ready/crc/pkg/crc/errors"
	"github.com/code-ready/crc/pkg/crc/logging"
	"github.com/code-ready/crc/pkg/crc/machine"
	"github.com/code-ready/crc/pkg/crc/preflight"
	"github.com/code-ready/crc/pkg/crc/version"
	"golang.org/x/sync/semaphore"
)

type Handler struct {
	MachineClient AdaptedClient
	Config        crcConfig.Storage

	StartLock      *semaphore.Weighted
	StopDeleteLock *semaphore.Weighted

	startCancelFunc     context.CancelFunc
	startCancelFuncLock sync.Mutex
}

func (h *Handler) Status() string {
	clusterStatus := h.MachineClient.Status()
	return encodeStructToJSON(clusterStatus)
}

func (h *Handler) Stop() string {
	cleanup, err := h.lockAndCancelStart()
	if err != nil {
		startErr := &Result{
			Name:  h.MachineClient.GetName(),
			Error: err.Error(),
		}
		return encodeStructToJSON(startErr)
	}
	defer cleanup()

	commandResult := h.MachineClient.Stop()
	return encodeStructToJSON(commandResult)
}

func (h *Handler) Start(args json.RawMessage) string {
	if !h.StartLock.TryAcquire(int64(1)) {
		startErr := &StartResult{
			Name:  h.MachineClient.GetName(),
			Error: "Start already in progress",
		}
		return encodeStructToJSON(startErr)
	}
	defer h.StartLock.Release(int64(1))

	ctx, cancelFunc := context.WithCancel(context.Background())
	h.startCancelFuncLock.Lock()
	h.startCancelFunc = cancelFunc
	h.startCancelFuncLock.Unlock()
	defer func() {
		h.startCancelFuncLock.Lock()
		h.startCancelFunc = nil
		h.startCancelFuncLock.Unlock()
	}()

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

	startConfig := getStartConfig(h.Config, parsedArgs)
	status := h.MachineClient.Start(ctx, startConfig)
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
	cleanup, err := h.lockAndCancelStart()
	if err != nil {
		startErr := &Result{
			Name:  h.MachineClient.GetName(),
			Error: err.Error(),
		}
		return encodeStructToJSON(startErr)
	}
	defer cleanup()

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
		err := Result{
			Success: false,
			Error:   "Failed while encoding JSON to string",
		}
		s, _ := json.Marshal(err)
		return string(s)
	}
	return string(s)
}

func encodeErrorToJSON(errMsg string) string {
	err := Result{
		Success: false,
		Error:   errMsg,
	}
	return encodeStructToJSON(err)
}

func (h *Handler) lockAndCancelStart() (func(), error) {
	if !h.StopDeleteLock.TryAcquire(int64(1)) {
		return nil, goerrors.New("stop or delete already in progress")
	}

	h.startCancelFuncLock.Lock()
	if h.startCancelFunc != nil {
		h.startCancelFunc()
	}
	h.startCancelFuncLock.Unlock()

	// Wait for start to finish and block start action until stop finished
	timeout, cancelFunc := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelFunc()
	if err := h.StartLock.Acquire(timeout, int64(1)); err != nil {
		h.StopDeleteLock.Release(int64(1))
		return nil, goerrors.New("startup sequence didn't abort in less than 15s")
	}

	return func() {
		h.StartLock.Release(int64(1))
		h.StopDeleteLock.Release(int64(1))
	}, nil
}
