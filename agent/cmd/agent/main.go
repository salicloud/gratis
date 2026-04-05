package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/salicloud/gratis/agent/internal/rpc"
)

func main() {
	apiAddr := flag.String("api", "localhost:9090", "Gratis API gRPC address")
	token := flag.String("token", "", "Server provisioning token")
	flag.Parse()

	if *token == "" {
		if t := os.Getenv("GRATIS_TOKEN"); t != "" {
			token = &t
		} else {
			slog.Error("provisioning token required (--token or GRATIS_TOKEN)")
			os.Exit(1)
		}
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	agent := rpc.NewAgent(*apiAddr, *token)
	if err := agent.Run(ctx); err != nil {
		slog.Error("agent exited with error", "err", err)
		os.Exit(1)
	}
}
