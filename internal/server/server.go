// Package server wires the cct client and MCP tools into a runnable MCP server
// over either stdio or streamable HTTP.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cct"
	"github.com/richardwooding/capetown-opendata-mcp/internal/tools"
)

// Config configures the MCP server.
type Config struct {
	Name      string
	Version   string
	Transport string // "stdio" or "http"
	HTTPAddr  string
	Timeout   time.Duration
	CacheTTL  time.Duration
	Token     string
}

// Server is a configured Cape Town Open Data MCP server.
type Server struct {
	cfg    Config
	client *cct.Client
	mcp    *mcp.Server
}

// New builds a Server and registers all tools.
func New(cfg Config) *Server {
	client := cct.New(cct.Options{
		Timeout:  cfg.Timeout,
		Token:    cfg.Token,
		CacheTTL: cfg.CacheTTL,
	})
	m := mcp.NewServer(&mcp.Implementation{
		Name:       cfg.Name,
		Version:    cfg.Version,
		WebsiteURL: "https://github.com/richardwooding/capetown-opendata-mcp",
	}, nil)
	tools.New(client).Register(m)
	return &Server{cfg: cfg, client: client, mcp: m}
}

// Run starts the server on the configured transport and blocks until ctx is
// cancelled or the transport stops.
func (s *Server) Run(ctx context.Context) error {
	defer s.client.Close()
	switch s.cfg.Transport {
	case "", "stdio":
		return s.mcp.Run(ctx, &mcp.StdioTransport{})
	case "http":
		return s.runHTTP(ctx)
	default:
		return fmt.Errorf("unknown transport %q (want \"stdio\" or \"http\")", s.cfg.Transport)
	}
}

func (s *Server) runHTTP(ctx context.Context) error {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.mcp }, nil)
	httpServer := &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
