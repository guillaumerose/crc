package config

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/code-ready/crc/pkg/crc/config"
	"github.com/code-ready/crc/pkg/crc/constants"
	"github.com/code-ready/crc/pkg/crc/network"
	"github.com/spf13/cobra"
)

const (
	CPUs                    = "cpus"
	Memory                  = "memory"
	DiskSize                = "disk-size"
	NameServer              = "nameserver"
	PullSecretFile          = "pull-secret-file"
	DisableUpdateCheck      = "disable-update-check"
	ExperimentalFeatures    = "enable-experimental-features"
	NetworkMode             = "network-mode"
	HTTPProxy               = "http-proxy"
	HTTPSProxy              = "https-proxy"
	NoProxy                 = "no-proxy"
	ProxyCAFile             = "proxy-ca-file"
	ConsentTelemetry        = "consent-telemetry"
	EnableClusterMonitoring = "enable-cluster-monitoring"
)

func RegisterSettings(cfg *config.Config) {
	// Start command settings in config
	cfg.AddSetting(CPUs, constants.DefaultCPUs, config.ValidateCPUs, config.RequiresRestartMsg)
	cfg.AddSetting(Memory, constants.DefaultMemory, config.ValidateMemory, config.RequiresRestartMsg)
	cfg.AddSetting(DiskSize, constants.DefaultDiskSize, config.ValidateDiskSize, config.RequiresRestartMsg)
	cfg.AddSetting(NameServer, "", config.ValidateIPAddress, config.SuccessfullyApplied)
	cfg.AddSetting(PullSecretFile, "", config.ValidatePath, config.SuccessfullyApplied)
	cfg.AddSetting(DisableUpdateCheck, false, config.ValidateBool, config.SuccessfullyApplied)
	cfg.AddSetting(ExperimentalFeatures, false, config.ValidateBool, config.SuccessfullyApplied)
	cfg.AddSetting(NetworkMode, string(network.DefaultMode), network.ValidateMode, network.SuccessfullyAppliedMode)
	// Proxy Configuration
	cfg.AddSetting(HTTPProxy, "", config.ValidateURI, config.SuccessfullyApplied)
	cfg.AddSetting(HTTPSProxy, "", config.ValidateURI, config.SuccessfullyApplied)
	cfg.AddSetting(NoProxy, "", config.ValidateNoProxy, config.SuccessfullyApplied)
	cfg.AddSetting(ProxyCAFile, "", config.ValidatePath, config.SuccessfullyApplied)

	cfg.AddSetting(EnableClusterMonitoring, false, config.ValidateBool, config.SuccessfullyApplied)

	// Telemeter Configuration
	cfg.AddSetting(ConsentTelemetry, "", config.ValidateYesNo, config.SuccessfullyApplied)
}

func isPreflightKey(key string) bool {
	return strings.HasPrefix(key, "skip-")
}

// less is used to sort the config keys. We want to sort first the regular keys, and
// then the keys related to preflight starting with a skip- prefix.
func less(lhsKey, rhsKey string) bool {
	if isPreflightKey(lhsKey) {
		if isPreflightKey(rhsKey) {
			// ignore skip prefix
			return lhsKey[4:] < rhsKey[4:]
		}
		// lhs is preflight, rhs is not preflight
		return false
	}

	if isPreflightKey(rhsKey) {
		// lhs is not preflight, rhs is preflight
		return true
	}

	// lhs is not preflight, rhs is not preflight
	return lhsKey < rhsKey
}

func configurableFields(config *config.Config) string {
	var fields []string
	var buf bytes.Buffer
	writer := tabwriter.NewWriter(&buf, 0, 8, 1, ' ', tabwriter.TabIndent)
	for _, keyAndValueType := range keysAndValueType(config) {
		fmt.Fprintln(writer, keyAndValueType)
	}
	writer.Flush()
	keys := strings.Split(buf.String(), "\n")
	sort.Slice(keys, func(i, j int) bool {
		return less(keys[i], keys[j])
	})
	for _, key := range keys {
		if key == "" {
			continue
		}
		fields = append(fields, " * "+key)
	}
	return strings.Join(fields, "\n")
}
func keysAndValueType(config *config.Config) []string {
	var keyAndValueType []string
	for key, value := range config.AllConfigs() {
		var valueType string
		switch value.Value.(type) {
		case int:
			valueType = "Number"
		case string:
			valueType = "String"
		case bool:
			valueType = "true/false"
		default:
			valueType = fmt.Sprintf("%T", value.Value)
		}
		keyAndValueType = append(keyAndValueType, fmt.Sprintf("%s\t%s", key, valueType))
	}
	return keyAndValueType
}

func GetConfigCmd(config *config.Config) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config SUBCOMMAND [flags]",
		Short: "Modify crc configuration",
		Long: `Modifies crc configuration properties.
Properties: ` + "\n\n" + configurableFields(config),
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	configCmd.AddCommand(configGetCmd(config))
	configCmd.AddCommand(configSetCmd(config))
	configCmd.AddCommand(configUnsetCmd(config))
	configCmd.AddCommand(configViewCmd(config))
	return configCmd
}
