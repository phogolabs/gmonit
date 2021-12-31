package gmonit

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

// Config defines a ginkgomon Runner.
type Config struct {
	Name              string        // prefixes all output lines
	StartCheck        string        // text to match to indicate sucessful start.
	StartCheckTimeout time.Duration // how long to wait to see StartCheck
	Command           *exec.Cmd     // process to be executed
	Cleanup           func()        // invoked once the process exits
}

/*
Runner invokes a new process using gomega's gexec package.

If a start check is defined, the runner will wait until it sees the start check
before declaring ready.

Runner implements gexec.Exiter and gbytes.BufferProvider, so you can test exit
codes and process output using the appropriate gomega matchers:
http://onsi.github.io/gomega/#gexec-testing-external-processes
*/
type Runner struct {
	Name              string
	StartCheck        string
	StartCheckTimeout time.Duration
	Command           *exec.Cmd
	Cleanup           func()

	// private fields
	session *gexec.Session
	ready   chan struct{}
}

// New creates a ginkgomon Runner from a config object. Runners must be created
// with New to properly initialize their internal state.
func New(config Config) *Runner {
	return &Runner{
		Name:              config.Name,
		Command:           config.Command,
		StartCheck:        config.StartCheck,
		StartCheckTimeout: config.StartCheckTimeout,
		Cleanup:           config.Cleanup,
		ready:             make(chan struct{}),
	}
}

// ExitCode returns the exit code of the process, or -1 if the process has not
// exited.  It can be used with the gexec.Exit matcher.
func (r *Runner) ExitCode() int {
	r.validate()
	// done!
	<-r.ready
	return r.session.ExitCode()
}

// Buffer returns a gbytes.Buffer, for use with the gbytes.Say matcher.
func (r *Runner) Buffer() *gbytes.Buffer {
	r.validate()
	// done!
	<-r.ready
	return r.session.Buffer()
}

// Err returns the Buffer associated with the stderr stream.
// For use with the Say matcher.
func (r *Runner) Err() *gbytes.Buffer {
	r.validate()
	// done!
	<-r.ready
	return r.session.Err
}

func (r *Runner) Run(ch <-chan os.Signal, ready chan<- struct{}) (err error) {
	defer ginkgo.GinkgoRecover()

	var (
		observer = gbytes.NewBuffer()
		prefixer = gexec.NewPrefixedWriter(formatter.F("{{green}}[%v]{{/}} ", r.Name), ginkgo.GinkgoWriter)
		logger   = io.MultiWriter(observer, prefixer)
	)

	// start the process
	if r.session, err = gexec.Start(r.Command, logger, logger); err != nil {
		return fmt.Errorf("runner '%s' cannot start a process because of failure: %w", r.Name, err)
	}

	fmt.Fprintf(logger, "process started %s (pid: %d)", r.Command.Path, r.Command.Process.Pid)

	if r.ready != nil {
		close(r.ready)
	}

	var (
		detector = observer.Detect(r.StartCheck)
		deadline = r.deadline()
	)

	for {
		select {
		case <-detector:
			observer.CancelDetects()
			// clean up
			detector = nil
			deadline = nil
			// done!
			close(ready)
		case <-deadline:
			// clean up hanging process
			r.session.Kill().Wait()
			// read the application log
			output := string(observer.Contents())
			// fail to start
			return fmt.Errorf("runner '%s' did not see '%s' in command's output within the deadline. output: %s", r.Name, r.StartCheck, output)
		case signal := <-ch:
			r.session.Signal(signal)
		case <-r.session.Exited:
			if r.Cleanup != nil {
				r.Cleanup()
			}

			if r.session.ExitCode() == 0 {
				return nil
			}

			return fmt.Errorf("exit with status code: %d", r.session.ExitCode())
		}
	}
}

func (r *Runner) deadline() <-chan time.Time {
	timeout := r.StartCheckTimeout
	// set the default timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	var deadline <-chan time.Time
	// initialize the deadline if the check string is provided
	if r.StartCheck != "" {
		deadline = time.After(timeout)
	}

	return deadline
}

func (r *Runner) validate() {
	var err error

	if r.ready == nil {
		err = fmt.Errorf("runner '%s' improperly created without using New", r.Name)
	}

	if err != nil {
		ginkgo.Fail(err.Error(), 2)
	}
}
