package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestLeanRowsByDefault(t *testing.T) {
	var query string
	body := `{"features":[{"properties":{"NAME":"Site","DESC":null,"NOTE":"","Shape__Area":12.5,"Shape.STLength()":3.1}}]}`
	tools := New(capturingServerBody(t, &query, body))

	_, res, err := tools.heritageInventory(context.Background(), nil, HeritageInventoryInput{})
	if err != nil {
		t.Fatalf("heritageInventory: %v", err)
	}
	attrs := res.Features[0].Attributes
	if len(attrs) != 1 || attrs["NAME"] != "Site" {
		t.Errorf("attrs = %v, want only NAME", attrs)
	}
}

func TestKeepNullsAndShapeWhenAsked(t *testing.T) {
	var query string
	body := `{"features":[{"properties":{"NAME":"Site","DESC":null,"Shape__Area":12.5}}]}`
	tools := New(capturingServerBody(t, &query, body))
	keep := false

	_, res, err := tools.heritageInventory(context.Background(), nil, HeritageInventoryInput{CommonQuery{
		OmitNulls: &keep, Fields: []string{"NAME", "DESC", "Shape__Area"},
	}})
	if err != nil {
		t.Fatalf("heritageInventory: %v", err)
	}
	attrs := res.Features[0].Attributes
	if _, ok := attrs["DESC"]; !ok {
		t.Errorf("DESC should be kept with omit_nulls=false: %v", attrs)
	}
	if _, ok := attrs["Shape__Area"]; !ok {
		t.Errorf("Shape__Area should be kept when named in fields: %v", attrs)
	}
	q, _ := url.ParseQuery(query)
	if q.Get("outFields") != "NAME,DESC,Shape__Area" {
		t.Errorf("outFields = %q", q.Get("outFields"))
	}
}

func TestGeometryCoordinatesRounded(t *testing.T) {
	var query string
	body := `{"features":[{"properties":{"Ward":"62"},"geometry":{"type":"Point","coordinates":[18.446912345678,-33.971498765432]}}]}`
	tools := New(capturingServerBody(t, &query, body))

	_, res, err := tools.wards(context.Background(), nil, WardsInput{CommonQuery{IncludeGeometry: true}})
	if err != nil {
		t.Fatalf("wards: %v", err)
	}
	coords := res.Features[0].Geometry.(map[string]any)["coordinates"].([]any)
	if coords[0] != 18.446912 || coords[1] != -33.971499 {
		t.Errorf("coordinates = %v, want rounded to 6 decimals", coords)
	}
}

func TestResponseBudgetTrimsAndPages(t *testing.T) {
	var query string
	wide := strings.Repeat("x", 1000)
	var rows []string
	for i := range 100 {
		rows = append(rows, fmt.Sprintf(`{"properties":{"ID":%d,"TEXT":%q}}`, i, wide))
	}
	body := `{"features":[` + strings.Join(rows, ",") + `]}`
	tools := New(capturingServerBody(t, &query, body))

	_, res, err := tools.queryLayer(context.Background(), nil, QueryLayerInput{
		Service: "ODP_SPLIT_3", LayerID: 2, Limit: 100, Offset: 10,
	})
	if err != nil {
		t.Fatalf("queryLayer: %v", err)
	}
	if res.Count == 0 || res.Count >= 100 {
		t.Fatalf("count = %d, want a trimmed page", res.Count)
	}
	if !res.ExceededLimit || res.Note == "" || res.NextOffset == nil || *res.NextOffset != 10+res.Count {
		t.Errorf("trimmed page should set exceeded_limit, note and next_offset=%d: %+v", 10+res.Count, res)
	}
}

func TestResponseBudgetKeepsOneHugeRow(t *testing.T) {
	var query string
	body := fmt.Sprintf(`{"features":[{"properties":{"TEXT":%q}},{"properties":{"TEXT":"b"}}]}`, strings.Repeat("x", maxResponseChars*2))
	tools := New(capturingServerBody(t, &query, body))

	_, res, err := tools.heritageInventory(context.Background(), nil, HeritageInventoryInput{})
	if err != nil {
		t.Fatalf("heritageInventory: %v", err)
	}
	if res.Count != 1 || res.Note == "" {
		t.Errorf("count = %d note = %q, want the single oversized row with a note", res.Count, res.Note)
	}
}

func TestDefaultLimitIs25(t *testing.T) {
	var query string
	tools := New(capturingServer(t, &query))
	if _, _, err := tools.wards(context.Background(), nil, WardsInput{}); err != nil {
		t.Fatalf("wards: %v", err)
	}
	if q, _ := url.ParseQuery(query); q.Get("resultRecordCount") != "25" {
		t.Errorf("resultRecordCount = %q, want 25", q.Get("resultRecordCount"))
	}
}
