package protection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func stateOptions(t *testing.T) Options {
	t.Helper()
	switch runtime.GOOS {
	case "darwin", "dragonfly", "freebsd", "linux", "netbsd", "openbsd":
	default:
		t.Skip("persistent transfer budgets require advisory locking on this platform")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	o := testOptions()
	o.StateFile = filepath.Join(dir, "transfer-budget.json")
	o.DailyEgress = 2 * leaseSize
	return o
}

func writeState(t *testing.T, o Options, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(o.StateFile, []byte(contents), 0600))
}

func TestPersistentBudgetLeasesBeforeSendingAndSurvivesRestart(t *testing.T) {
	o := stateOptions(t)
	g, _ := testGuard(t, o)
	require.NoError(t, g.ReserveEgress(1))
	snapshot := g.Snapshot()
	require.True(t, snapshot.Persistent)
	require.Equal(t, int64(leaseSize), snapshot.Reserved)
	require.Equal(t, int64(leaseSize), snapshot.Remaining)
	data, err := os.ReadFile(o.StateFile)
	require.NoError(t, err)
	var state budgetState
	require.NoError(t, json.Unmarshal(data, &state))
	require.Equal(t, int64(leaseSize), state.Reserved)
	info, err := os.Stat(o.StateFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	require.NoError(t, g.Close())

	g, now := testGuard(t, o)
	require.Equal(t, snapshot, g.Snapshot())
	// A crashed process's unused lease cannot be reclaimed by a restart.
	require.NoError(t, g.ReserveEgress(1))
	require.Equal(t, int64(2*leaseSize), g.Snapshot().Reserved)
	require.NoError(t, g.ReserveEgress(leaseSize-1))
	require.ErrorIs(t, g.ReserveEgress(1), ErrDailyEgress)
	*now = now.Add(24 * time.Hour)
	require.NoError(t, g.ReserveEgress(1))
	require.Equal(t, int64(leaseSize), g.Snapshot().Reserved)
}

func TestPersistentBudgetClockRollbackAndLoweredLimit(t *testing.T) {
	o := stateOptions(t)
	day := time.Now().UTC().Add(48 * time.Hour).Format(time.DateOnly)
	writeState(t, o, `{"version":1,"day":"`+day+`","reserved":2097152}`)
	o.DailyEgress = 100
	g, now := testGuard(t, o)
	require.ErrorIs(t, g.ReserveEgress(1), ErrDailyEgress)
	require.Equal(t, day, g.Snapshot().Day)
	require.Zero(t, g.Snapshot().Remaining)
	*now = now.Add(24 * time.Hour)
	require.ErrorIs(t, g.ReserveEgress(1), ErrDailyEgress)
	*now = now.Add(48 * time.Hour)
	require.NoError(t, g.ReserveEgress(1))
	require.Equal(t, int64(100), g.Snapshot().Reserved)
}

func TestPersistentBudgetRejectsCorruptStateAndReleasesLock(t *testing.T) {
	for _, contents := range []string{
		"", "invalid", `{}`, `{"version":2,"day":"2026-10-25","reserved":0}`,
		`{"version":1,"day":"invalid","reserved":0}`,
		`{"version":1,"day":"2026-10-25","reserved":-1}`,
		`{"version":1,"day":"2026-10-25","reserved":9223372036854775808}`,
		`{"version":1,"day":"2026-10-25","reserved":0,"unknown":true}`,
		`{"version":1,"day":"2026-10-25","reserved":0} {}`,
		`{"version":1,"day":"2026-10-25","reserved":0}` + strings.Repeat(" ", 4096),
	} {
		t.Run(contents[:min(len(contents), 60)], func(t *testing.T) {
			o := stateOptions(t)
			writeState(t, o, contents)
			_, err := NewGuard(o)
			require.Error(t, err)
			writeState(t, o, `{"version":1,"day":"2026-10-25","reserved":0}`)
			_, _ = testGuard(t, o)
		})
	}
}

func TestPersistentBudgetSingleWriter(t *testing.T) {
	o := stateOptions(t)
	g, _ := testGuard(t, o)
	_, err := NewGuard(o)
	require.ErrorContains(t, err, "lock transfer state")
	require.NoError(t, g.Close())
	_, _ = testGuard(t, o)
	_, err = os.Stat(o.StateFile + ".lock")
	require.NoError(t, err, "lock inode must remain stable across owners")
}

func TestPersistentBudgetRejectsUnsafePaths(t *testing.T) {
	t.Run("parent permissions", func(t *testing.T) {
		o := stateOptions(t)
		require.NoError(t, os.Chmod(filepath.Dir(o.StateFile), 0755))
		_, err := NewGuard(o)
		require.ErrorContains(t, err, "private directory")
	})
	t.Run("file permissions", func(t *testing.T) {
		o := stateOptions(t)
		writeState(t, o, `{}`)
		require.NoError(t, os.Chmod(o.StateFile, 0644))
		_, err := NewGuard(o)
		require.ErrorContains(t, err, "mode 0600")
	})
	for _, suffix := range []string{"", ".lock"} {
		t.Run("symlink"+suffix, func(t *testing.T) {
			o := stateOptions(t)
			target := filepath.Join(filepath.Dir(o.StateFile), "target")
			require.NoError(t, os.WriteFile(target, []byte("unchanged"), 0600))
			require.NoError(t, os.Symlink(target, o.StateFile+suffix))
			_, err := NewGuard(o)
			require.Error(t, err)
			data, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, "unchanged", string(data))
		})
	}
	t.Run("parent symlink", func(t *testing.T) {
		o := stateOptions(t)
		dir := t.TempDir()
		link := filepath.Join(dir, "link")
		require.NoError(t, os.Symlink(filepath.Dir(o.StateFile), link))
		o.StateFile = filepath.Join(link, "state.json")
		_, err := NewGuard(o)
		require.Error(t, err)
	})
}

func TestPersistentBudgetWriteFailureStaysClosed(t *testing.T) {
	o := stateOptions(t)
	g, _ := testGuard(t, o)
	// Renaming the directory after startup reliably fails writes even as root.
	dir := filepath.Dir(o.StateFile)
	require.NoError(t, os.Rename(dir, dir+"-moved"))
	t.Cleanup(func() { require.NoError(t, os.Rename(dir+"-moved", dir)) })
	require.ErrorIs(t, g.ReserveEgress(1), ErrState)
	require.Zero(t, g.Snapshot().Reserved)
	require.NoError(t, os.Mkdir(dir, 0700))
	t.Cleanup(func() { require.NoError(t, os.Remove(dir)) })
	// Recovering the path does not reuse stale state after a partial save failure.
	require.ErrorIs(t, g.ReserveEgress(1), ErrState)
}
