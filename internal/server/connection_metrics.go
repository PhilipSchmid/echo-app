package server

import (
	"net"
	"sync"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
)

type meteredListener struct {
	net.Listener
	name string
}

func trackConnections(listener net.Listener, name string) net.Listener {
	return &meteredListener{Listener: listener, name: name}
}

func (l *meteredListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	metrics.ConnectionOpened(l.name)
	return &meteredConn{Conn: conn, name: l.name}, nil
}

type meteredConn struct {
	net.Conn
	name string
	once sync.Once
}

func (c *meteredConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { metrics.ConnectionClosed(c.name) })
	return err
}
