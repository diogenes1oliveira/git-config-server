package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
)

type Command struct {
	Args        []string
	Pid         int
	RestartArgs []string
	cmd         *exec.Cmd
	sigCh       chan os.Signal
	exitCh      chan int
	errorCh     chan error
	ctx         context.Context
	cancel      context.CancelFunc
	exitCode    int
}

func NewCommand(ctx context.Context, args []string, restartArgs []string) *Command {
	return &Command{
		Args:        args,
		RestartArgs: restartArgs,
		Pid:         -1,
		ctx:         ctx,
	}
}

func (c *Command) Start() error {
	if c.IsRunning() {
		return fmt.Errorf("command %v is already running", c)
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.cmd = exec.CommandContext(ctx, c.Args[0], c.Args[1:]...)
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr

	log.Printf("starting command: %v", c)
	err := c.cmd.Start()
	if err != nil {
		return err
	}
	c.cancel = cancel
	c.exitCh = make(chan int, 1)
	c.errorCh = make(chan error, 1)

	c.Pid = c.cmd.Process.Pid
	log.Printf("command running: %v", c)

	go func() {
		defer func() {
			c.cancel = nil
		}()
		defer close(c.exitCh)
		defer close(c.errorCh)
		defer cancel()

		err := c.cmd.Wait()
		c.sigCh = nil
		c.exitCode = 0

		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				c.exitCode = exitError.ExitCode()
				c.exitCh <- c.exitCode
			} else {
				log.Printf("command failed: %v\n", err)
				c.errorCh <- err
				return
			}
		}
		log.Printf("command %v finished with exit code %d\n", c, c.exitCode)
	}()

	return nil
}

func (c *Command) IsRunning() bool {
	return c.cancel != nil
}

func (c *Command) Stop() error {
	cancel := c.cancel

	if cancel == nil {
		log.Printf("already stopped\n")
		return nil
	}

	log.Printf("cancelling command context\n")
	cancel()
	select {
	case err := <-c.errorCh:
		if err != nil {
			return err
		}
	case <-c.exitCh:
		//pass
	}

	return nil
}

func (c *Command) Restart() error {
	if len(c.RestartArgs) > 0 {
		log.Printf("executing restart command\n")
		cmd := exec.Command(c.RestartArgs[0], c.RestartArgs[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("failed to restart command: %w", err)
		}
		return nil
	}

	log.Printf("Stopping command %s (pid=%d)\n", c.Args[0], c.Pid)
	err := c.Stop()
	if err != nil {
		return fmt.Errorf("failed to stop command: %w", err)
	}

	log.Printf("starting command again: %v", c.Args[0])
	err = c.Start()
	if err != nil {
		return fmt.Errorf("failed to start command again: %w", err)
	}

	log.Printf("Command running with pid=%d", c.Pid)
	return nil
}

func (c *Command) String() string {
	if c.Pid >= 0 {
		return fmt.Sprintf("Command(args=%v pid=%d)", c.Args, c.Pid)
	} else {
		return fmt.Sprintf("Command(args=%v)", c.Args)
	}
}

func runShellCommand(shellCommand, runner, workingDir string) error {

	cmd := exec.Command(runner, "-c", shellCommand)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if workingDir != "" {
		cmd.Dir = workingDir
	} else {
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get cwd: %w", err)
		}
		cmd.Dir = dir
	}

	log.Printf("running command with runner %s on cwd=%s: %s\n", runner, cmd.Dir, shellCommand)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run shell command: %w", err)
	}

	return nil
}
