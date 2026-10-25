// Package connection preserves transport identity across trusted forwarding.
package connection

import (
	"context"
	"net"
)

type connKey struct{}

// WithConn stores the connection without inspecting its lazy PROXY header.
func WithConn(ctx context.Context, conn net.Conn) context.Context {
	return context.WithValue(ctx, connKey{}, conn)
}

// RemoteAddr is called after the protocol handler has read the connection.
func RemoteAddr(ctx context.Context) net.Addr {
	if conn, ok := ctx.Value(connKey{}).(net.Conn); ok {
		return conn.RemoteAddr()
	}
	return nil
}

// SocketAddr returns the real socket endpoint only for validated forwarding.
func SocketAddr(addr net.Addr) net.Addr {
	if forwarded, ok := addr.(interface{ SocketAddr() net.Addr }); ok {
		return forwarded.SocketAddr()
	}
	return nil
}
