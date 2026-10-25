package server

import (
	"io"
	"net"
	"net/http"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/ipv4"
)

const transferChunk = 64 << 10

type protectedListener struct {
	net.Listener
	guard *protection.Guard
	name  string
	proxy bool
}

func protectListener(ln net.Listener, guard *protection.Guard, name string) *protectedListener {
	return &protectedListener{Listener: ln, guard: guard, name: name}
}

func (l *protectedListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		var admissionErr error
		if l.proxy {
			admissionErr = l.guard.AllowProxyConnection()
		} else {
			admissionErr = l.guard.AllowConnection(conn.RemoteAddr().String())
		}
		if err := admissionErr; err != nil {
			metrics.RecordRejection(l.name, err.Error())
			_ = conn.Close()
			time.Sleep(time.Millisecond)
			continue
		}
		return protectConn(conn, l.guard, l.name), nil
	}
}

type protectedConn struct {
	net.Conn
	guard          *protection.Guard
	name           string
	received, sent prometheus.Counter
}

func protectConn(conn net.Conn, guard *protection.Guard, name string) net.Conn {
	return &protectedConn{Conn: conn, guard: guard, name: name,
		received: metrics.TransportBytes(name, "received"), sent: metrics.TransportBytes(name, "sent")}
}

func (c *protectedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p[:min(len(p), transferChunk)])
	c.received.Add(float64(n))
	if limitErr := c.guard.AllowIngress(n); limitErr != nil {
		metrics.RecordRejection(c.name, limitErr.Error())
		_ = c.Close()
		return 0, limitErr
	}
	return n, err
}

func (c *protectedConn) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		chunk := p[:min(len(p), transferChunk)]
		if err := c.guard.ReserveEgress(len(chunk)); err != nil {
			metrics.RecordRejection(c.name, err.Error())
			_ = c.Close()
			return written, err
		}
		n, err := c.Conn.Write(chunk)
		c.sent.Add(float64(n))
		written += n
		if err != nil {
			return written, err
		}
		if n != len(chunk) {
			return written, io.ErrShortWrite
		}
		p = p[n:]
	}
	return written, nil
}

func protectHTTP(getGuard func() *protection.Guard, next http.Handler, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := getGuard().AllowOperation(r.RemoteAddr); err != nil {
			metrics.RecordRejection(name, err.Error())
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Request rate limit reached", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Keep QUIC's OOB and batch I/O while accounting below encryption and retransmission.
type protectedPacketConn struct {
	*net.UDPConn
	batch          *ipv4.PacketConn
	guard          *protection.Guard
	received, sent prometheus.Counter
}

func protectPacketConn(conn *net.UDPConn, guard *protection.Guard) *protectedPacketConn {
	return &protectedPacketConn{UDPConn: conn, batch: ipv4.NewPacketConn(conn), guard: guard,
		received: metrics.TransportBytes("QUIC", "received"), sent: metrics.TransportBytes("QUIC", "sent")}
}

func (c *protectedPacketConn) admit(n int) bool {
	c.received.Add(float64(n))
	// A minimum charge also bounds empty and tiny datagram floods.
	if err := c.guard.AllowIngress(max(n, 64)); err != nil {
		metrics.RecordRejection("QUIC", err.Error())
		return false
	}
	return true
}

func (c *protectedPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.UDPConn.ReadFrom(p)
		if c.admit(n) || err != nil {
			return n, addr, err
		}
		time.Sleep(time.Millisecond)
	}
}

func (c *protectedPacketConn) ReadMsgUDP(p, oob []byte) (int, int, int, *net.UDPAddr, error) {
	for {
		n, oobn, flags, addr, err := c.UDPConn.ReadMsgUDP(p, oob)
		if c.admit(n) || err != nil {
			return n, oobn, flags, addr, err
		}
		time.Sleep(time.Millisecond)
	}
}

func (c *protectedPacketConn) ReadBatch(messages []ipv4.Message, flags int) (int, error) {
	for {
		n, err := c.batch.ReadBatch(messages, flags)
		// ReadBatch may return -1 with a socket error on Linux.
		kept := c.admitBatch(messages[:max(0, n)])
		if kept > 0 || err != nil {
			return kept, err
		}
		time.Sleep(time.Millisecond)
	}
}

func (c *protectedPacketConn) admitBatch(messages []ipv4.Message) int {
	kept := 0
	for i := range messages {
		if !c.admit(messages[i].N) {
			continue
		}
		if kept != i {
			// quic-go owns a parallel buffer array; never move the buffer identities.
			dst, src := &messages[kept], &messages[i]
			copy(dst.Buffers[0], src.Buffers[0][:src.N])
			copy(dst.OOB, src.OOB[:src.NN])
			dst.N, dst.NN, dst.Flags, dst.Addr = src.N, src.NN, src.Flags, src.Addr
		}
		kept++
	}
	return kept
}

func (c *protectedPacketConn) reserve(n int) error {
	if err := c.guard.ReserveEgress(n); err != nil {
		metrics.RecordRejection("QUIC", err.Error())
		return err
	}
	return nil
}

func (c *protectedPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if err := c.reserve(len(p)); err != nil {
		return 0, err
	}
	n, err := c.UDPConn.WriteTo(p, addr)
	c.sent.Add(float64(n))
	return n, err
}

func (c *protectedPacketConn) WriteMsgUDP(p, oob []byte, addr *net.UDPAddr) (int, int, error) {
	if err := c.reserve(len(p)); err != nil {
		return 0, 0, err
	}
	n, oobn, err := c.UDPConn.WriteMsgUDP(p, oob, addr)
	c.sent.Add(float64(n))
	return n, oobn, err
}
