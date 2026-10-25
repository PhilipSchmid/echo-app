package ui

import (
	"net/netip"
	"strconv"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
)

type stackDetails struct {
	Transport string
	PeerPort  string
	LocalIP   string
	LocalPort string
}

func newStackDetails(request handlers.HTTPResponse) stackDetails {
	stack := stackDetails{Transport: "Not observed"}
	switch request.HTTPVersion {
	case "HTTP/1.0", "HTTP/1.1", "HTTP/2.0":
		stack.Transport = "TCP"
	case "HTTP/3", "HTTP/3.0":
		stack.Transport = "QUIC / UDP"
	}
	if peer, err := netip.ParseAddrPort(request.RemoteAddr); err == nil && peer.Port() != 0 {
		stack.PeerPort = strconv.Itoa(int(peer.Port()))
	}
	if local, err := netip.ParseAddrPort(request.LocalAddr); err == nil && local.Port() != 0 {
		stack.LocalIP = local.Addr().Unmap().String()
		stack.LocalPort = strconv.Itoa(int(local.Port()))
	}
	return stack
}
