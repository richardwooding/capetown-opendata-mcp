// Command capetown-opendata-mcp is a Model Context Protocol server exposing the
// City of Cape Town Open Data Portal to MCP clients.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/richardwooding/capetown-opendata-mcp/internal/server"
)

// Build information, injected via -ldflags at release time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "capetown-opendata-mcp:", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("capetown-opendata-mcp", flag.ContinueOnError)
	var (
		transport   = fs.String("transport", env("CAPETOWN_MCP_TRANSPORT", "stdio"), "transport: stdio or http")
		httpAddr    = fs.String("http-addr", env("CAPETOWN_MCP_HTTP_ADDR", ":8080"), "address for the http transport")
		timeout     = fs.Duration("timeout", envDuration("CAPETOWN_MCP_TIMEOUT", 30*time.Second), "per-request upstream timeout")
		cacheTTL    = fs.Duration("cache-ttl", envDuration("CAPETOWN_MCP_CACHE_TTL", 5*time.Minute), "response cache TTL (0 disables caching)")
		token       = fs.String("arcgis-token", os.Getenv("CAPETOWN_MCP_ARCGIS_TOKEN"), "optional ArcGIS token for authenticated services")
		showVersion = fs.Bool("version", false, "print version and exit")
	)
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *showVersion {
		fmt.Printf("capetown-opendata-mcp %s (commit %s, built %s)\n", version, commit, date)
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(server.Config{
		Name:      "capetown-opendata",
		Version:   version,
		Transport: *transport,
		HTTPAddr:  *httpAddr,
		Timeout:   *timeout,
		CacheTTL:  *cacheTTL,
		Token:     *token,
	})
	return srv.Run(ctx)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
