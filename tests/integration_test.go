//go:build integration

package integration_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	pb "github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/quic-go/quic-go/http3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestAllProtocols(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test builds and launches the application")
	}
	binary := filepath.Join(t.TempDir(), "echo-observatory")
	if output, err := exec.Command("go", "build", "-o", binary, "../cmd/echo-observatory").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	ports := make(map[string]string)
	dependencyURL, setDependencyReady := newReadinessDependency(t)
	args := []string{
		"--tls", "--h2c", "--tcp", "--grpc", "--quic", "--metrics",
		"--message=integration-test", "--node=test-node", "--max-request-size=4",
		"--response-controls", "--print-http-request-headers",
		"--external-readiness-probe-type=http", "--external-readiness-probe-target=" + dependencyURL,
		"--external-readiness-probe-interval=20ms", "--external-readiness-probe-timeout=500ms",
	}
	for _, listener := range []string{"http", "tls", "tcp", "grpc", "quic", "metrics"} {
		ports[listener] = unusedPort(t, listener == "quic")
		args = append(args, "--"+listener+"-port="+ports[listener])
	}
	logFile, err := os.Create(filepath.Join(t.TempDir(), "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = logFile.Close() }()
	cmd := exec.Command(binary, args...)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "ECHO_OBSERVATORY_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			if err := <-done; err != nil {
				t.Logf("process cleanup: %v", err)
			}
		}
		if t.Failed() {
			data, _ := os.ReadFile(logFile.Name())
			t.Logf("server log:\n%s", data)
		}
	}()

	plain := &http.Client{Timeout: 2 * time.Second}
	defer plain.CloseIdleConnections()
	adminURL := "http://127.0.0.1:" + ports["metrics"]
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, requestErr := plain.Get(adminURL + "/ready")
		if requestErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		select {
		case exitErr := <-done:
			waited = true
			t.Fatalf("application exited before ready: %v", exitErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("application did not become ready within 10 seconds")
		}
		time.Sleep(20 * time.Millisecond)
	}

	for _, tc := range []struct {
		name, port, scheme, protocol, listener string
	}{
		{"HTTP", ports["http"], "http", "HTTP/1.1", "HTTP"},
		{"H2C", ports["http"], "http", "HTTP/2.0", "H2C"},
		{"HTTPS", ports["tls"], "https", "HTTP/1.1", "TLS"},
		{"HTTPS_HTTP2", ports["tls"], "https", "HTTP/2.0", "TLS"},
		{"HTTP3", ports["quic"], "https", "HTTP/3.0", "QUIC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var transport http.RoundTripper
			if tc.name == "HTTP3" {
				h3 := &http3.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "diagnostic.example"}} // Self-signed demo certificate.
				defer func() { _ = h3.Close() }()
				transport = h3
			} else {
				protocols := new(http.Protocols)
				protocols.SetHTTP1(tc.protocol == "HTTP/1.1")
				protocols.SetHTTP2(tc.name == "HTTPS_HTTP2")
				protocols.SetUnencryptedHTTP2(tc.name == "H2C")
				tr := &http.Transport{Protocols: protocols, TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "diagnostic.example"}} // Self-signed demo certificate.
				defer tr.CloseIdleConnections()
				transport = tr
			}
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			if err := checkJSON(client, tc.scheme+"://127.0.0.1:"+tc.port+"/test%2Fpath?item=42&encoded=a%2Bb", tc.listener, tc.protocol); err != nil {
				t.Fatal(err)
			}
			if tc.name == "HTTP" || tc.name == "HTTP3" {
				checkResponseControls(t, client, tc.scheme+"://127.0.0.1:"+tc.port)
			}
		})
	}
	t.Run("TCP", func(t *testing.T) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+ports["tcp"], 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
			t.Fatal(err)
		}
		var response map[string]any
		decoder := json.NewDecoder(conn)
		if err := decoder.Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response["listener"] != "TCP" || response["message"] != "integration-test" {
			t.Fatalf("unexpected response: %v", response)
		}
		if err := decoder.Decode(&response); err != io.EOF {
			t.Fatalf("expected connection EOF, got %v", err)
		}
	})
	t.Run("gRPC", func(t *testing.T) {
		conn, err := grpc.NewClient("127.0.0.1:"+ports["grpc"], grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		response, err := pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if response.Listener != "gRPC" || response.Message != "integration-test" || response.GrpcMethod != "/echo.EchoService/Echo" {
			t.Fatalf("unexpected response: %v", response)
		}
	})
	for path, want := range map[string]string{"/health": "healthy", "/ready": "ready", "/metrics": "echo_app_requests_total"} {
		resp, err := plain.Get(adminURL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusOK || !strings.Contains(string(body), want) {
			t.Fatalf("%s: status=%d body=%s err=%v", path, resp.StatusCode, body, readErr)
		}
	}
	finishFeatureShutdown := prepareFeatureShutdown(t, ports, setDependencyReady)
	plain.CloseIdleConnections()
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	finishFeatureShutdown()
	select {
	case err := <-done:
		waited = true
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown exceeded 10 seconds")
	}
}

func unusedPort(t *testing.T, udp bool) string {
	t.Helper()
	if udp {
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		return strconv.Itoa(conn.LocalAddr().(*net.UDPAddr).Port)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	return strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
}

func checkJSON(client *http.Client, url, listener, protocol string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Host = "diagnostic.example:8443"
	req.Header.Set("Authorization", "Bearer integration-secret")
	req.Header.Set("Cookie", "session=integration-secret")
	req.Header.Set("Forwarded", "for=203.0.113.10;proto=https;host=proxy.example")
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || resp.Proto != protocol {
		return fmt.Errorf("status=%d protocol=%s, expected 200 %s", resp.StatusCode, resp.Proto, protocol)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}
	if body["listener"] != listener || body["message"] != "integration-test" || body["source_ip"] != "127.0.0.1" || body["http_endpoint"] != "/test/path" {
		return fmt.Errorf("unexpected response: %v", body)
	}
	return checkRequestDiagnostics(resp, body)
}

func TestWireAssertionsRejectUnavailableOrInvalidResponses(t *testing.T) {
	client := &http.Client{Timeout: time.Second}
	if err := checkJSON(client, "http://127.0.0.1:"+unusedPort(t, false), "HTTP", "HTTP/1.1"); err == nil {
		t.Fatal("an unavailable listener must fail")
	}
	for _, body := range []string{"", "{}", `{"listener":"HTTP","message":"wrong"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }))
		err := checkJSON(client, server.URL, "HTTP", "HTTP/1.1")
		server.Close()
		if err == nil {
			t.Fatalf("invalid response %q must fail", body)
		}
	}
}
