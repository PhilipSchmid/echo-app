//go:build integration

package integration_test

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func TestUIConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test builds and launches the application")
	}
	binary := filepath.Join(t.TempDir(), "echo-observatory")
	if output, err := exec.Command("go", "build", "-o", binary, "../cmd/echo-observatory").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	for _, tc := range []struct {
		name, env, flag string
		enabled         bool
	}{
		{"default", "", "", true},
		{"flag opt-out", "true", "--ui=false", false},
		{"environment opt-out", "false", "", false},
		{"flag overrides environment", "false", "--ui=true", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port := unusedPort(t)
			quicPort := unusedPort(t)
			args := []string{"--http-port=" + port, "--tls", "--quic", "--tls-port=" + quicPort, "--quic-port=" + quicPort, "--metrics=false"}
			if tc.flag != "" {
				args = append(args, tc.flag)
			}
			cmd := exec.Command(binary, args...)
			for _, value := range os.Environ() {
				if !strings.HasPrefix(value, "ECHO_OBSERVATORY_") {
					cmd.Env = append(cmd.Env, value)
				}
			}
			if tc.env != "" {
				cmd.Env = append(cmd.Env, "ECHO_OBSERVATORY_UI="+tc.env)
			}
			log, err := os.Create(filepath.Join(t.TempDir(), "server.log"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = log.Close() }()
			cmd.Stdout, cmd.Stderr = log, log
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			defer func() {
				_ = cmd.Process.Signal(syscall.SIGTERM)
				select {
				case err := <-done:
					if err != nil {
						t.Errorf("shutdown: %v", err)
					}
				case <-time.After(5 * time.Second):
					_ = cmd.Process.Kill()
					t.Logf("forced shutdown: %v", <-done)
					t.Error("shutdown exceeded five seconds")
				}
				if t.Failed() {
					data, _ := os.ReadFile(log.Name())
					t.Logf("server log:\n%s", data)
				}
			}()
			client := &http.Client{Timeout: time.Second}
			defer client.CloseIdleConnections()
			base := "http://127.0.0.1:" + port
			deadline := time.Now().Add(5 * time.Second)
			for {
				response, err := client.Get(base + "/")
				if err == nil {
					var body map[string]any
					err = json.NewDecoder(response.Body).Decode(&body)
					_ = response.Body.Close()
					if err != nil || body["listener"] != "HTTP" {
						t.Fatalf("root stopped returning echo metadata: %v, %v", body, err)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("application did not start: %v", err)
				}
				time.Sleep(20 * time.Millisecond)
			}
			h3 := &http3.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // Local demo certificate.
			defer func() { _ = h3.Close() }()
			h3Client := &http.Client{Transport: h3, Timeout: 3 * time.Second}
			response3, err := h3Client.Get("https://127.0.0.1:" + quicPort + "/ui/")
			if err != nil {
				t.Fatal(err)
			}
			data3, err := io.ReadAll(response3.Body)
			_ = response3.Body.Close()
			if err != nil || response3.Proto != "HTTP/3.0" || response3.StatusCode != 200 {
				t.Fatalf("HTTP/3 UI: %v, %v", response3, err)
			}
			if tc.enabled {
				if !strings.HasPrefix(response3.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(data3), "HTTP/3.0") {
					t.Fatalf("HTTP/3 UI not rendered: %s", data3)
				}
			} else {
				var body map[string]any
				if err := json.Unmarshal(data3, &body); err != nil || body["listener"] != "QUIC" {
					t.Fatalf("disabled HTTP/3 UI: %s, %v", data3, err)
				}
			}
			response, err := client.Get(base + "/ui/")
			if err != nil {
				t.Fatal(err)
			}
			data, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil || response.StatusCode != http.StatusOK {
				t.Fatalf("UI status=%d, read error=%v", response.StatusCode, readErr)
			}
			isHTML := strings.HasPrefix(response.Header.Get("Content-Type"), "text/html")
			if isHTML != tc.enabled {
				t.Fatalf("UI enabled=%v, content type=%q", tc.enabled, response.Header.Get("Content-Type"))
			}
			if !tc.enabled {
				var body map[string]any
				if err := json.Unmarshal(data, &body); err != nil || body["listener"] != "HTTP" {
					t.Fatalf("disabled UI must preserve metadata fallback: %s, %v", data, err)
				}
			}
			for _, target := range []struct {
				client   *http.Client
				base     string
				listener string
			}{
				{client, base, "HTTP"},
				{h3Client, "https://127.0.0.1:" + quicPort, "QUIC"},
			} {
				for _, path := range []string{"/ui/workbench/", "/ui/privacy/", "/ui/privacy.js"} {
					response, err := target.client.Get(target.base + path)
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(response.Body)
					_ = response.Body.Close()
					if err != nil || response.StatusCode != http.StatusOK {
						t.Fatalf("%s: status=%d error=%v", path, response.StatusCode, err)
					}
					if tc.enabled {
						switch path {
						case "/ui/workbench/":
							if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(data), `id="request-form"`) || strings.Contains(string(data), `id="visit-body"`) {
								t.Fatalf("workbench not separate from received request: %s", data)
							}
						case "/ui/privacy/":
							if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(data), "<h1>Privacy &amp; data</h1>") || !strings.Contains(string(data), `src="../privacy.js"`) {
								t.Fatalf("privacy page not rendered: %s", data)
							}
						case "/ui/privacy.js":
							if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/javascript") || !strings.Contains(string(data), "initTheme") {
								t.Fatalf("privacy module not served: %s", data)
							}
						}
					} else {
						var body map[string]any
						if err := json.Unmarshal(data, &body); err != nil || body["listener"] != target.listener {
							t.Fatalf("disabled %s must preserve metadata fallback: %s, %v", path, data, err)
						}
					}
				}
			}
		})
	}
}

func checkUIOverProtocol(t *testing.T, client *http.Client, base, protocol, listener string) {
	t.Helper()
	for _, path := range []string{"/ui/", "/ui/workbench/", "/ui/examples/", "/ui/connect/", "/ui/privacy/", "/ui/config.json", "/ui/certificate.json", "/ui/certificate.js", "/ui/app.js", "/ui/headers.js", "/ui/visit.js", "/ui/common.js", "/ui/guide.js", "/ui/controls.js", "/ui/presets.js", "/ui/highlight.js", "/ui/terminal.js", "/ui/privacy.js", "/ui/app.css", "/ui/logo.svg", "/ui/favicon.svg", "/ui/nunito-latin.woff2", "/ui/sometype-mono-latin.woff2", "/robots.txt", "/sitemap.xml"} {
		response, err := client.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || response.Proto != protocol || len(data) == 0 {
			t.Fatalf("%s: status=%d proto=%s err=%v", path, response.StatusCode, response.Proto, err)
		}
		cacheControl := "no-store"
		if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".woff2") {
			cacheControl = "public, max-age=0, must-revalidate"
		}
		if response.Header.Get("Cache-Control") != cacheControl || response.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("missing UI protections: %s", path)
		}
		if path == "/robots.txt" && !strings.Contains(string(data), "Sitemap: https://echo.container.education/sitemap.xml") {
			t.Fatal("missing public sitemap discovery")
		}
		if path == "/sitemap.xml" && !strings.Contains(string(data), "https://echo.container.education/ui/workbench/") {
			t.Fatal("missing workbench sitemap entry")
		}
		if path == "/sitemap.xml" && !strings.Contains(string(data), "https://echo.container.education/ui/privacy/") {
			t.Fatal("missing privacy sitemap entry")
		}
		if strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
			privacyLink := `href="../privacy/"`
			if path == "/ui/" {
				privacyLink = `href="./privacy/"`
			}
			if !strings.Contains(string(data), privacyLink) {
				t.Fatalf("missing privacy link on %s", path)
			}
		}
		if path == "/ui/privacy/" && (!strings.Contains(string(data), "<title>Privacy &amp; data — Echo Observatory</title>") || !strings.Contains(string(data), "<h1>Privacy &amp; data</h1>") || !strings.Contains(string(data), `src="../privacy.js"`)) {
			t.Fatalf("privacy page not rendered over %s", protocol)
		}
		if path == "/ui/privacy.js" && !strings.Contains(string(data), "initTheme") {
			t.Fatalf("privacy module not served over %s", protocol)
		}
		if path == "/ui/certificate.json" {
			var details struct {
				Mode        string `json:"mode"`
				Certificate *struct {
					Fingerprint string `json:"sha256_fingerprint"`
				} `json:"certificate"`
			}
			if err := json.Unmarshal(data, &details); err != nil || details.Mode != "demo" || details.Certificate == nil {
				t.Fatalf("missing native certificate details over %s: %s, %v", protocol, data, err)
			}
			if response.TLS != nil && len(response.TLS.PeerCertificates) > 0 {
				fingerprint := sha256.Sum256(response.TLS.PeerCertificates[0].Raw)
				if !strings.EqualFold(strings.ReplaceAll(details.Certificate.Fingerprint, ":", ""), hex.EncodeToString(fingerprint[:])) {
					t.Fatalf("UI certificate fingerprint differs from the certificate served over %s", protocol)
				}
			}
		}
		if path == "/ui/" && (!strings.Contains(string(data), "<dd>"+protocol+"</dd>") || !strings.Contains(string(data), "<dd>"+listener+"</dd>")) {
			t.Fatalf("wrong navigation diagnostics: %s", data)
		}
		if path == "/ui/" {
			transport := "TCP"
			if protocol == "HTTP/3.0" {
				transport = "QUIC / UDP"
				if !strings.Contains(string(data), "TLS is integrated into QUIC.") {
					t.Fatal("missing QUIC security explanation")
				}
			}
			if !strings.Contains(string(data), `id="connection-stack"`) || !strings.Contains(string(data), "<p>"+transport+"</p>") {
				t.Fatalf("wrong layer transport for %s", protocol)
			}
		}
		if path == "/ui/workbench/" && (!strings.Contains(string(data), `id="request-form"`) || !strings.Contains(string(data), `id="websocket-form"`) || strings.Contains(string(data), `id="visit-body"`) || strings.Contains(string(data), `id="connection-stack"`)) {
			t.Fatalf("workbench page separation failed over %s", protocol)
		}
		if strings.HasPrefix(base, "https:") && protocol != "HTTP/3.0" && response.Header.Get("Alt-Svc") == "" {
			t.Fatal("missing HTTP/3 discovery")
		}
	}
	response, err := client.Get(base + "/ws")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("WebSocket without upgrade: %d", response.StatusCode)
	}
}
