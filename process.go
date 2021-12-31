package gmonit

import (
	"fmt"
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// Interrupt interrupts a process
func Interrupt(process *Process, intervals ...interface{}) {
	if process != nil {
		process.Signal(os.Interrupt)
		gomega.EventuallyWithOffset(1, process.Wait(), intervals...).Should(gomega.Receive(), "process interrupted because it failed to exit in time")
	}
}

// Kill kills a process
func Kill(process *Process, intervals ...interface{}) {
	if process != nil {
		process.Signal(os.Kill)
		gomega.EventuallyWithOffset(1, process.Wait(), intervals...).Should(gomega.Receive(), "process killer because it failed to exit in time")
	}
}

// Process represents a process
type Process struct {
	runner  *Runner
	signals chan os.Signal
	ready   chan struct{}
	exited  chan struct{}
	status  error
}

// Background start the runner in background
func Background(runner *Runner) *Process {
	process := &Process{
		runner:  runner,
		signals: make(chan os.Signal),
		ready:   make(chan struct{}),
		exited:  make(chan struct{}),
	}

	// start the process
	go process.Run()

	return process
}

// Invoke invokes a runner
func Invoke(runner *Runner) *Process {
	process := Background(runner)

	select {
	case <-process.Ready():
	case err := <-process.Wait():
		err = fmt.Errorf("runner '%s' cannot start a process because of failure:: %w", runner.Name, err)
		ginkgo.Fail(err.Error(), 1)
	}

	return process
}

// Ready makes the process ready
func (p *Process) Ready() <-chan struct{} {
	return p.ready
}

// Wait waits for the process
func (p *Process) Wait() <-chan error {
	exit := make(chan error, 1)

	go func() {
		<-p.exited
		exit <- p.status
	}()

	return exit
}

// Signal sends a signal
func (p *Process) Signal(signal os.Signal) {
	go func() {
		select {
		case p.signals <- signal:
		case <-p.exited:
		}
	}()
}

// Run starts the process
func (p *Process) Run() {
	p.status = p.runner.Run(p.signals, p.ready)
	close(p.exited)
}
