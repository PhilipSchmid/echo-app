package server

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	pb "github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

// Compare response-size preflight with the same echo handler and 64 KiB cap.
func BenchmarkGRPCUnarySizePreflight(b *testing.B) {
	logger := logrus.StandardLogger()
	output, level := logger.Out, logger.GetLevel()
	logger.SetOutput(io.Discard)
	logger.SetLevel(logrus.InfoLevel)
	b.Cleanup(func() { logger.SetOutput(output); logger.SetLevel(level) })
	cfg := &config.Config{Message: "benchmark-test", Node: "bench-node", MaxResponseSize: 64 << 10}
	srv := NewGRPCServer(cfg, nil, nil)
	echo := handlers.NewEchoServer(cfg)
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 50051}})
	request := &pb.EchoRequest{}
	handler := func(ctx context.Context, request any) (any, error) {
		return echo.Echo(ctx, request.(*pb.EchoRequest))
	}
	for _, checked := range []bool{false, true} {
		name := "handler_only"
		if checked {
			name = "with_64KiB_preflight"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				var err error
				if checked {
					_, err = srv.protectUnary(ctx, request, &grpc.UnaryServerInfo{}, handler)
				} else {
					_, err = handler(ctx, request)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
