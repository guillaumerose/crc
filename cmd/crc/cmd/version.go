package cmd

import (
	"fmt"
	"io"
	"os"

	cmdConfig "github.com/code-ready/crc/cmd/crc/cmd/config"
	"github.com/code-ready/crc/pkg/crc/constants"
	crcversion "github.com/code-ready/crc/pkg/crc/version"
	"github.com/spf13/cobra"
)

func init() {
	addOutputFormatFlag(versionCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  "Print version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPrintVersion(os.Stdout, defaultVersion(), outputFormat)
	},
}

func runPrintVersion(writer io.Writer, version *version, outputFormat string) error {
	checkIfNewVersionAvailable(config.Get(cmdConfig.DisableUpdateCheck).AsBool())
	return render(version, writer, outputFormat)
}

type version struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Embedded bool   `json:"embedded"`
}

func defaultVersion() *version {
	return &version{
		Version:  crcversion.GetCRCVersion(),
		Commit:   crcversion.GetCommitSha(),
		Embedded: constants.BundleEmbedded(),
	}
}

func (v *version) prettyPrintTo(writer io.Writer) error {
	for _, line := range v.lines() {
		if _, err := fmt.Fprint(writer, line); err != nil {
			return err
		}
	}
	return nil
}

func (v *version) lines() []string {
	return []string{
		fmt.Sprintf("podman-machine version: %s+%s\n", v.Version, v.Commit),
	}
}
