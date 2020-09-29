package launchd

import (
	"fmt"
	"io/ioutil"
	goos "os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/code-ready/crc/pkg/crc/constants"
	"github.com/code-ready/crc/pkg/os"
	"howett.net/plist"
)

// AgentConfig is struct to contain configuration for agent plist file
type AgentConfig struct {
	Label            string   `plist:"Label"`
	StdOutFilePath   string   `plist:"StandardOutPath"`
	ProgramArguments []string `plist:"ProgramArguments"`
	Disabled         bool     `plist:"Disabled"`
	RunAtLoad        bool     `plist:"RunAtLoad"`
}

var (
	launchAgentsDir = filepath.Join(constants.GetHomeDir(), "Library", "LaunchAgents")
)

func ensureLaunchAgentsDirExists() error {
	if err := goos.MkdirAll(launchAgentsDir, 0700); err != nil {
		return err
	}
	return nil
}

func getPlistPath(label string) string {
	plistName := fmt.Sprintf("%s.plist", label)

	return filepath.Join(launchAgentsDir, plistName)
}

// CreatePlist creates a launchd agent plist config file
func CreatePlist(config AgentConfig) error {
	if err := ensureLaunchAgentsDirExists(); err != nil {
		return err
	}

	plistContent, err := plist.MarshalIndent(config, plist.XMLFormat, "  ")
	if err != nil {
		return err
	}
	// #nosec G306
	err = ioutil.WriteFile(getPlistPath(config.Label), plistContent, 0644)
	return err
}

func PlistExists(label string) bool {
	return os.FileExists(getPlistPath(label))
}

// LoadPlist loads a launchd agents' plist file
func LoadPlist(label string) error {
	return exec.Command("launchctl", "load", getPlistPath(label)).Run() // #nosec G204
}

// UnloadPlist Unloads a launchd agent's service
func UnloadPlist(label string) error {
	return exec.Command("launchctl", "unload", getPlistPath(label)).Run() // #nosec G204
}

// RemovePlist removes a launchd agent plist config file
func RemovePlist(label string) error {
	if _, err := goos.Stat(getPlistPath(label)); !goos.IsNotExist(err) {
		return goos.Remove(getPlistPath(label))
	}
	return nil
}

// StartAgent starts a launchd agent
func StartAgent(label string) error {
	return exec.Command("launchctl", "start", label).Run() // #nosec G204
}

// StopAgent stops a launchd agent
func StopAgent(label string) error {
	return exec.Command("launchctl", "stop", label).Run() // #nosec G204
}

// RestartAgent restarts a launchd agent
func RestartAgent(label string) error {
	err := StopAgent(label)
	if err != nil {
		return err
	}
	return StartAgent(label)
}

// AgentRunning checks if a launchd service is running
func AgentRunning(label string) bool {
	// This command return a PID if the process
	// is running, otherwise returns "-" or empty
	// output if the agent is not loaded in launchd
	launchctlListCommand := `launchctl list | grep %s | awk '{print $1}'`
	cmd := fmt.Sprintf(launchctlListCommand, label)
	out, err := exec.Command("bash", "-c", cmd).Output() // #nosec G204
	if err != nil {
		return false
	}
	// match PID
	if match, err := regexp.MatchString(`^\d+$`, strings.TrimSpace(string(out))); err == nil && match {
		return true
	}
	return false
}

// Remove removes the agent from launchd
func Remove(label string) error {
	return exec.Command("launchctl", "remove", label).Run() // #nosec G204
}
