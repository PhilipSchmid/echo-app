//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtectionBudgetPersistsAndAdminRemainsAvailable(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test builds and restarts the application")
	}
	switch runtime.GOOS {
	case "darwin", "dragonfly", "freebsd", "linux", "netbsd", "openbsd":
	default:
		t.Skip("persistent quota locking is unsupported on this platform")
	}
	binary := filepath.Join(t.TempDir(), "echo-observatory")
	buildCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(buildCtx, "go", "build", "-race", "-o", binary, "../cmd/echo-observatory").CombinedOutput()
	require.NoError(t, err, "build: %s", output)
	stateDir := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.Mkdir(stateDir, 0700))
	stateFile := filepath.Join(stateDir, "transfer-budget.json")
	usedPorts := make(map[string]bool)
	client := &http.Client{Timeout: time.Second}
	t.Cleanup(client.CloseIdleConnections)

	first := startProtectedCLI(t, binary, stateFile, usedPorts, client)
	connection, err := net.DialTimeout("tcp", first.tcpAddress, time.Second)
	require.NoError(t, err)
	require.NoError(t, connection.SetDeadline(time.Now().Add(2*time.Second)))
	data, err := io.ReadAll(io.LimitReader(connection, 65536))
	_ = connection.Close()
	require.NoError(t, err)
	require.Less(t, len(data), 4096, "restart must discard a partly unused durable reservation")
	var response struct {
		Listener string `json:"listener"`
	}
	require.NoError(t, json.Unmarshal(data, &response))
	require.Equal(t, "TCP", response.Listener, "the first protected process must serve real echo traffic")
	state, err := os.ReadFile(stateFile)
	require.NoError(t, err)
	var reservation struct {
		Day      string `json:"day"`
		Reserved int64  `json:"reserved"`
	}
	require.NoError(t, json.Unmarshal(state, &reservation))
	require.Equal(t, int64(4096), reservation.Reserved, "the first write must durably reserve the entire small allowance")
	first.stop(t)

	second := startProtectedCLI(t, binary, stateFile, usedPorts, client)
	if time.Now().UTC().Format(time.DateOnly) != reservation.Day {
		t.Skip("UTC day rolled over during process restart")
	}
	connection, err = net.DialTimeout("tcp", second.tcpAddress, time.Second)
	require.NoError(t, err)
	require.NoError(t, connection.SetDeadline(time.Now().Add(2*time.Second)))
	data, err = io.ReadAll(io.LimitReader(connection, 65536))
	_ = connection.Close()
	require.NoError(t, err)
	require.Empty(t, data, "restart must not refund unused reserved bytes to the TCP listener")
	publicResponse, publicErr := client.Get(second.httpURL + "/")
	if publicResponse != nil {
		_ = publicResponse.Body.Close()
	}
	require.Error(t, publicErr, "the same exhausted quota must also block HTTP echo output")

	for path, want := range map[string]string{"/health": "healthy", "/ready": "ready"} {
		adminResponse, requestErr := client.Get(second.adminURL + path)
		require.NoError(t, requestErr)
		body, readErr := io.ReadAll(io.LimitReader(adminResponse.Body, 65536))
		_ = adminResponse.Body.Close()
		require.NoError(t, readErr)
		assert.Equal(t, http.StatusOK, adminResponse.StatusCode, "private admin %s", path)
		assert.Equal(t, want, string(body))
	}
	scrape, err := client.Get(second.adminURL + "/metrics")
	require.NoError(t, err)
	defer func() { _ = scrape.Body.Close() }()
	require.Equal(t, http.StatusOK, scrape.StatusCode)
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(io.LimitReader(scrape.Body, 1<<20))
	require.NoError(t, err)
	for name, want := range map[string]float64{
		"echo_observatory_protection_enabled":           1,
		"echo_observatory_daily_egress_persistent":      1,
		"echo_observatory_daily_egress_limit_bytes":     4096,
		"echo_observatory_daily_egress_reserved_bytes":  4096,
		"echo_observatory_daily_egress_remaining_bytes": 0,
	} {
		samples := families[name].GetMetric()
		require.Len(t, samples, 1, name)
		assert.Equal(t, want, samples[0].GetGauge().GetValue(), name)
	}
	var rejectedTCP float64
	for _, sample := range families["echo_observatory_rejections_total"].GetMetric() {
		labels := make(map[string]string)
		for _, label := range sample.GetLabel() {
			labels[label.GetName()] = label.GetValue()
		}
		if labels["listener"] == "TCP" && labels["reason"] == "daily_egress" {
			rejectedTCP += sample.GetCounter().GetValue()
		}
	}
	assert.GreaterOrEqual(t, rejectedTCP, float64(1), "the private scrape must explain the exhausted TCP allowance")
	second.stop(t)
}

type protectedCLI struct {
	cmd                           *exec.Cmd
	done                          chan error
	waited                        bool
	httpURL, adminURL, tcpAddress string
}

func startProtectedCLI(t *testing.T, binary, stateFile string, usedPorts map[string]bool, client *http.Client) *protectedCLI {
	t.Helper()
	nextPort := func() string {
		for {
			port := unusedPort(t)
			if !usedPorts[port] {
				usedPorts[port] = true
				return port
			}
		}
	}
	httpPort, tcpPort, adminPort := nextPort(), nextPort(), nextPort()
	cmd := exec.Command(binary,
		"--protection=true", "--request-rate=1000", "--peer-request-rate=0", "--daily-egress-bytes=4096",
		"--transfer-state-file="+stateFile, "--tcp", "--metrics", "--log-level=warn",
		"--http-port="+httpPort, "--tcp-port="+tcpPort, "--metrics-port="+adminPort,
	)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "ECHO_OBSERVATORY_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	logFile, err := os.Create(filepath.Join(t.TempDir(), "server.log"))
	require.NoError(t, err)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	process := &protectedCLI{
		cmd: cmd, done: make(chan error, 1),
		httpURL: "http://127.0.0.1:" + httpPort, adminURL: "http://127.0.0.1:" + adminPort,
		tcpAddress: "127.0.0.1:" + tcpPort,
	}
	t.Cleanup(func() {
		process.stop(t)
		_ = logFile.Close()
		if t.Failed() {
			data, _ := os.ReadFile(logFile.Name())
			t.Logf("server log:\n%s", data)
		}
	})
	require.NoError(t, cmd.Start())
	go func() { process.done <- cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		ready, requestErr := client.Get(process.adminURL + "/ready")
		if requestErr == nil {
			_, _ = io.Copy(io.Discard, ready.Body)
			_ = ready.Body.Close()
			if ready.StatusCode == http.StatusOK {
				return process
			}
		}
		select {
		case exitErr := <-process.done:
			process.waited = true
			t.Fatalf("protected process exited before readiness: %v", exitErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("protected process did not become ready: %v", requestErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (process *protectedCLI) stop(t *testing.T) {
	t.Helper()
	if process.waited || process.cmd.Process == nil {
		return
	}
	_ = process.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case err := <-process.done:
		process.waited = true
		assert.NoError(t, err, "protected process must exit cleanly on SIGTERM")
	case <-time.After(5 * time.Second):
		_ = process.cmd.Process.Kill()
		select {
		case err := <-process.done:
			process.waited = true
			t.Logf("forced process shutdown: %v", err)
		case <-time.After(5 * time.Second):
			t.Error("killed process did not exit")
		}
		t.Error("protected process did not drain within five seconds")
	}
}
