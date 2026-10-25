package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
)

type budgetTestConn struct {
	net.Conn
	input  *bytes.Reader
	output bytes.Buffer
	closed bool
}

func (c *budgetTestConn) Read(p []byte) (int, error)  { return c.input.Read(p) }
func (c *budgetTestConn) Write(p []byte) (int, error) { return c.output.Write(p) }
func (c *budgetTestConn) Close() error                { c.closed = true; return nil }

func TestTransportBudgetIsSharedAcrossTCPAndQUIC(t *testing.T) {
	guard := protocolTestGuard(t, 1000, 10)
	raw := &budgetTestConn{}
	tcp := protectConn(raw, guard, "TLS")
	n, err := tcp.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, udp.Close()) })
	receiver, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, receiver.Close()) })
	packet := protectPacketConn(udp, guard)
	var _ quic.OOBCapablePacketConn = packet
	n, _, err = packet.WriteMsgUDP([]byte("world"), nil, receiver.LocalAddr().(*net.UDPAddr))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	n, err = packet.WriteTo([]byte("!"), receiver.LocalAddr())
	require.ErrorIs(t, err, protection.ErrDailyEgress)
	require.Zero(t, n)
	n, err = tcp.Write([]byte("!"))
	require.ErrorIs(t, err, protection.ErrDailyEgress)
	require.Zero(t, n)
	require.True(t, raw.closed)
	require.Equal(t, "hello", raw.output.String())
}

func TestProtectedReadBoundsIngressBeforeParsing(t *testing.T) {
	guard, err := protection.NewGuard(protection.Options{Enabled: true, RequestRate: 1000, IngressRate: 1, EgressRate: 1, DailyEgress: 1 << 30})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, guard.Close()) })
	raw := &budgetTestConn{input: bytes.NewReader(bytes.Repeat([]byte("x"), (1<<20)+transferChunk))}
	conn := protectConn(raw, guard, "HTTP")
	buffer := make([]byte, 2*transferChunk)
	for range 16 {
		n, err := conn.Read(buffer)
		require.NoError(t, err)
		require.Equal(t, transferChunk, n)
	}
	n, err := conn.Read(buffer)
	require.ErrorIs(t, err, protection.ErrIngressRate)
	require.Zero(t, n)
	require.True(t, raw.closed)
}

func TestQUICBatchFilteringPreservesBufferOwnership(t *testing.T) {
	guard, err := protection.NewGuard(protection.Options{Enabled: true, RequestRate: 1000, IngressRate: 1, EgressRate: 1, DailyEgress: 1 << 30})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, guard.Close()) })
	require.NoError(t, guard.AllowIngress((1<<20)-64))
	packet := &protectedPacketConn{guard: guard, received: metrics.TransportBytes("QUIC", "received")}
	first, second := make([]byte, 128), bytes.Repeat([]byte("b"), 128)
	firstOOB, secondOOB := make([]byte, 16), bytes.Repeat([]byte("o"), 16)
	peer := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234}
	messages := []ipv4.Message{
		{Buffers: [][]byte{first}, OOB: firstOOB, N: 65},
		{Buffers: [][]byte{second}, OOB: secondOOB, N: 12, NN: 3, Flags: 1, Addr: peer},
	}
	require.Equal(t, 1, packet.admitBatch(messages))
	require.Same(t, &first[0], &messages[0].Buffers[0][0])
	require.Same(t, &second[0], &messages[1].Buffers[0][0])
	require.Same(t, &firstOOB[0], &messages[0].OOB[0])
	require.Equal(t, []byte("bbbbbbbbbbbb"), messages[0].Buffers[0][:12])
	require.Equal(t, []byte("ooo"), messages[0].OOB[:3])
	require.Equal(t, 12, messages[0].N)
	require.Equal(t, peer, messages[0].Addr)
	require.False(t, packet.admit(0), "empty datagrams also consume a minimum ingress charge")
}

func TestQUICClosedBatchSocketReturnsError(t *testing.T) {
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	packet := protectPacketConn(udp, nil)
	require.NoError(t, udp.Close())
	n, err := packet.ReadBatch([]ipv4.Message{{Buffers: [][]byte{make([]byte, 128)}}}, 0)
	require.Error(t, err)
	require.Zero(t, n)
}

func TestHTTPProtectionAppliesToKeepAliveAcrossProtocols(t *testing.T) {
	fixture := newTLSFixture(t)
	for _, protocol := range []struct {
		name, scheme string
		tls, h2c     bool
		major        int
	}{
		{"HTTP1", "http", false, false, 1}, {"H2C", "http", false, true, 2},
		{"HTTPS1", "https", true, false, 1}, {"HTTPS2", "https", true, false, 2},
	} {
		t.Run(protocol.name, func(t *testing.T) {
			cfg := fixture.cfg
			cfg.TLSClientCAFile, cfg.H2C = "", protocol.h2c
			guard := protocolTestGuard(t, 0.001, 1<<30)
			srv := NewHTTPServer(&cfg, protocol.tls, nil)
			srv.SetProtection(guard)
			address := startHTTPBoundaryServer(t, srv)
			transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: fixture.roots, ServerName: "localhost"}, ForceAttemptHTTP2: protocol.major == 2}
			if protocol.h2c {
				transport.Protocols = new(http.Protocols)
				transport.Protocols.SetUnencryptedHTTP2(true)
			}
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			for _, path := range []string{"/", "/ui/config.json"} {
				response, err := client.Get(protocol.scheme + "://" + address + path)
				require.NoError(t, err)
				_, err = io.Copy(io.Discard, response.Body)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				require.Equal(t, protocol.major, response.ProtoMajor)
				if path == "/" {
					require.Equal(t, http.StatusOK, response.StatusCode)
				} else {
					require.Equal(t, http.StatusTooManyRequests, response.StatusCode)
					require.Equal(t, "1", response.Header.Get("Retry-After"))
				}
			}
		})
	}
}

func TestHTTP3ProtectionCountsWireBytesAndRejectsStreams(t *testing.T) {
	fixture := newTLSFixture(t)
	fixture.cfg.TLSClientCAFile = ""
	srv := NewQUICServer(&fixture.cfg, nil)
	srv.listenAddr = "127.0.0.1:0"
	guard := protocolTestGuard(t, 0.001, 1<<30)
	srv.SetProtection(guard)
	before := testutil.ToFloat64(metrics.TransportBytes("QUIC", "sent"))
	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()
	select {
	case <-srv.Ready():
	case err := <-done:
		t.Fatalf("QUIC start: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("QUIC startup timeout")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
		require.ErrorIs(t, <-done, http.ErrServerClosed)
	})
	transport := &http3.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: fixture.roots, ServerName: "localhost"}}
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	for _, status := range []int{http.StatusOK, http.StatusTooManyRequests} {
		response, err := client.Get("https://" + srv.packet.LocalAddr().String() + "/")
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, status, response.StatusCode)
	}
	require.Greater(t, testutil.ToFloat64(metrics.TransportBytes("QUIC", "sent")), before)
	require.Greater(t, guard.Snapshot().Reserved, int64(0))
}

func TestAdminRemainsAvailableAfterPublicBudgetExhaustion(t *testing.T) {
	guard := protocolTestGuard(t, 1000, 1)
	require.NoError(t, guard.ReserveEgress(1))
	raw := &budgetTestConn{}
	_, err := protectConn(raw, guard, "HTTP").Write([]byte("x"))
	require.ErrorIs(t, err, protection.ErrDailyEgress)
	admin := NewMetricsServer(&config.Config{}, nil)
	admin.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- admin.Start(context.Background()) }()
	select {
	case <-admin.Ready():
	case err := <-done:
		t.Fatalf("admin start: %v", err)
	case <-time.After(time.Second):
		t.Fatal("admin startup timeout")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, admin.Shutdown(ctx))
		require.NoError(t, <-done)
	})
	for _, path := range []string{"/health", "/ready", "/metrics"} {
		response := httptest.NewRecorder()
		admin.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, response.Code, path)
	}
}
