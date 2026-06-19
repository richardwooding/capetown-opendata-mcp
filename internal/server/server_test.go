package server

import (
	"context"
	"strings"
	"testing"
)

func TestUnknownTransport(t *testing.T) {
	s := New(Config{Name: "test", Version: "0.0.0", Transport: "carrier-pigeon"})
	err := s.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unknown transport") {
		t.Fatalf("want unknown transport error, got %v", err)
	}
}

func TestNewRegistersWithoutPanic(_ *testing.T) {
	// New builds the MCP server and registers all tools; schema inference runs
	// here, so a successful construction is the assertion.
	_ = New(Config{Name: "capetown-opendata", Version: "0.0.0"})
}
