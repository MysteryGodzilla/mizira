// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package keeper

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// proc is a child the keeper started.
type proc struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser // closing it asks a bot started with METALD_STOP_ON_STDIN_EOF to stop
	done  chan struct{}
	mu    sync.Mutex
	code  int
	since time.Time
}

// launch starts exe with its output appended to the log files (stderr to errLog, or to log when
// errLog is empty). env is added to the keeper's own environment.
func launch(exe string, args []string, dir, log, errLog string, env []string) (*proc, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	hide(cmd)
	var files []*os.File
	open := func(path string) (*os.File, error) {
		if path == "" {
			return nil, nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if f != nil {
			files = append(files, f)
		}
		return f, err
	}
	closeAll := func() {
		for _, f := range files {
			_ = f.Close()
		}
	}
	out, err := open(log)
	if err != nil {
		closeAll()
		return nil, err
	}
	errOut := out
	if errLog != "" {
		if errOut, err = open(errLog); err != nil {
			closeAll()
			return nil, err
		}
	}
	if out != nil {
		cmd.Stdout = out
	}
	if errOut != nil {
		cmd.Stderr = errOut
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		closeAll()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		closeAll()
		return nil, err
	}
	p := &proc{cmd: cmd, stdin: stdin, done: make(chan struct{}), since: time.Now()}
	go func() {
		err := cmd.Wait()
		closeAll()
		p.mu.Lock()
		p.code = 0
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			p.code = exit.ExitCode()
		} else if err != nil {
			p.code = -1
		}
		p.mu.Unlock()
		close(p.done)
	}()
	return p, nil
}

func (p *proc) pid() int { return p.cmd.Process.Pid }

func (p *proc) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *proc) exitCode() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.code
}

// stop closes stdin and waits up to grace for the process to leave, then kills it. It reports
// whether the process left by itself.
func (p *proc) stop(grace time.Duration) bool {
	_ = p.stdin.Close()
	select {
	case <-p.done:
		return true
	case <-time.After(grace):
	}
	_ = p.cmd.Process.Kill()
	<-p.done
	return false
}
