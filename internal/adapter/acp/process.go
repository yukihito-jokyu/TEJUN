package acp

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
)

type process struct {
	cmd    *exec.Cmd
	path   string
	stdin  io.WriteCloser
	done   chan struct{}
	err    error
	cancel context.CancelFunc
	once   sync.Once
}

func startProcess(
	ownerContext context.Context,
	startContext context.Context,
	command string,
	args []string,
	environment []string,
) (*process, io.ReadCloser, error) {
	if err := startContext.Err(); err != nil {
		return nil, nil, err
	}

	path, err := exec.LookPath(command)
	if err != nil {
		return nil, nil, err
	}

	processContext, cancel := context.WithCancel(ownerContext)
	cmd := exec.CommandContext(processContext, path, args...)
	cmd.Env = environment

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()

		return nil, nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()

		_ = stdin.Close()

		return nil, nil, err
	}

	if err := cmd.Start(); err != nil {
		cancel()

		_ = stdin.Close()
		_ = stdout.Close()

		return nil, nil, err
	}

	if err := startContext.Err(); err != nil {
		cancel()

		_ = stdin.Close()
		_ = stdout.Close()
		_ = cmd.Wait()

		return nil, nil, err
	}

	p := &process{cmd: cmd, path: path, stdin: stdin, done: make(chan struct{}), cancel: cancel}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()

	return p, stdout, nil
}

func (p *process) close() error {
	p.once.Do(func() {
		p.cancel()

		_ = p.stdin.Close()
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
	})
	<-p.done

	if errors.Is(p.err, context.Canceled) || errors.Is(p.err, context.DeadlineExceeded) {
		return nil
	}

	if exitErr := new(exec.ExitError); errors.As(p.err, &exitErr) {
		return nil
	}

	return p.err
}
