package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"awesomeProject/internal/config"
)

// errAlreadyRunning reports that another tuicord process already holds the
// single-instance lock. A second instance means two gateway sessions and two
// startup REST bursts on one account, which Discord's fraud detection can flag
// as automated abuse, so a single instance is the only safe configuration.
var errAlreadyRunning = errors.New("another tuicord instance is already running for this account")

// acquireInstanceLock takes an exclusive advisory lock beside the config file.
// The returned closure releases it and must be deferred for the process
// lifetime. The lock is held by an open descriptor, so the kernel releases it
// automatically if the process dies; a crash never leaves a stale lock.
func acquireInstanceLock() (func(), error) {
	path, err := config.LockPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errAlreadyRunning
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
