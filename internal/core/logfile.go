// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RotatingFile is a log file that starts afresh once it reaches maxSize bytes, keeping the
// last `keep` files as name.1 (newest) to name.<keep> (oldest). It caps disk use, so a chatty
// or attacked bot can't fill the drive.
type RotatingFile struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	keep    int
	file    *os.File
	size    int64
}

// OpenRotatingFile opens (or creates) path for appending. The folder and file are created
// owner-only where the OS supports it: the log can name users and quote what they said.
func OpenRotatingFile(path string, maxSize int64, keep int) (*RotatingFile, error) {
	if maxSize <= 0 {
		return nil, fmt.Errorf("log file max size must be positive, got %d", maxSize)
	}
	if keep < 0 {
		keep = 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	r := &RotatingFile{path: path, maxSize: maxSize, keep: keep}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *RotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.file, r.size = f, info.Size()
	return nil
}

// Write appends p, rotating first if p would take the file past maxSize. A single record is
// never split across files.
func (r *RotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return 0, os.ErrClosed
	}
	if r.size > 0 && r.size+int64(len(p)) > r.maxSize {
		if err := r.rotate(); err != nil {
			// Keep logging into the current file rather than losing lines.
			fmt.Fprintf(os.Stderr, "log file rotation failed: %v\n", err)
		}
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate shifts name.N-1 to name.N (dropping the oldest), name to name.1, and opens a fresh name.
// On Windows a file must be closed before it can be renamed, hence close first.
func (r *RotatingFile) rotate() error {
	if err := r.file.Close(); err != nil {
		return err
	}
	r.file = nil
	if r.keep == 0 {
		os.Remove(r.path)
	} else {
		os.Remove(r.backup(r.keep))
		for i := r.keep - 1; i >= 1; i-- {
			os.Rename(r.backup(i), r.backup(i+1)) // missing files are fine
		}
		if err := os.Rename(r.path, r.backup(1)); err != nil {
			// Reopen the old file so logging continues.
			if openErr := r.open(); openErr != nil {
				return openErr
			}
			return err
		}
	}
	return r.open()
}

func (r *RotatingFile) backup(i int) string { return fmt.Sprintf("%s.%d", r.path, i) }

// Close closes the current file.
func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}
