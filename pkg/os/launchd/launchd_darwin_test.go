package launchd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"howett.net/plist"
)

const plistExample = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Disabled</key>
    <false/>
    <key>Label</key>
    <string>label</string>
    <key>ProgramArguments</key>
    <array>
      <string>binary</string>
      <string>arg1</string>
      <string>arg2</string>
    </array>
    <key>RunAtLoad</key>
    <false/>
    <key>StandardOutPath</key>
    <string>stdout</string>
  </dict>
</plist>`

func TestSerializeAgentConfig(t *testing.T) {
	bin, err := plist.MarshalIndent(AgentConfig{
		Label:            "label",
		StdOutFilePath:   "stdout",
		ProgramArguments: []string{"binary", "arg1", "arg2"},
	}, plist.XMLFormat, "  ")
	assert.NoError(t, err)
	assert.Equal(t, plistExample, string(bin))
}
