package launchd

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTemplate(t *testing.T) {
	bin, err := templatize(AgentConfig{
		Label:          "label",
		BinaryPath:     "binary",
		StdOutFilePath: "stdout",
		Args:           []string{"arg1", "arg2"},
	})
	assert.NoError(t, err)
	assert.Equal(t, `<?xml version='1.0' encoding='UTF-8'?>
	<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
	<plist version='1.0'>
		<dict>
			<key>Label</key>
			<string>label</string>
			<key>ProgramArguments</key>
			<array>
				<string>binary</string>
			
				<string>arg1</string>
			
				<string>arg2</string>
			
			</array>
			<key>StandardOutPath</key>
			<string>stdout</string>
			<key>Disabled</key>
			<false/>
			<key>RunAtLoad</key>
			<true/>
		</dict>
	</plist>`, string(bin))
}
