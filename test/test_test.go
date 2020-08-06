package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os/exec"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

var _ = Describe("basic commands", func() {
	Describe("version", func() {
		It("show version", func() {
			Expect(RunCRCOrDie("version")).To(ContainSubstring("CodeReady Containers version"))
		})

		It("show version as json format", func() {
			Skip("broken")

			raw := RunCRCOrDie("version", "-o", "json")

			var parsed map[string]string
			Expect(json.Unmarshal([]byte(raw), &parsed)).To(Not(HaveOccurred()))

			Expect(parsed).To(Equal(map[string]string{
				"version": "1.0",
			}))
		})
	})

	Describe("health story", func() {
		It("should setup crc", func() {
			Skip("too long for demo")

			RunCRCOrDie("setup")
		})

		It("should start crc virtual machine", func() {
			Skip("too long for demo")

			NewCRCCommand("start").WithTimeout(time.After(10 * time.Minute)).ExecOrDie()
		})

		It("should be ready", func() {
			By("using crc command")
			Eventually(func() (string, error) {
				return RunCRC("status")
			}, 2*time.Minute, 5*time.Second).Should(MatchRegexp(`.*Running \(v\d+\.\d+\.\d+.*\).*`))

			By("looking at node status")
			Eventually(func() (string, error) {
				return RunOC("get", "nodes")
			}, 2*time.Minute, 5*time.Second).Should(And(ContainSubstring("Ready"), Not(ContainSubstring("Not ready"))))
		})

		It("should be able to create a new project and an app", func() {
			projectName := fmt.Sprintf("testproj%d", rand.Int())
			defer func() {
				RunOCOrDie("delete", "project", projectName)
			}()
			Expect(RunOCOrDie("new-project", projectName)).To(Or(
				ContainSubstring(fmt.Sprintf(`Now using project "%s" on server "https://api.crc.testing:6443".`, projectName)),
				ContainSubstring(fmt.Sprintf(`Already on project "%s" on server "https://api.crc.testing:6443".`, projectName))))
			Expect(RunOCOrDie("new-app", "httpd-example")).To(ContainSubstring(`service "httpd-example" created`))
		})
	})
})

// CRCBuilder is used to build, customize and execute a kubectl Command.
// Add more functions to customize the builder as needed.
type CRCBuilder struct {
	cmd     *exec.Cmd
	timeout <-chan time.Time
}

// NewCRCCommand returns a CRCBuilder for running kubectl.
func NewCRCCommand(args ...string) *CRCBuilder {
	cmd := exec.Command("crc", args...)
	return &CRCBuilder{
		cmd: cmd,
	}
}

// WithEnv sets the given environment and returns itself.
func (b *CRCBuilder) WithEnv(env []string) *CRCBuilder {
	b.cmd.Env = env
	return b
}

// WithTimeout sets the given timeout and returns itself.
func (b *CRCBuilder) WithTimeout(t <-chan time.Time) *CRCBuilder {
	b.timeout = t
	return b
}

// WithStdinData sets the given data to stdin and returns itself.
func (b CRCBuilder) WithStdinData(data string) *CRCBuilder {
	b.cmd.Stdin = strings.NewReader(data)
	return &b
}

// WithStdinReader sets the given reader and returns itself.
func (b CRCBuilder) WithStdinReader(reader io.Reader) *CRCBuilder {
	b.cmd.Stdin = reader
	return &b
}

// ExecOrDie runs the kubectl executable or dies if error occurs.
func (b CRCBuilder) ExecOrDie() string {
	str, err := b.Exec()
	Expect(err).To(Not(HaveOccurred()))
	return str
}

// Exec runs the kubectl executable.
func (b CRCBuilder) Exec() (string, error) {
	stdout, _, err := b.ExecWithFullOutput()
	return stdout, err
}

// ExecWithFullOutput runs the kubectl executable, and returns the stdout and stderr.
func (b CRCBuilder) ExecWithFullOutput() (string, string, error) {
	return Exec(b.cmd, b.timeout)
}

func Exec(cmd *exec.Cmd, timeout <-chan time.Time) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	logrus.Infof("Running '%s %s'", cmd.Path, strings.Join(cmd.Args[1:], " ")) // skip arg[0] as it is printed separately
	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("error starting %v:\nCommand stdout:\n%v\nstderr:\n%v\nerror:\n%v", cmd, cmd.Stdout, cmd.Stderr, err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Wait()
	}()
	select {
	case err := <-errCh:
		if err != nil {
			var rc = 127
			if ee, ok := err.(*exec.ExitError); ok {
				rc = int(ee.Sys().(syscall.WaitStatus).ExitStatus())
				logrus.Infof("rc: %d", rc)
			}
			return stdout.String(), stderr.String(), CodeExitError{
				Err:  fmt.Errorf("error running %v:\nCommand stdout:\n%v\nstderr:\n%v\nerror:\n%v", cmd, cmd.Stdout, cmd.Stderr, err),
				Code: rc,
			}
		}
	case <-timeout:
		cmd.Process.Kill()
		return "", "", fmt.Errorf("timed out waiting for command %v:\nCommand stdout:\n%v\nstderr:\n%v", cmd, cmd.Stdout, cmd.Stderr)
	}
	logrus.Infof("stderr: %q", stderr.String())
	logrus.Infof("stdout: %q", stdout.String())
	return stdout.String(), stderr.String(), nil
}

// RunKubectlOrDie is a convenience wrapper over kubectlBuilder
func RunCRCOrDie(args ...string) string {
	return NewCRCCommand(args...).ExecOrDie()
}

// RunKubectl is a convenience wrapper over kubectlBuilder
func RunCRC(args ...string) (string, error) {
	return NewCRCCommand(args...).Exec()
}

// RunKubectlOrDie is a convenience wrapper over kubectlBuilder
func RunOCOrDie(args ...string) string {
	stdout, _, err := Exec(exec.Command("oc", args...), nil)
	Expect(err).To(Not(HaveOccurred()))
	return stdout
}

// RunKubectlOrDie is a convenience wrapper over kubectlBuilder
func RunOC(args ...string) (string, error) {
	stdout, _, err := Exec(exec.Command("oc", args...), nil)
	return stdout, err
}

// ExitError is an interface that presents an API similar to os.ProcessState, which is
// what ExitError from os/exec is.  This is designed to make testing a bit easier and
// probably loses some of the cross-platform properties of the underlying library.
type ExitError interface {
	String() string
	Error() string
	Exited() bool
	ExitStatus() int
}

// CodeExitError is an implementation of ExitError consisting of an error object
// and an exit code (the upper bits of os.exec.ExitStatus).
type CodeExitError struct {
	Err  error
	Code int
}

var _ ExitError = CodeExitError{}

func (e CodeExitError) Error() string {
	return e.Err.Error()
}

func (e CodeExitError) String() string {
	return e.Err.Error()
}

func (e CodeExitError) Exited() bool {
	return true
}

func (e CodeExitError) ExitStatus() int {
	return e.Code
}
