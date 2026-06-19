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

// capturingServerBody records the query string of the last request and returns
// the supplied response body for every request.
func capturingServerBody(t *testing.T, lastQuery *string, body string) *cct.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*lastQuery = r.URL.RawQuery
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	c := cct.New(cct.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})
	t.Cleanup(c.Close)
	return c
}

// capturingServer records the query string of the last request and returns a
// fixed single-feature GeoJSON set.
func capturingServer(t *testing.T, lastQuery *string) *cct.Client {
	t.Helper()
	return capturingServerBody(t, lastQuery, `{"features":[
		{"properties":{"BlockID":14},"geometry":{"type":"Point","coordinates":[18.4,-33.9]}}
	],"exceededTransferLimit":false}`)
}

func TestLoadShedding(t *testing.T) {
	var query string
	tools := New(capturingServer(t, &query))

	_, res, err := tools.loadShedding(context.Background(), nil, LoadSheddingInput{})
	if err != nil {
		t.Fatalf("loadShedding: %v", err)
	}
	if res.Count != 1 {
		t.Fatalf("want 1 feature, got %d", res.Count)
	}
	if v := res.Features[0].Attributes["BlockID"]; v != float64(14) {
		t.Fatalf("unexpected attribute: %v", v)
	}
}

func TestCommonWhereAndBBoxAndGeometryOmitted(t *testing.T) {
	var query string
	tools := New(capturingServer(t, &query))

	_, res, err := tools.loadShedding(context.Background(), nil, LoadSheddingInput{
		CommonQuery: CommonQuery{Where: "BlockID > 5", BBox: []float64{18.3, -34.0, 18.6, -33.8}},
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
	if !strings.Contains(query, "OFC_SBRB_NAME") || !strings.Contains(query, "Newlands") {
		t.Fatalf("expected OFC_SBRB_NAME = 'Newlands' filter, got %q", query)
	}
}

// TestLandParcelsSuburbAndUserWhere checks that a dataset's own filter and a
// user-supplied where clause are AND-combined.
func TestLandParcelsSuburbAndUserWhere(t *testing.T) {
	var query string
	tools := New(capturingServer(t, &query))

	if _, _, err := tools.landParcels(context.Background(), nil, LandParcelsInput{
		Suburb:      "Newlands",
		CommonQuery: CommonQuery{Where: "OBJECTID > 100"},
	}); err != nil {
		t.Fatalf("landParcels: %v", err)
	}
	if !strings.Contains(query, "AND") {
		t.Fatalf("expected suburb and user where to be AND-combined, got %q", query)
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

func TestOmitNulls(t *testing.T) {
	var query string
	body := `{"features":[
		{"properties":{"NAME":"Site","DESC":null,"AGE":null}}
	],"exceededTransferLimit":false}`
	tools := New(capturingServerBody(t, &query, body))

	_, res, err := tools.heritageInventory(context.Background(), nil, HeritageInventoryInput{
		CommonQuery: CommonQuery{OmitNulls: true},
	})
	if err != nil {
		t.Fatalf("heritageInventory: %v", err)
	}
	attrs := res.Features[0].Attributes
	if _, ok := attrs["DESC"]; ok {
		t.Errorf("null attribute DESC should have been dropped: %v", attrs)
	}
	if v := attrs["NAME"]; v != "Site" {
		t.Errorf("non-null attribute NAME should be kept, got %v", v)
	}
}

func TestOmitNullsDefaultOff(t *testing.T) {
	var query string
	body := `{"features":[{"properties":{"NAME":"Site","DESC":null}}],"exceededTransferLimit":false}`
	tools := New(capturingServerBody(t, &query, body))

	_, res, err := tools.heritageInventory(context.Background(), nil, HeritageInventoryInput{})
	if err != nil {
		t.Fatalf("heritageInventory: %v", err)
	}
	if _, ok := res.Features[0].Attributes["DESC"]; !ok {
		t.Error("null attribute should be retained when omit_nulls is false")
	}
}

func TestOffsetAndNextOffset(t *testing.T) {
	var query string
	// One feature plus exceededTransferLimit -> more results available.
	body := `{"features":[{"properties":{"BlockID":1}}],"exceededTransferLimit":true}`
	tools := New(capturingServerBody(t, &query, body))

	_, res, err := tools.queryLayer(context.Background(), nil, QueryLayerInput{
		LayerID:     138,
		CommonQuery: CommonQuery{Limit: 1, Offset: 5},
	})
	if err != nil {
		t.Fatalf("queryLayer: %v", err)
	}
	if !strings.Contains(query, "resultOffset=5") {
		t.Fatalf("expected resultOffset=5 in query, got %q", query)
	}
	if res.NextOffset == nil || *res.NextOffset != 6 {
		t.Fatalf("expected next_offset 6, got %v", res.NextOffset)
	}
}

func TestAnnotateErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"400", fmt.Errorf("arcgis query layer 78: arcgis error 400: Unable to complete operation"), "layer_info(layer_id=78)"},
		{"timeout", fmt.Errorf("context deadline exceeded"), "smaller limit"},
		{"other", fmt.Errorf("connection refused"), "connection refused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := annotateErr(tc.err, 78).Error()
			if !strings.Contains(got, tc.want) {
				t.Errorf("annotateErr = %q, want it to contain %q", got, tc.want)
			}
		})
	}
	if annotateErr(nil, 1) != nil {
		t.Error("annotateErr(nil) should be nil")
	}
}

func TestServiceInfoNameFilter(t *testing.T) {
	var query string
	body := `{"serviceDescription":"svc","layers":[
		{"id":3,"name":"Electricity Public Lighting"},
		{"id":223,"name":"Routine Inland Water Quality Monitoring"},
		{"id":78,"name":"Ward"}
	],"tables":[{"id":229,"name":"Inland Water Quality Results (Raw)"}]}`
	tools := New(capturingServerBody(t, &query, body))

	_, res, err := tools.serviceInfo(context.Background(), nil, ServiceInfoInput{NameContains: "water"})
	if err != nil {
		t.Fatalf("serviceInfo: %v", err)
	}
	if len(res.Layers) != 1 || res.Layers[0].ID != 223 {
		t.Fatalf("expected only the water layer, got %v", res.Layers)
	}
	if len(res.Tables) != 1 || res.Tables[0].ID != 229 {
		t.Fatalf("expected the water table, got %v", res.Tables)
	}

	// No filter -> everything is returned.
	_, all, err := tools.serviceInfo(context.Background(), nil, ServiceInfoInput{})
	if err != nil {
		t.Fatalf("serviceInfo (no filter): %v", err)
	}
	if len(all.Layers) != 3 || len(all.Tables) != 1 {
		t.Fatalf("expected all layers/tables unfiltered, got %d layers %d tables", len(all.Layers), len(all.Tables))
	}
}

func TestFieldValues(t *testing.T) {
	var query string
	body := `{"features":[
		{"properties":{"OFC_SBRB_NAME":"ACACIA PARK"}},
		{"properties":{"OFC_SBRB_NAME":"ADMIRALS PARK"}},
		{"properties":{"OFC_SBRB_NAME":null}}
	],"exceededTransferLimit":false}`
	tools := New(capturingServerBody(t, &query, body))

	_, res, err := tools.fieldValues(context.Background(), nil, FieldValuesInput{
		LayerID: 56,
		Field:   "OFC_SBRB_NAME",
	})
	if err != nil {
		t.Fatalf("fieldValues: %v", err)
	}
	if !strings.Contains(query, "returnDistinctValues=true") {
		t.Errorf("expected returnDistinctValues=true in query, got %q", query)
	}
	// Null values are dropped; two real suburb names remain.
	if res.Count != 2 || len(res.Values) != 2 {
		t.Fatalf("expected 2 distinct values, got %d: %v", res.Count, res.Values)
	}
	if res.Values[0] != "ACACIA PARK" {
		t.Errorf("unexpected first value: %v", res.Values[0])
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
