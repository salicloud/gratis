package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/salicloud/gratis/api/internal/grpc"
	"github.com/salicloud/gratis/api/internal/store"
)

func main() {
	grpcAddr := flag.String("grpc", ":9090", "gRPC listen address (agents connect here)")
	httpAddr := flag.String("http", ":8080", "HTTP API listen address")
	dbURL    := flag.String("db", "", "PostgreSQL DSN (or GRATIS_DB_URL)")
	adminKey := flag.String("admin-key", "", "Admin API key (or GRATIS_ADMIN_KEY)")
	flag.Parse()

	if *dbURL == "" {
		if v := os.Getenv("GRATIS_DB_URL"); v != "" {
			dbURL = &v
		}
	}
	if *adminKey == "" {
		if v := os.Getenv("GRATIS_ADMIN_KEY"); v != "" {
			adminKey = &v
		}
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var st *store.Store
	if *dbURL != "" {
		var err error
		st, err = store.New(ctx, *dbURL)
		if err != nil {
			slog.Error("failed to connect to database", "err", err)
			os.Exit(1)
		}
		defer st.Close()
		slog.Info("database connected")
	} else {
		slog.Warn("no database configured — running in dev mode (in-memory auth, no persistence)")
	}

	srv := grpc.NewServer(*grpcAddr, *httpAddr, *adminKey, st)
	if err := srv.Run(ctx); err != nil {
		slog.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}
