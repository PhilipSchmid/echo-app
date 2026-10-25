package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/connection"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	pb "github.com/PhilipSchmid/echo-observatory/v2/proto"
	proxyproto "github.com/pires/go-proxyproto"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func proxyWireHeader(t *testing.T, version byte, ipv6 bool) ([]byte, *net.TCPAddr, *net.TCPAddr) {
	t.Helper()
	source, destination := "198.51.100.12", "203.0.113.24"
	if ipv6 {
		source, destination = "2001:db8:1::12", "2001:db8:2::24"
	}
	src := &net.TCPAddr{IP: net.ParseIP(source), Port: 32123}
	dst := &net.TCPAddr{IP: net.ParseIP(destination), Port: 443}
	header, err := proxyproto.HeaderProxyFromAddrs(version, src, dst).Format()
	require.NoError(t, err)
	return header, src, dst
}

func proxyListenerForTest(t *testing.T, trusted []string) net.Listener {
	t.Helper()
	raw, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	listener, err := publicTCPListener(raw, &config.Config{ProxyProtocol: true, ProxyTrustedCIDRs: trusted}, nil, "HTTP")
	if err != nil {
		_ = raw.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func proxyClientForTest(t *testing.T, listener net.Listener, wire []byte) net.Conn {
	t.Helper()
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, client.SetDeadline(time.Now().Add(2*time.Second)))
	if wire != nil {
		_, err = client.Write(wire)
		require.NoError(t, err)
	}
	return client
}

func TestProxyProtocolWireAddressesAndCoalescedPayload(t *testing.T) {
	for _, version := range []byte{1, 2} {
		for _, ipv6 := range []bool{false, true} {
			name := string(rune('0'+version)) + "/IPv4"
			if ipv6 {
				name = string(rune('0'+version)) + "/IPv6"
			}
			t.Run(name, func(t *testing.T) {
				listener := proxyListenerForTest(t, []string{"127.0.0.1/32"})
				header, source, destination := proxyWireHeader(t, version, ipv6)
				client := proxyClientForTest(t, listener, append(header, []byte("payload")...))
				conn, err := listener.Accept()
				require.NoError(t, err)
				defer func() { _ = conn.Close() }()
				got := make([]byte, len("payload"))
				_, err = io.ReadFull(conn, got)
				require.NoError(t, err)
				require.Equal(t, "payload", string(got), "header parsing must not consume coalesced application bytes")
				require.Equal(t, source.String(), conn.RemoteAddr().String())
				require.Equal(t, destination.String(), conn.LocalAddr().String())
				peer, local := connection.SocketAddr(conn.RemoteAddr()), connection.SocketAddr(conn.LocalAddr())
				require.NotNil(t, peer)
				require.NotNil(t, local)
				require.Equal(t, client.LocalAddr().String(), peer.String())
				require.Equal(t, listener.Addr().String(), local.String())
			})
		}
	}
}

func TestProxyProtocolLocalAndUnknownRetainSocketIdentity(t *testing.T) {
	local, err := (&proxyproto.Header{Version: 2, Command: proxyproto.LOCAL, TransportProtocol: proxyproto.UNSPEC}).Format()
	require.NoError(t, err)
	for _, test := range []struct {
		name string
		wire []byte
	}{{"v1 UNKNOWN", []byte("PROXY UNKNOWN\r\n")}, {"v2 LOCAL", local}} {
		t.Run(test.name, func(t *testing.T) {
			listener := proxyListenerForTest(t, []string{"127.0.0.1/32"})
			client := proxyClientForTest(t, listener, append(test.wire, 'x'))
			conn, err := listener.Accept()
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			_, err = io.ReadFull(conn, make([]byte, 1))
			require.NoError(t, err)
			require.Equal(t, client.LocalAddr().String(), conn.RemoteAddr().String())
			require.Equal(t, listener.Addr().String(), conn.LocalAddr().String())
			require.Nil(t, connection.SocketAddr(conn.RemoteAddr()), "LOCAL/UNKNOWN must not claim an original client")
		})
	}
}

func TestProxyProtocolRejectsInvalidHeaders(t *testing.T) {
	header, _, _ := proxyWireHeader(t, 2, false)
	badTLV := append(bytes.Clone(header), 0xea, 0, 5, 'x')
	binary.BigEndian.PutUint16(badTLV[14:16], uint16(len(badTLV)-16))
	truncated := bytes.Clone(header[:20])
	for _, test := range []struct {
		name string
		wire []byte
	}{
		{"missing header", []byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")},
		{"malformed address", []byte("PROXY TCP4 invalid 203.0.113.1 1234 80\r\n")},
		{"oversize v1", []byte("PROXY UNKNOWN " + strings.Repeat("a", 108) + "\r\n")},
		{"truncated v2", truncated},
		{"truncated TLV", badTLV},
		{"UDP header on TCP", append(append(bytes.Clone(header[:13]), byte(proxyproto.UDPv4)), header[14:]...)},
	} {
		t.Run(test.name, func(t *testing.T) {
			listener := proxyListenerForTest(t, []string{"127.0.0.1/32"})
			client := proxyClientForTest(t, listener, test.wire)
			require.NoError(t, client.(*net.TCPConn).CloseWrite())
			conn, err := listener.Accept()
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			_, err = conn.Read(make([]byte, 1))
			require.Error(t, err)
		})
	}
}

func TestProxyProtocolRejectsUntrustedPeer(t *testing.T) {
	listener := proxyListenerForTest(t, []string{"192.0.2.10/32"})
	header, _, _ := proxyWireHeader(t, 2, false)
	client := proxyClientForTest(t, listener, append(header, 'x'))
	type result struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan result, 1)
	go func() {
		conn, err := listener.Accept()
		accepted <- result{conn, err}
	}()
	_, err := client.Read(make([]byte, 1))
	require.Error(t, err, "an untrusted peer must be disconnected")
	require.NoError(t, listener.Close())
	select {
	case got := <-accepted:
		if got.conn != nil {
			_ = got.conn.Close()
		}
		require.Nil(t, got.conn, "a valid claimed address must not bypass the real peer allowlist")
		require.Error(t, got.err)
	case <-time.After(time.Second):
		t.Fatal("listener close did not unblock accept")
	}
}

func TestProxyProtocolHeaderDeadlineAndIdleAccept(t *testing.T) {
	listener := proxyListenerForTest(t, []string{"127.0.0.1/32"})
	parsed := listener.(*forwardingListener).Listener.(*proxyproto.Listener)
	require.Equal(t, 5*time.Second, parsed.ReadHeaderTimeout)
	parsed.ReadHeaderTimeout = 50 * time.Millisecond
	_ = proxyClientForTest(t, listener, nil)
	first, err := listener.Accept()
	require.NoError(t, err)
	defer func() { _ = first.Close() }()
	header, _, _ := proxyWireHeader(t, 1, false)
	_ = proxyClientForTest(t, listener, append(header, 'x'))
	second, err := listener.Accept()
	require.NoError(t, err, "an idle trusted peer must not block the accept loop")
	defer func() { _ = second.Close() }()
	_, err = io.ReadFull(second, make([]byte, 1))
	require.NoError(t, err)
	started := time.Now()
	_, err = first.Read(make([]byte, 1))
	require.Error(t, err)
	require.Less(t, time.Since(started), time.Second, "missing trusted header must time out")
}

func proxyDialer(header []byte) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if err := conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
			_ = conn.Close()
			return nil, err
		}
		if _, err := conn.Write(header); err != nil {
			_ = conn.Close()
			return nil, err
		}
		if err := conn.SetWriteDeadline(time.Time{}); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return conn, nil
	}
}

func startProxyHTTPServer(t *testing.T, cfg *config.Config, secure bool) (*HTTPServer, string) {
	t.Helper()
	srv := NewHTTPServer(cfg, secure, nil)
	srv.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()
	select {
	case <-srv.Ready():
	case err := <-done:
		t.Fatalf("HTTP listener failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP listener did not bind")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
		require.ErrorIs(t, <-done, http.ErrServerClosed)
	})
	return srv, srv.netListener.Addr().String()
}

func TestProxyProtocolPreservesHTTPAndTLSMetadata(t *testing.T) {
	fixture := newTLSFixture(t)
	for _, test := range []struct {
		name          string
		version       byte
		ipv6, tls, h2 bool
	}{
		{"HTTP1", 1, false, false, false},
		{"h2c", 2, true, false, true},
		{"HTTPS1", 2, false, true, false},
		{"HTTPS2", 1, true, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := fixture.cfg
			cfg.ProxyProtocol, cfg.ProxyTrustedCIDRs = true, []string{"127.0.0.1/32"}
			cfg.H2C = !test.tls && test.h2
			_, address := startProxyHTTPServer(t, &cfg, test.tls)
			header, source, destination := proxyWireHeader(t, test.version, test.ipv6)
			if !test.tls && test.version == 2 {
				// A forwarding TLV must not impersonate this app's actual TLS handshake.
				header = append(header, 0x20, 0, 5, 1, 0, 0, 0, 0)
				binary.BigEndian.PutUint16(header[14:16], uint16(len(header)-16))
			}
			dial := proxyDialer(header)
			clientTLS := &tls.Config{RootCAs: fixture.roots, ServerName: "localhost", Certificates: []tls.Certificate{fixture.client}, MinVersion: tls.VersionTLS12}
			protocols := new(http.Protocols)
			protocols.SetHTTP1(test.tls || !test.h2)
			protocols.SetHTTP2(test.tls && test.h2)
			protocols.SetUnencryptedHTTP2(!test.tls && test.h2)
			transport := &http.Transport{DialContext: dial, TLSClientConfig: clientTLS, Protocols: protocols}
			defer transport.CloseIdleConnections()
			scheme := "http"
			if test.tls {
				scheme = "https"
			}
			request, err := http.NewRequest(http.MethodGet, scheme+"://"+address+"/", nil)
			require.NoError(t, err)
			request.Header.Set("X-Forwarded-For", "203.0.113.250")
			response, err := (&http.Client{Transport: transport, Timeout: 3 * time.Second}).Do(request)
			require.NoError(t, err)
			defer func() { _ = response.Body.Close() }()
			require.Equal(t, http.StatusOK, response.StatusCode)
			major, family := 1, "IPv4"
			if test.h2 {
				major = 2
			}
			if test.ipv6 {
				family = "IPv6"
			}
			require.Equal(t, major, response.ProtoMajor)
			var body handlers.HTTPResponse
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			require.Equal(t, source.IP.String(), body.SourceIP)
			require.Equal(t, family, body.IPVersion)
			require.Equal(t, source.String(), body.RemoteAddr)
			require.Equal(t, destination.String(), body.LocalAddr)
			require.Equal(t, "127.0.0.1", body.SocketPeerIP)
			require.Equal(t, "IPv4", body.SocketPeerIPVersion)
			require.Equal(t, address, body.SocketLocalAddr)
			if test.tls {
				require.NotNil(t, response.TLS)
				require.NotEmpty(t, response.TLS.VerifiedChains, "TLS must still terminate at and authenticate the app")
				require.NotEmpty(t, body.TLSVersion)
				require.Equal(t, "localhost", body.TLSServerName)
			} else {
				require.Empty(t, body.TLSVersion)
				require.Empty(t, body.TLSCipherSuite)
			}
		})
	}
}

func TestProxyProtocolPreservesGRPCTLSMetadata(t *testing.T) {
	fixture := newTLSFixture(t)
	fixture.cfg.ProxyProtocol, fixture.cfg.ProxyTrustedCIDRs = true, []string{"127.0.0.1/32"}
	srv := NewGRPCServer(&fixture.cfg, nil, nil)
	address := startHardenedGRPCTestServer(t, srv)
	header, source, _ := proxyWireHeader(t, 2, true)
	dial := proxyDialer(header)
	client, err := grpc.NewClient(address,
		grpc.WithContextDialer(func(ctx context.Context, address string) (net.Conn, error) { return dial(ctx, "tcp", address) }),
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: fixture.roots, ServerName: "localhost", Certificates: []tls.Certificate{fixture.client}, MinVersion: tls.VersionTLS12})))
	require.NoError(t, err)
	defer func() { require.NoError(t, client.Close()) }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := pb.NewEchoServiceClient(client).Echo(ctx, &pb.EchoRequest{})
	require.NoError(t, err)
	require.Equal(t, source.IP.String(), response.SourceIp)
	require.Equal(t, "IPv6", response.IpVersion)
	require.Equal(t, "127.0.0.1", response.SocketPeerIp)
	require.Equal(t, "IPv4", response.SocketPeerIpVersion)
}

func TestProxyProtocolNativeTCPAndIdleShutdown(t *testing.T) {
	cfg := &config.Config{ProxyProtocol: true, ProxyTrustedCIDRs: []string{"127.0.0.1/32"}}
	srv := NewTCPServer(cfg)
	srv.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()
	select {
	case <-srv.Ready():
	case <-time.After(3 * time.Second):
		t.Fatal("TCP listener did not bind")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
		require.NoError(t, <-done)
	})
	idle, err := net.DialTimeout("tcp", srv.listener.Addr().String(), time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = idle.Close() })
	header, source, _ := proxyWireHeader(t, 2, false)
	conn, err := proxyDialer(header)(context.Background(), "tcp", srv.listener.Addr().String())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetDeadline(time.Now().Add(time.Second)))
	var response handlers.BaseResponse
	require.NoError(t, json.NewDecoder(conn).Decode(&response))
	require.Equal(t, source.IP.String(), response.SourceIP)
	require.Equal(t, "127.0.0.1", response.SocketPeerIP)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, srv.Shutdown(ctx), "idle pre-header clients must not delay TCP shutdown")
}

func TestProxyProtocolHTTPIdleShutdownHonorsContext(t *testing.T) {
	cfg := &config.Config{ProxyProtocol: true, ProxyTrustedCIDRs: []string{"127.0.0.1/32"}, MaxRequestSize: 1024}
	srv, address := startProxyHTTPServer(t, cfg, false)
	idle, err := net.DialTimeout("tcp", address, time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = idle.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := time.Now()
	if err := srv.Shutdown(ctx); err != nil {
		require.ErrorIs(t, err, context.DeadlineExceeded)
	}
	require.Less(t, time.Since(started), 2*time.Second, "forced shutdown must respect its timeout")
	require.NoError(t, idle.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = idle.Read(make([]byte, 1))
	require.Error(t, err, "forced shutdown must close the pre-header connection")
}
