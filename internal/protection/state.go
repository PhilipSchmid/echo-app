package protection

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type budgetState struct {
	Version  int    `json:"version"`
	Day      string `json:"day"`
	Reserved int64  `json:"reserved"`
}

func privateFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return fmt.Errorf("transfer state and lock files must be regular files with mode 0600")
	}
	return nil
}

func (g *Guard) openState() (retErr error) {
	dir := filepath.Dir(g.opts.StateFile)
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return fmt.Errorf("transfer state parent must be an existing private directory with mode 0700")
	}
	if err := privateFile(g.opts.StateFile); err != nil {
		return fmt.Errorf("validate transfer state: %w", err)
	}
	lockPath := g.opts.StateFile + ".lock"
	if err := privateFile(lockPath); err != nil {
		return fmt.Errorf("validate transfer state lock: %w", err)
	}
	g.lock, err = lockState(lockPath)
	if err != nil {
		return fmt.Errorf("lock transfer state: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = g.lock.Close()
		}
	}()
	file, err := os.Open(g.opts.StateFile)
	if errors.Is(err, os.ErrNotExist) {
		return g.saveState(g.state)
	}
	if err != nil {
		return fmt.Errorf("open transfer state: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err = file.Stat()
	if err != nil {
		return fmt.Errorf("stat transfer state: %w", err)
	}
	if info.Size() > 4096 {
		return fmt.Errorf("transfer state exceeds 4096 bytes")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 4097))
	decoder.DisallowUnknownFields()
	var state budgetState
	if err := decoder.Decode(&state); err != nil {
		return fmt.Errorf("decode transfer state: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("transfer state contains trailing data")
	}
	day, err := time.Parse(time.DateOnly, state.Day)
	if err != nil || day.Format(time.DateOnly) != state.Day || state.Version != 1 || state.Reserved < 0 {
		return fmt.Errorf("invalid transfer state")
	}
	// Loading a future day never grants a fresh allowance after clock rollback.
	g.state = state
	return nil
}

func (g *Guard) saveState(state budgetState) error {
	dir := filepath.Dir(g.opts.StateFile)
	file, err := os.CreateTemp(dir, ".transfer-budget-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(name)
	}()
	if err := json.NewEncoder(file).Encode(state); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, g.opts.StateFile); err != nil {
		return err
	}
	parent, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = parent.Close() }()
	return parent.Sync()
}
