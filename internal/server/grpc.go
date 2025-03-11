package server

import (
	"net"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/handlers"
	"github.com/PhilipSchmid/echo-app/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func StartGRPCServer(cfg *config.Config) {
	listener, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		logrus.Fatalf("Failed to start gRPC server: %v", err)
	}
	defer listener.Close()

	// Create the gRPC server
	grpcServer := grpc.NewServer()
	proto.RegisterEchoServiceServer(grpcServer, handlers.NewEchoServer(cfg))

	// Enable reflection
	reflection.Register(grpcServer)

	logrus.Infof("gRPC server listening on port %s", cfg.GRPCPort)
	if err := grpcServer.Serve(listener); err != nil {
		logrus.Errorf("gRPC server failed: %v", err)
	}
}
