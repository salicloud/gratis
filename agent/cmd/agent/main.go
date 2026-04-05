package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/salicloud/gratis/agent/internal/database"
	"github.com/salicloud/gratis/agent/internal/dns"
	"github.com/salicloud/gratis/agent/internal/rpc"
)

func main() {
	apiAddr  := flag.String("api", "localhost:9090", "Gratis API gRPC address")
	token    := flag.String("token", "", "Server provisioning token (or GRATIS_TOKEN)")
	dbSocket := flag.String("db-socket", "/var/run/mysqld/mysqld.sock", "MariaDB Unix socket path")
	pdnsURL  := flag.String("pdns-url", "", "PowerDNS API URL (e.g. http://localhost:8081)")
	pdnsKey  := flag.String("pdns-key", "", "PowerDNS API key (or GRATIS_PDNS_KEY)")
	flag.Parse()

	if *token == "" {
		if t := os.Getenv("GRATIS_TOKEN"); t != "" {
			token = &t
		} else {
			slog.Error("provisioning token required (--token or GRATIS_TOKEN)")
			os.Exit(1)
		}
	}
	if *pdnsKey == "" {
		if k := os.Getenv("GRATIS_PDNS_KEY"); k != "" {
			pdnsKey = &k
		}
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	// Database manager — optional, skip if socket not reachable
	var dbManager *database.Manager
	if db, err := database.NewManager(*dbSocket); err != nil {
		slog.Warn("MariaDB not available, database commands disabled", "err", err)
	} else {
		dbManager = db
		defer dbManager.Close()
		slog.Info("MariaDB connected", "socket", *dbSocket)
	}

	// PowerDNS client — optional
	var dnsClient *dns.Client
	if *pdnsURL != "" && *pdnsKey != "" {
		dnsClient = dns.NewClient(*pdnsURL, *pdnsKey)
		slog.Info("PowerDNS configured", "url", *pdnsURL)
	} else {
		slog.Warn("PowerDNS not configured, DNS commands disabled")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dispatcher := rpc.NewDispatcher(dbManager, dnsClient)
	agent := rpc.NewAgent(*apiAddr, *token, dispatcher)

	if err := agent.Run(ctx); err != nil {
		slog.Error("agent exited with error", "err", err)
		os.Exit(1)
	}
}
