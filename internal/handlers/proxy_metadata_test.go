package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/connection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/peer"
)

type forwardedMetadataAddr struct {
	net.Addr
	socket net.Addr
}

func (a forwardedMetadataAddr) SocketAddr() net.Addr { return a.socket }

type metadataConn struct {
	net.Conn
	remote, local net.Addr
	output        bytes.Buffer
}

func (c *metadataConn) RemoteAddr() net.Addr        { return c.remote }
func (c *metadataConn) LocalAddr() net.Addr         { return c.local }
func (c *metadataConn) Write(p []byte) (int, error) { return c.output.Write(p) }
func (c *metadataConn) Close() error                { return nil }

func proxyMetadataAddresses() (net.Addr, net.Addr) {
	return forwardedMetadataAddr{
			Addr:   &net.TCPAddr{IP: net.ParseIP("2001:db8::42"), Port: 40500},
			socket: &net.TCPAddr{IP: net.ParseIP("172.16.0.2"), Port: 50000},
		}, forwardedMetadataAddr{
			Addr:   &net.TCPAddr{IP: net.ParseIP("2001:db8::43"), Port: 443},
			socket: &net.TCPAddr{IP: net.ParseIP("172.16.0.3"), Port: 8443},
		}
}

func TestHTTPResponseSeparatesTrustedClientFromSocketPeer(t *testing.T) {
	remote, local := proxyMetadataAddresses()
	req := httptest.NewRequest(http.MethodGet, "https://echo.example/", nil)
	req.RemoteAddr = remote.String()
	req.Header.Set("X-Forwarded-For", "198.51.100.99")
	ctx := connection.WithConn(req.Context(), &metadataConn{remote: remote, local: local})
	ctx = context.WithValue(ctx, http.LocalAddrContextKey, local)
	response := NewHTTPResponse(req.WithContext(ctx), &config.Config{}, "TLS")
	assert.Equal(t, "2001:db8::42", response.SourceIP)
	assert.Equal(t, "IPv6", response.IPVersion)
	assert.Equal(t, "[2001:db8::42]:40500", response.RemoteAddr)
	assert.Equal(t, "[2001:db8::43]:443", response.LocalAddr)
	assert.Equal(t, "IPv6", response.LocalIPVersion)
	assert.Equal(t, "172.16.0.2:50000", response.SocketPeerAddr)
	assert.Equal(t, "172.16.0.2", response.SocketPeerIP)
	assert.Equal(t, "IPv4", response.SocketPeerIPVersion)
	assert.Equal(t, "172.16.0.3:8443", response.SocketLocalAddr)
	assert.Equal(t, "IPv4", response.SocketLocalIPVersion)
}

func TestDirectHTTPResponseDoesNotInventForwardedMetadata(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://echo.example/", nil)
	req.RemoteAddr = "192.0.2.8:40500"
	req.Header.Set("X-Forwarded-For", "2001:db8::99")
	response := NewHTTPResponse(req, &config.Config{}, "HTTP")
	assert.Equal(t, "192.0.2.8", response.SourceIP)
	assert.Equal(t, "IPv4", response.IPVersion)
	data, err := json.Marshal(response)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "socket_peer_")
	assert.NotContains(t, string(data), "socket_local_")
}

func TestGRPCResponseSeparatesTrustedClientFromSocketPeer(t *testing.T) {
	remote, _ := proxyMetadataAddresses()
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: remote})
	response := buildGRPCResponse(ctx, &config.Config{}, "/echo.EchoService/Echo")
	assert.Equal(t, "2001:db8::42", response.SourceIp)
	assert.Equal(t, "IPv6", response.IpVersion)
	assert.Equal(t, "172.16.0.2", response.SocketPeerIp)
	assert.Equal(t, "IPv4", response.SocketPeerIpVersion)
	assert.Equal(t, "172.16.0.2:50000", response.SocketPeerAddr)
}

func TestTCPResponseSeparatesTrustedClientFromSocketPeer(t *testing.T) {
	remote, local := proxyMetadataAddresses()
	conn := &metadataConn{remote: remote, local: local}
	TCPHandler(context.Background(), conn, &config.Config{})
	var response TCPResponse
	require.NoError(t, json.Unmarshal(conn.output.Bytes(), &response))
	assert.Equal(t, "2001:db8::42", response.SourceIP)
	assert.Equal(t, "IPv6", response.IPVersion)
	assert.Equal(t, "172.16.0.2", response.SocketPeerIP)
	assert.Equal(t, "IPv4", response.SocketPeerIPVersion)
	assert.Equal(t, "172.16.0.2:50000", response.SocketPeerAddr)
}
