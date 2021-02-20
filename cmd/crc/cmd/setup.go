package cmd

import (
	"fmt"
	"io"
	"os"

	cmdConfig "github.com/code-ready/crc/cmd/crc/cmd/config"
	"github.com/code-ready/crc/pkg/crc/constants"
	crcErrors "github.com/code-ready/crc/pkg/crc/errors"
	"github.com/code-ready/crc/pkg/crc/input"
	"github.com/code-ready/crc/pkg/crc/preflight"
	"github.com/spf13/cobra"
)

func init() {
	setupCmd.Flags().Bool(cmdConfig.ExperimentalFeatures, false, "Allow the use of experimental features")
	addOutputFormatFlag(setupCmd)
	rootCmd.AddCommand(setupCmd)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up prerequisites",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := viper.BindFlagSet(cmd.Flags()); err != nil {
			return err
		}
		return runSetup(args)
	},
}

func runSetup(arguments []string) error {
	if config.Get(cmdConfig.ConsentTelemetry).AsString() == "" {
		fmt.Println("Podman Desktop is constantly improving and we would like to know more about usage (more details at https://developers.redhat.com/article/tool-data-collection)")
		fmt.Println("Your preference can be changed manually if desired using 'podman-machine config set consent-telemetry <yes/no>'")
		if input.PromptUserForYesOrNo("Would you like to contribute anonymous usage statistics", false) {
			if _, err := config.Set(cmdConfig.ConsentTelemetry, "yes"); err != nil {
				return err
			}
			fmt.Printf("Thanks for helping us! You can disable telemetry with the command 'crc config set %s no'.\n", cmdConfig.ConsentTelemetry)
		} else {
			if _, err := config.Set(cmdConfig.ConsentTelemetry, "no"); err != nil {
				return err
			}
			fmt.Printf("No worry, you can still enable telemetry manually with the command 'crc config set %s yes'.\n", cmdConfig.ConsentTelemetry)
		}
	}
	err := preflight.SetupHost(config)
	return render(&setupResult{
		Success: err == nil,
		Error:   crcErrors.ToSerializableError(err),
	}, os.Stdout, outputFormat)
}

type setupResult struct {
	Success bool                         `json:"success"`
	Error   *crcErrors.SerializableError `json:"error,omitempty"`
}

func (s *setupResult) prettyPrintTo(writer io.Writer) error {
	if s.Error != nil {
		return s.Error
	}
	_, err := fmt.Fprintf(writer, "Setup is complete, you can now run 'crc start%s' to start the OpenShift cluster\n", extraArguments())
	return err
}

func extraArguments() string {
	var bundle string
	if !constants.IsRelease() {
		bundle = " -b $bundlename"
	}
	return bundle
}
