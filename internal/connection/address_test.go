package connection

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

type unreadConn struct{ net.Conn }

func (unreadConn) RemoteAddr() net.Addr { panic("header inspected before protocol goroutine") }

func TestWithConnDoesNotInspectAddress(t *testing.T) {
	ctx := WithConn(context.Background(), unreadConn{})
	require.NotNil(t, ctx)
	require.Nil(t, RemoteAddr(context.Background()))
	require.Nil(t, SocketAddr(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80}))
}
