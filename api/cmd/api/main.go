package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/salicloud/gratis/api/internal/grpc"
)

func main() {
	grpcAddr := flag.String("grpc", ":9090", "gRPC listen address (agents connect here)")
	httpAddr := flag.String("http", ":8080", "HTTP API listen address")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := grpc.NewServer(*grpcAddr, *httpAddr)
	if err := srv.Run(ctx); err != nil {
		slog.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}
