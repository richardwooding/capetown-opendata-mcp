package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

const polygonBody = `{"features":[{"properties":{"NAME":"Site","Shape__Area":9},"geometry":{"type":"Polygon","coordinates":[[[18.40,-33.90],[18.42,-33.90],[18.42,-33.94],[18.40,-33.94],[18.40,-33.90]]]}}]}`

func TestGeometryDetailDefaultsToSimplified(t *testing.T) {
	var query string
	tools := New(capturingServerBody(t, &query, polygonBody))
	_, res, err := tools.heritageInventory(context.Background(), nil, HeritageInventoryInput{CommonQuery{IncludeGeometry: true}})
	if err != nil {
		t.Fatalf("heritageInventory: %v", err)
	}
	q, _ := url.ParseQuery(query)
	if q.Get("maxAllowableOffset") != "0.00005" || q.Get("geometryPrecision") != "6" {
		t.Errorf("simplify params = %q / %q", q.Get("maxAllowableOffset"), q.Get("geometryPrecision"))
	}
	if _, ok := res.Features[0].Attributes["Shape__Area"]; ok {
		t.Error("Shape__Area should be dropped unless named in fields")
	}
}

func TestGeometryDetailFull(t *testing.T) {
	var query string
	tools := New(capturingServerBody(t, &query, polygonBody))
	if _, _, err := tools.heritageInventory(context.Background(), nil, HeritageInventoryInput{CommonQuery{IncludeGeometry: true, GeometryDetail: "full"}}); err != nil {
		t.Fatalf("heritageInventory: %v", err)
	}
	if q, _ := url.ParseQuery(query); q.Has("maxAllowableOffset") {
		t.Errorf("full detail must not simplify: %q", query)
	}
}

func TestGeometryDetailCentroid(t *testing.T) {
	var query string
	tools := New(capturingServerBody(t, &query, polygonBody))
	_, res, err := tools.heritageInventory(context.Background(), nil, HeritageInventoryInput{CommonQuery{IncludeGeometry: true, GeometryDetail: "centroid"}})
	if err != nil {
		t.Fatalf("heritageInventory: %v", err)
	}
	g := res.Features[0].Geometry.(map[string]any)
	c := g["coordinates"].([]any)
	if g["type"] != "Point" || c[0] != 18.41 || c[1] != -33.92 {
		t.Errorf("centroid = %v, want Point [18.41 -33.92]", g)
	}
}

func TestGeometryDetailRejectsUnknown(t *testing.T) {
	var query string
	tools := New(capturingServerBody(t, &query, polygonBody))
	_, _, err := tools.heritageInventory(context.Background(), nil, HeritageInventoryInput{CommonQuery{IncludeGeometry: true, GeometryDetail: "low"}})
	if err == nil || !strings.Contains(err.Error(), "centroid") {
		t.Errorf("want an error listing the options, got %v", err)
	}
}

func TestTrimNoteExplainsBudgetAndSetting(t *testing.T) {
	var query string
	var rows []string
	for i := range 200 {
		rows = append(rows, fmt.Sprintf(`{"properties":{"ID":%d,"TEXT":%q},"geometry":{"type":"Point","coordinates":[18.4,-33.9]}}`, i, strings.Repeat("y", 400)))
	}
	body := `{"features":[` + strings.Join(rows, ",") + `]}`

	small := New(capturingServerBody(t, &query, body))
	_, res, err := small.heritageInventory(context.Background(), nil, HeritageInventoryInput{CommonQuery{Limit: 200, IncludeGeometry: true}})
	if err != nil {
		t.Fatalf("heritageInventory: %v", err)
	}
	for _, want := range []string{fmt.Sprintf("Returned %d of up to 200", res.Count), "8000 tokens", "fields", "centroid", "max_response_tokens", "CAPETOWN_MCP_MAX_RESPONSE_TOKENS", "next_cursor"} {
		if !strings.Contains(res.Note, want) {
			t.Errorf("note missing %q: %s", want, res.Note)
		}
	}

	big := New(capturingServerBody(t, &query, body)).WithResponseTokens(40000)
	_, res2, err := big.heritageInventory(context.Background(), nil, HeritageInventoryInput{CommonQuery{Limit: 200, IncludeGeometry: true}})
	if err != nil {
		t.Fatalf("heritageInventory: %v", err)
	}
	if res2.Count <= res.Count {
		t.Errorf("a larger budget should return more rows: %d vs %d", res2.Count, res.Count)
	}
}

func TestWithResponseTokensIgnoresTinyValues(t *testing.T) {
	if got := New(nil).WithResponseTokens(10).responseTokens; got != DefaultResponseTokens {
		t.Errorf("responseTokens = %d, want the default", got)
	}
}
