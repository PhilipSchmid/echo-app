package server

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	proxyproto "github.com/pires/go-proxyproto"
)

const proxyHeaderTimeout = 5 * time.Second

var errProxyHeader = errors.New("invalid or missing trusted PROXY header")

func publicTCPListener(ln net.Listener, cfg *config.Config, guard *protection.Guard, name string) (net.Listener, error) {
	protected := protectListener(ln, guard, name)
	if !cfg.ProxyProtocol {
		return protected, nil
	}
	if len(cfg.ProxyTrustedCIDRs) == 0 {
		return nil, errors.New("PROXY protocol requires trusted proxy CIDRs")
	}
	policy, err := proxyproto.TrustProxyHeaderFromRanges(cfg.ProxyTrustedCIDRs)
	if err != nil {
		return nil, err
	}
	protected.proxy = true
	parsed := &proxyproto.Listener{
		Listener: protected, ReadHeaderTimeout: proxyHeaderTimeout, ValidateHeader: validateProxyHeader,
		ConnPolicy: func(opts proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
			result, err := policy(opts)
			if err != nil {
				metrics.RecordRejection(name, "proxy_peer")
			}
			return result, err
		},
	}
	return &forwardingListener{Listener: parsed, guard: guard, name: name}, nil
}

func validateProxyHeader(h *proxyproto.Header) error {
	if h.Command.IsLocal() {
		return nil
	}
	if h.TransportProtocol != proxyproto.TCPv4 && h.TransportProtocol != proxyproto.TCPv6 {
		return errProxyHeader
	}
	for _, addr := range []net.Addr{h.SourceAddr, h.DestinationAddr} {
		tcp, ok := addr.(*net.TCPAddr)
		if !ok || tcp.IP == nil || tcp.IP.IsUnspecified() || tcp.IP.IsMulticast() {
			return errProxyHeader
		}
	}
	// TLVs are untrusted metadata; validate framing without using TLS/identity claims.
	_, err := h.TLVs()
	return err
}

type forwardingListener struct {
	net.Listener
	guard *protection.Guard
	name  string
}

func (l *forwardingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	pc := conn.(*proxyproto.Conn) // The required-header policy never returns raw connections.
	return &forwardingConn{Conn: pc, parsed: pc, peer: pc.Raw().RemoteAddr(), local: pc.Raw().LocalAddr(), guard: l.guard, name: l.name}, nil
}

type forwardingConn struct {
	net.Conn
	parsed      *proxyproto.Conn
	peer, local net.Addr
	guard       *protection.Guard
	name        string
	once        sync.Once
	err         error
	remoteAddr  net.Addr
	localAddr   net.Addr
}

func (c *forwardingConn) prepare() error {
	c.once.Do(func() {
		c.remoteAddr, c.localAddr = c.peer, c.local
		h := c.parsed.ProxyHeader()
		if h == nil {
			c.err = errProxyHeader
			metrics.RecordRejection(c.name, "proxy_header")
		} else {
			if !h.Command.IsLocal() {
				c.remoteAddr = forwardedAddr{Addr: h.SourceAddr, socket: c.peer}
				c.localAddr = forwardedAddr{Addr: h.DestinationAddr, socket: c.local}
			}
			c.err = c.guard.AllowPeerConnection(c.remoteAddr.String())
			if c.err != nil {
				metrics.RecordRejection(c.name, c.err.Error())
			}
		}
		if c.err != nil {
			_ = c.Close()
		}
	})
	return c.err
}

func (c *forwardingConn) RemoteAddr() net.Addr { _ = c.prepare(); return c.remoteAddr }
func (c *forwardingConn) LocalAddr() net.Addr  { _ = c.prepare(); return c.localAddr }

func (c *forwardingConn) Read(b []byte) (int, error) {
	if err := c.prepare(); err != nil {
		return 0, err
	}
	return c.Conn.Read(b)
}

func (c *forwardingConn) Write(b []byte) (int, error) {
	if err := c.prepare(); err != nil {
		return 0, err
	}
	return c.Conn.Write(b)
}

type forwardedAddr struct {
	net.Addr
	socket net.Addr
}

func (a forwardedAddr) SocketAddr() net.Addr { return a.socket }
