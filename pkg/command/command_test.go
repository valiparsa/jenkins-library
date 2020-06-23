package command

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/SAP/jenkins-library/pkg/log"
	"github.com/stretchr/testify/assert"
)

//based on https://golang.org/src/os/exec/exec_test.go
func helperCommand(command string, s ...string) (cmd *exec.Cmd) {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, s...)
	cmd = exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestShellRun(t *testing.T) {

	t.Run("test shell", func(t *testing.T) {
		ExecCommand = helperCommand
		defer func() { ExecCommand = exec.Command }()
		o := new(bytes.Buffer)
		e := new(bytes.Buffer)

		s := Command{stdout: o, stderr: e}
		s.RunShell("/bin/bash", "myScript")

		t.Run("success case", func(t *testing.T) {
			t.Run("stdin-stdout", func(t *testing.T) {
				expectedOut := "Stdout: command /bin/bash - Stdin: myScript\n"
				if oStr := o.String(); oStr != expectedOut {
					t.Errorf("expected: %v got: %v", expectedOut, oStr)
				}
			})
			t.Run("stderr", func(t *testing.T) {
				expectedErr := "Stderr: command /bin/bash\n"
				if eStr := e.String(); eStr != expectedErr {
					t.Errorf("expected: %v got: %v", expectedErr, eStr)
				}
			})
		})
	})
}

func TestExecutableRun(t *testing.T) {

	t.Run("test shell", func(t *testing.T) {
		ExecCommand = helperCommand
		defer func() { ExecCommand = exec.Command }()
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)

		t.Run("success case", func(t *testing.T) {
			ex := Command{stdout: stdout, stderr: stderr}
			ex.RunExecutable("echo", []string{"foo bar", "baz"}...)

			t.Run("stdin", func(t *testing.T) {
				expectedOut := "foo bar baz\n"
				if oStr := stdout.String(); oStr != expectedOut {
					t.Errorf("expected: %v got: %v", expectedOut, oStr)
				}
			})
			t.Run("stderr", func(t *testing.T) {
				expectedErr := "Stderr: command echo\n"
				if eStr := stderr.String(); eStr != expectedErr {
					t.Errorf("expected: %v got: %v", expectedErr, eStr)
				}
			})
		})

		t.Run("success case - log parsing", func(t *testing.T) {
			ex := Command{stdout: stdout, stderr: stderr, ErrorCategoryMapping: map[string][]string{"config": {"command echo"}}}
			ex.RunExecutable("echo", []string{"foo bar", "baz"}...)
			assert.Equal(t, log.ErrorConfiguration, log.GetErrorCategory())
		})
	})
}

func TestEnvironmentVariables(t *testing.T) {

	ExecCommand = helperCommand
	defer func() { ExecCommand = exec.Command }()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	ex := Command{stdout: stdout, stderr: stderr}

	// helperCommand function replaces the full environment with one single entry
	// (GO_WANT_HELPER_PROCESS), hence there is no need for checking if the DEBUG
	// environment variable already exists in the set of environment variables for the
	// current process.
	ex.SetEnv([]string{"DEBUG=true"})
	ex.RunExecutable("env")

	oStr := stdout.String()

	if !strings.Contains(oStr, "DEBUG=true") {
		t.Errorf("expected Environment variable not found")
	}
}

func TestPrepareOut(t *testing.T) {

	t.Run("os", func(t *testing.T) {
		s := Command{}
		s.prepareOut()

		if s.stdout != os.Stdout {
			t.Errorf("expected out to be os.Stdout")
		}

		if s.stderr != os.Stderr {
			t.Errorf("expected err to be os.Stderr")
		}
	})

	t.Run("custom", func(t *testing.T) {
		o := bytes.NewBufferString("")
		e := bytes.NewBufferString("")
		s := Command{stdout: o, stderr: e}
		s.prepareOut()

		expectOut := "Test out"
		expectErr := "Test err"
		s.stdout.Write([]byte(expectOut))
		s.stderr.Write([]byte(expectErr))

		t.Run("out", func(t *testing.T) {
			if o.String() != expectOut {
				t.Errorf("expected: %v got: %v", expectOut, o.String())
			}
		})
		t.Run("err", func(t *testing.T) {
			if e.String() != expectErr {
				t.Errorf("expected: %v got: %v", expectErr, e.String())
			}
		})
	})
}

func TestParseConsoleErrors(t *testing.T) {
	cmd := Command{
		ErrorCategoryMapping: map[string][]string{
			"config": {"configuration error 1", "configuration error 2"},
			"build":  {"build failed"},
		},
	}

	tt := []struct {
		consoleLine      string
		expectedCategory log.ErrorCategory
	}{
		{consoleLine: "this is an error", expectedCategory: log.ErrorUndefined},
		{consoleLine: "this is configuration error 2", expectedCategory: log.ErrorConfiguration},
		{consoleLine: "the build failed", expectedCategory: log.ErrorBuild},
	}

	for _, test := range tt {
		cmd.parseConsoleErrors(test.consoleLine)
		assert.Equal(t, test.expectedCategory, log.GetErrorCategory(), test.consoleLine)
	}
}

func TestCmdPipes(t *testing.T) {
	cmd := helperCommand("echo", "foo bar", "baz")
	defer func() { ExecCommand = exec.Command }()

	t.Run("success case", func(t *testing.T) {
		o, e, err := cmdPipes(cmd)
		t.Run("no error", func(t *testing.T) {
			if err != nil {
				t.Errorf("error occured but no error expected")
			}
		})

		t.Run("out pipe", func(t *testing.T) {
			if o == nil {
				t.Errorf("no pipe received")
			}
		})

		t.Run("err pipe", func(t *testing.T) {
			if e == nil {
				t.Errorf("no pipe received")
			}
		})
	})
}

//based on https://golang.org/src/os/exec/exec_test.go
//this is not directly executed
func TestHelperProcess(*testing.T) {

	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(2)
	}

	cmd, args := args[0], args[1:]
	switch cmd {
	case "/bin/bash":
		o, _ := ioutil.ReadAll(os.Stdin)
		fmt.Fprintf(os.Stdout, "Stdout: command %v - Stdin: %v\n", cmd, string(o))
		fmt.Fprintf(os.Stderr, "Stderr: command %v\n", cmd)
	case "echo":
		iargs := []interface{}{}
		for _, s := range args {
			iargs = append(iargs, s)
		}
		fmt.Println(iargs...)
		fmt.Fprintf(os.Stderr, "Stderr: command %v\n", cmd)
	case "env":
		for _, e := range os.Environ() {
			fmt.Println(e)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", cmd)
		os.Exit(2)

	}
}
