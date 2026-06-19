package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cct"
)

// capturingServer records the query string of the last request and returns a
// fixed GeoJSON feature set.
func capturingServer(t *testing.T, lastQuery *string) *cct.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*lastQuery = r.URL.RawQuery
		fmt.Fprint(w, `{"features":[
			{"properties":{"BLOCK_NAME":"Zone A","STAGE":4},"geometry":{"type":"Point","coordinates":[18.4,-33.9]}}
		],"exceededTransferLimit":false}`)
	}))
	t.Cleanup(srv.Close)
	c := cct.New(cct.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})
	t.Cleanup(c.Close)
	return c
}

func TestLoadSheddingStageFilter(t *testing.T) {
	var query string
	tools := New(capturingServer(t, &query))

	_, res, err := tools.loadShedding(context.Background(), nil, LoadSheddingInput{Stage: 4})
	if err != nil {
		t.Fatalf("loadShedding: %v", err)
	}
	if res.Count != 1 {
		t.Fatalf("want 1 feature, got %d", res.Count)
	}
	if v := res.Features[0].Attributes["BLOCK_NAME"]; v != "Zone A" {
		t.Fatalf("unexpected attribute: %v", v)
	}
	if !strings.Contains(query, "STAGE+%3D+4") && !strings.Contains(query, "STAGE+=+4") {
		t.Fatalf("expected STAGE = 4 where clause, got query %q", query)
	}
}

func TestCommonWhereAndBBoxAndGeometryOmitted(t *testing.T) {
	var query string
	tools := New(capturingServer(t, &query))

	_, res, err := tools.loadShedding(context.Background(), nil, LoadSheddingInput{
		Stage:       2,
		CommonQuery: CommonQuery{Where: "SUBURB_NAME = 'Obs'", BBox: []float64{18.3, -34.0, 18.6, -33.8}},
	})
	if err != nil {
		t.Fatalf("loadShedding: %v", err)
	}
	// Geometry not requested -> returnGeometry=false and no geometry in output.
	if !strings.Contains(query, "returnGeometry=false") {
		t.Fatalf("expected returnGeometry=false, got %q", query)
	}
	if res.Features[0].Geometry != nil {
		t.Fatal("geometry should be omitted when include_geometry is false")
	}
	// Both the base and user where clauses should be combined.
	if !strings.Contains(query, "AND") {
		t.Fatalf("expected combined where clause, got %q", query)
	}
	if !strings.Contains(query, "geometry=") {
		t.Fatalf("expected bbox geometry param, got %q", query)
	}
}

func TestIncludeGeometry(t *testing.T) {
	var query string
	tools := New(capturingServer(t, &query))

	_, res, err := tools.wards(context.Background(), nil, WardsInput{
		CommonQuery: CommonQuery{IncludeGeometry: true},
	})
	if err != nil {
		t.Fatalf("wards: %v", err)
	}
	if res.Features[0].Geometry == nil {
		t.Fatal("geometry should be present when include_geometry is true")
	}
}

func TestLandParcelsSuburbFilter(t *testing.T) {
	var query string
	tools := New(capturingServer(t, &query))

	if _, _, err := tools.landParcels(context.Background(), nil, LandParcelsInput{Suburb: "Newlands"}); err != nil {
		t.Fatalf("landParcels: %v", err)
	}
	if !strings.Contains(query, "SUBURB") || !strings.Contains(query, "Newlands") {
		t.Fatalf("expected SUBURB = 'Newlands' filter, got %q", query)
	}
}

func TestEqEscapesQuotes(t *testing.T) {
	got := eq("SUBURB", "O'Hara")
	want := "SUBURB = 'O''Hara'"
	if got != want {
		t.Fatalf("eq escaping: want %q, got %q", want, got)
	}
}

func TestLimitClamped(t *testing.T) {
	if got := effectiveLimit(0); got != defaultLimit {
		t.Fatalf("default: want %d, got %d", defaultLimit, got)
	}
	if got := effectiveLimit(99999); got != maxLimit {
		t.Fatalf("max clamp: want %d, got %d", maxLimit, got)
	}
	if got := effectiveLimit(50); got != 50 {
		t.Fatalf("passthrough: want 50, got %d", got)
	}
}

// TestRegisterAllTools ensures every tool's input/output schema can be inferred
// and registered without panicking.
func TestRegisterAllTools(t *testing.T) {
	c := cct.New(cct.Options{})
	t.Cleanup(c.Close)
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	New(c).Register(s) // panics on invalid schema inference
}
