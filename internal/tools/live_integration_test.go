//go:build integration

// Live integration tests exercise the MCP tool handlers against the real City
// of Cape Town Open Data services (split across ODP_SPLIT_1..13). They hit the
// network and are excluded from the default build; run with:
//
//	go test -tags=integration -v ./internal/tools/
//
// Their job is to prove the full tool surface works end-to-end after the City's
// service restructure: dataset tools resolve to the right split/layer, the
// aggregated service_info catalogue is populated, the generic tools accept a
// service argument, and the error paths stay friendly.
package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	capetown "github.com/richardwooding/capetown-opendata"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cct"
)

func liveTools(t *testing.T) *Tools {
	t.Helper()
	c := cct.New(cct.Options{Timeout: 90 * time.Second, MaxRetries: 3, CacheTTL: 5 * time.Minute})
	t.Cleanup(c.Close)
	return New(c)
}

// TestLiveServiceInfoAggregates checks the aggregated catalogue is populated
// across the split services and tags each layer with its host service.
func TestLiveServiceInfoAggregates(t *testing.T) {
	tl := liveTools(t)
	_, res, err := tl.serviceInfo(context.Background(), nil, ServiceInfoInput{})
	if err != nil {
		t.Fatalf("service_info: %v", err)
	}
	if len(res.Layers) < 50 {
		t.Fatalf("expected a large aggregated catalogue, got %d layers", len(res.Layers))
	}
	seenServices := map[string]bool{}
	var sawWard bool
	for _, l := range res.Layers {
		if l.Service == "" {
			t.Fatalf("layer %q missing its host service tag", l.Name)
		}
		seenServices[l.Service] = true
		if l.Service == "ODP_SPLIT_5" && l.Name == "Ward" {
			sawWard = true
		}
	}
	if len(seenServices) < 5 {
		t.Fatalf("expected layers from several services, got %v", seenServices)
	}
	if !sawWard {
		t.Error("expected to find ODP_SPLIT_5 'Ward' in the aggregated catalogue")
	}
	// Unavailable is informational (e.g. ODP_SPLIT_9 has been down); just log it.
	for _, u := range res.Unavailable {
		t.Logf("service unavailable: %s (%s)", u.Service, u.Error)
	}
}

// TestLiveDatasetTools count-checks every dedicated dataset tool and does a
// small fetch, proving each resolves to a live split/layer.
func TestLiveDatasetTools(t *testing.T) {
	tl := liveTools(t)
	ctx := context.Background()
	small := CommonQuery{Limit: 3}
	cases := []struct {
		name     string
		run      func() (FeatureResult, error)
		wantSome bool
	}{
		{"wards", func() (FeatureResult, error) {
			_, r, e := tl.wards(ctx, nil, WardsInput{CommonQuery: small})
			return r, e
		}, true},
		{"land_parcels_by_suburb", func() (FeatureResult, error) {
			_, r, e := tl.landParcels(ctx, nil, LandParcelsInput{Suburb: "Newlands", CommonQuery: small})
			return r, e
		}, true},
		{"heritage_inventory", func() (FeatureResult, error) {
			_, r, e := tl.heritageInventory(ctx, nil, HeritageInventoryInput{CommonQuery: small})
			return r, e
		}, true},
		{"load_shedding_blocks", func() (FeatureResult, error) {
			_, r, e := tl.loadShedding(ctx, nil, LoadSheddingInput{CommonQuery: small})
			return r, e
		}, false},
		{"public_lighting", func() (FeatureResult, error) {
			_, r, e := tl.publicLighting(ctx, nil, PublicLightingInput{CommonQuery: small})
			return r, e
		}, false},
		{"taxi_routes", func() (FeatureResult, error) {
			_, r, e := tl.taxiRoutes(ctx, nil, TaxiRoutesInput{CommonQuery: small})
			return r, e
		}, false},
		{"water_quality", func() (FeatureResult, error) {
			_, r, e := tl.waterQuality(ctx, nil, WaterQualityInput{CommonQuery: small})
			return r, e
		}, true},
		{"official_suburbs", func() (FeatureResult, error) {
			_, r, e := tl.officialSuburbs(ctx, nil, OfficialSuburbsInput{Suburb: "Athlone", CommonQuery: small})
			return r, e
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.run()
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if tc.wantSome && res.Count == 0 {
				t.Fatalf("%s: expected at least one feature", tc.name)
			}
		})
	}
}

// hasField reports whether a field named name is present in fields.
func hasField(fields []FieldInfo, name string) bool {
	for _, f := range fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

// TestLiveLayerInfo describes the ward layer via the generic layer_info tool.
func TestLiveLayerInfo(t *testing.T) {
	tl := liveTools(t)
	_, res, err := tl.layerInfo(context.Background(), nil, LayerInfoInput{Service: "ODP_SPLIT_5", LayerID: 6})
	if err != nil {
		t.Fatalf("layer_info: %v", err)
	}
	if res.Name != "Ward" {
		t.Fatalf("expected Ward layer, got %q", res.Name)
	}
	if !hasField(res.Fields, "WARD_NAME") {
		t.Error("expected WARD_NAME field on the ward layer")
	}
}

// TestLiveFieldValues lists distinct ward names via field_values.
func TestLiveFieldValues(t *testing.T) {
	tl := liveTools(t)
	_, res, err := tl.fieldValues(context.Background(), nil, FieldValuesInput{Service: "ODP_SPLIT_5", LayerID: 6, Field: "WARD_NAME", Limit: 10})
	if err != nil {
		t.Fatalf("field_values: %v", err)
	}
	if res.Count == 0 {
		t.Fatal("expected some distinct ward names")
	}
}

// TestLiveQueryLayerCount count-checks the ward layer via the generic tool.
func TestLiveQueryLayerCount(t *testing.T) {
	tl := liveTools(t)
	_, res, err := tl.queryLayer(context.Background(), nil, QueryLayerInput{Service: "ODP_SPLIT_5", LayerID: 6, CountOnly: true})
	if err != nil {
		t.Fatalf("query_layer count: %v", err)
	}
	if res.Count == 0 {
		t.Fatal("expected a positive ward count")
	}
}

// TestLiveErrorPaths confirms the friendly error surfaces for bad inputs.
func TestLiveErrorPaths(t *testing.T) {
	tl := liveTools(t)
	ctx := context.Background()

	t.Run("unknown_service", func(t *testing.T) {
		_, _, err := tl.queryLayer(ctx, nil, QueryLayerInput{Service: "ODP_SPLIT_99", LayerID: 0, CountOnly: true})
		if err == nil || !strings.Contains(err.Error(), "unknown service") {
			t.Fatalf("expected an unknown-service error, got %v", err)
		}
	})

	t.Run("missing_service", func(t *testing.T) {
		_, _, err := tl.queryLayer(ctx, nil, QueryLayerInput{LayerID: 0, CountOnly: true})
		if err == nil || !strings.Contains(err.Error(), "service is required") {
			t.Fatalf("expected a missing-service error, got %v", err)
		}
	})

	t.Run("bad_field", func(t *testing.T) {
		_, _, err := tl.queryLayer(ctx, nil, QueryLayerInput{
			Service: "ODP_SPLIT_5", LayerID: 6,
			Where: "NOT_A_FIELD = 1",
		})
		if err == nil {
			t.Fatal("expected an error for an invalid where field")
		}
		if !strings.Contains(err.Error(), "layer_info") {
			t.Errorf("expected a hint pointing at layer_info, got %v", err)
		}
	})
}

// TestLiveHubTables fetches a few rows from each ArcGIS Online table, proving
// the item IDs resolve and the default ordering is accepted.
func TestLiveHubTables(t *testing.T) {
	tl := liveTools(t)
	ctx := context.Background()

	_, sr, err := tl.serviceRequests(ctx, nil, ServiceRequestsInput{TableQuery{Limit: 3, Where: "Ward = '062'"}})
	if err != nil {
		t.Fatalf("service_requests: %v", err)
	}
	if sr.Count == 0 || sr.Features[0].Attributes["Ward"] != "062" {
		t.Fatalf("expected ward 062 service requests, got %+v", sr.Features)
	}

	_, bp, err := tl.buildingPlans(ctx, nil, BuildingPlansInput{TableQuery{Limit: 3, Where: "Ward_No = 62"}})
	if err != nil {
		t.Fatalf("building_plan_approvals: %v", err)
	}
	if bp.Count == 0 {
		t.Fatal("expected building plan rows for ward 62")
	}

	_, pe, err := tl.populationEstimates(ctx, nil, PopulationEstimatesInput{Suburb: "athlone"})
	if err != nil {
		t.Fatalf("population_estimates: %v", err)
	}
	if pe.Count != 1 || pe.Features[0].Attributes["Suburb_Name"] != "Athlone" {
		t.Fatalf("expected one Athlone estimate, got %+v", pe.Features)
	}
	if pop, _ := pe.Features[0].Attributes["Estimated_population"].(float64); pop < 1000 {
		t.Errorf("Athlone population = %v, want a plausible number", pop)
	}

	_, tf, err := tl.taxiFares(ctx, nil, TaxiFaresInput{OriginRank: "MBT019", Limit: 5})
	if err != nil {
		t.Fatalf("taxi_fares: %v", err)
	}
	if tf.Count == 0 || tf.Features[0].Attributes["Origin_Rank_Ref_No"] != "MBT019" {
		t.Fatalf("expected fares departing MBT019, got %+v", tf.Features)
	}
	if _, ok := tf.Features[0].Attributes["Fare"].(float64); !ok {
		t.Errorf("expected a numeric Fare, got %+v", tf.Features[0].Attributes)
	}

	box := []float64{18.44, -33.99, 18.48, -33.96}
	_, sp, err := tl.serviceRequestsSpatial(ctx, nil, ServiceRequestsSpatialInput{CommonQuery{
		BBox: box, Where: "Created_On_Date >= DATE '2026-01-01'", Limit: 3, IncludeGeometry: true, GeometryDetail: geometryCentroid,
		Fields: []string{"C3_Complaint_Type", "Created_On_Date"},
	}})
	if err != nil {
		t.Fatalf("service_requests_spatial: %v", err)
	}
	if sp.Count == 0 {
		t.Fatal("expected geocoded service requests in the Newlands box")
	}
	for _, f := range sp.Features {
		var g struct {
			Coordinates []float64 `json:"coordinates"`
		}
		raw, _ := json.Marshal(f.Geometry)
		if err := json.Unmarshal(raw, &g); err != nil || len(g.Coordinates) != 2 {
			t.Fatalf("geometry %s is not a point: %v", raw, err)
		}
		if x, y := g.Coordinates[0], g.Coordinates[1]; x < box[0] || x > box[2] || y < box[1] || y > box[3] {
			t.Errorf("point %v outside bbox %v", g.Coordinates, box)
		}
	}
}

// TestLiveServiceInfoListsHubTables checks the hub tables join the catalogue
// under their friendly labels.
func TestLiveServiceInfoListsHubTables(t *testing.T) {
	tl := liveTools(t)
	_, res, err := tl.serviceInfo(context.Background(), nil, ServiceInfoInput{})
	if err != nil {
		t.Fatalf("service_info: %v", err)
	}
	got := map[string]ServiceLayerSummary{}
	for _, l := range res.Layers {
		got[l.Service] = ServiceLayerSummary{IsTable: l.IsTable, Name: l.Name}
	}
	for _, svc := range capetown.HubServices() {
		l, ok := got[svc]
		wantTable := svc != capetown.ServiceServiceRequestsSpatial
		if !ok || l.IsTable != wantTable {
			t.Errorf("%s: expected in the catalogue with is_table=%v, got %+v (unavailable: %v)", svc, wantTable, l, res.Unavailable)
		}
		if strings.Contains(l.Name, "_") {
			t.Errorf("%s: name %q looks like the stale upstream name, want the hub item title", svc, l.Name)
		}
		t.Logf("%s: %s", svc, l.Name)
	}
}

// ServiceLayerSummary is what the hub-table catalogue check records per service.
type ServiceLayerSummary struct {
	IsTable bool
	Name    string
}

// TestLiveSummarize groups service requests by complaint type and land parcels
// by zoning, covering an ArcGIS Online table and an ODP_SPLIT layer.
func TestLiveSummarize(t *testing.T) {
	tl := liveTools(t)
	ctx := context.Background()

	_, sr, err := tl.summarizeLayer(ctx, nil, SummarizeLayerInput{
		Service: "SERVICE_REQUESTS", LayerID: 0,
		GroupBy: []string{"C3_Complaint_Type"},
		Where:   "Ward = '062' AND CreatedOnDT >= DATE '2026-01-01'",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("summarize service requests: %v", err)
	}
	if sr.Count == 0 {
		t.Fatal("expected complaint-type groups for ward 062")
	}
	if _, ok := sr.Groups[0]["record_count"]; !ok {
		t.Fatalf("expected a record_count column, got %v", sr.Groups[0])
	}
	t.Logf("ward 062 top complaint: %v", sr.Groups[0])

	_, lp, err := tl.summarizeLayer(ctx, nil, SummarizeLayerInput{
		Service: "ODP_SPLIT_4", LayerID: 0,
		GroupBy: []string{"ZONING"},
		Where:   "OFC_SBRB_NAME = 'NEWLANDS'",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("summarize land parcels: %v", err)
	}
	if lp.Count == 0 {
		t.Fatal("expected zoning groups for Newlands")
	}
	t.Logf("Newlands top zoning: %v", lp.Groups[0])

	_, pop, err := tl.summarizeLayer(ctx, nil, SummarizeLayerInput{
		Service: "POPULATION_ESTIMATES", LayerID: 0,
		Statistics: []StatisticInput{{Type: "sum", Field: "Estimated_population"}},
	})
	if err != nil {
		t.Fatalf("summarize population: %v", err)
	}
	total, _ := pop.Groups[0]["sum_Estimated_population"].(float64)
	if total < 4_000_000 || total > 7_000_000 {
		t.Errorf("city population sum = %v, want roughly 5.3 million", total)
	}
	t.Logf("city population estimate: %.0f", total)
}

// TestLiveDefaultServerHasAllSplits checks the default server (esapqa) lists
// ODP_SPLIT_9 and ODP_SPLIT_13, both missing as feature services on citymaps.
func TestLiveDefaultServerHasAllSplits(t *testing.T) {
	tl := liveTools(t)
	_, res, err := tl.serviceInfo(context.Background(), nil, ServiceInfoInput{})
	if err != nil {
		t.Fatalf("service_info: %v", err)
	}
	seen := map[string]bool{}
	for _, l := range res.Layers {
		seen[l.Service] = true
	}
	for _, s := range []string{"ODP_SPLIT_9", "ODP_SPLIT_13"} {
		if !seen[s] {
			t.Errorf("expected layers from %s on the default server; unavailable: %v", s, res.Unavailable)
		}
	}
}

// TestLiveCityMapsServer proves the server setting takes effect: citymaps
// still answers the existing datasets.
func TestLiveCityMapsServer(t *testing.T) {
	c := cct.New(cct.Options{Timeout: 90 * time.Second, MaxRetries: 3, ServerFolder: capetown.FolderCityMaps})
	t.Cleanup(c.Close)
	_, res, err := New(c).wards(context.Background(), nil, WardsInput{Limit: 1})
	if err != nil || res.Count != 1 {
		t.Fatalf("wards on citymaps: count=%d err=%v", res.Count, err)
	}
}

// TestLiveCursorPaging pages through a City layer (keyset on OBJECTID) and an
// ArcGIS Online table (keyset on ObjectId DESC) using only next_cursor, and
// checks no row repeats and the layer total matches summarize_layer.
func TestLiveCursorPaging(t *testing.T) {
	tl := liveTools(t)
	ctx := context.Background()

	_, sum, err := tl.summarizeLayer(ctx, nil, SummarizeLayerInput{Service: "ODP_SPLIT_3", LayerID: 2, Where: "CNFR_CCT_GRD = '2'"})
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	want := int(sum.Groups[0]["record_count"].(float64))
	seen := map[any]bool{}
	in := HeritageInventoryInput{CommonQuery{Where: "CNFR_CCT_GRD = '2'", Limit: 2000, Fields: []string{"HRTG_INV_SITE_NAME"}}}
	for range 20 {
		_, res, err := tl.heritageInventory(ctx, nil, in)
		if err != nil {
			t.Fatalf("heritage page: %v", err)
		}
		for _, f := range res.Features {
			id := f.Attributes["OBJECTID"]
			if seen[id] {
				t.Fatalf("OBJECTID %v repeated", id)
			}
			seen[id] = true
		}
		if res.NextCursor == "" {
			break
		}
		in.Cursor = res.NextCursor
	}
	if len(seen) != want {
		t.Errorf("paged %d heritage sites, summarize_layer counts %d", len(seen), want)
	}

	srIn := ServiceRequestsInput{TableQuery{Where: "Ward = '062'", Limit: 100, Fields: []string{"C3_Complaint_Type"}}}
	srSeen := map[any]bool{}
	for page := range 3 {
		_, res, err := tl.serviceRequests(ctx, nil, srIn)
		if err != nil {
			t.Fatalf("service requests page %d: %v", page, err)
		}
		for _, f := range res.Features {
			if id := f.Attributes["ObjectId"]; srSeen[id] {
				t.Fatalf("ObjectId %v repeated", id)
			} else {
				srSeen[id] = true
			}
		}
		if res.NextCursor == "" {
			t.Fatalf("page %d: expected next_cursor on a table with thousands of rows", page)
		}
		srIn.Cursor = res.NextCursor
	}
}

// TestLiveLayerInfoDataRange checks the ArcGIS Online tables report a readable
// name, a hub source, and a data range that runs past the date in their stale
// table names (the service requests freeze date, the building plans name's
// year, and the taxi survey's "2023 to 2025").
func TestLiveLayerInfoDataRange(t *testing.T) {
	tl := liveTools(t)
	for svc, stale := range map[string]string{"SERVICE_REQUESTS": "2026-10-06", "SERVICE_REQUESTS_SPATIAL": "2026-10-06", "BUILDING_PLANS": "2025-12-31", "TAXI_FARES": "2025-09-01"} {
		_, res, err := tl.layerInfo(context.Background(), nil, LayerInfoInput{Service: svc, LayerID: 0})
		if err != nil {
			t.Fatalf("%s layer_info: %v", svc, err)
		}
		if strings.Contains(res.Name, "_") || res.DataRange == nil {
			t.Fatalf("%s: name=%q data_range=%+v", svc, res.Name, res.DataRange)
		}
		if res.DataRange.Latest <= stale {
			t.Errorf("%s latest %s should be after the stale name's %s", svc, res.DataRange.Latest, stale)
		}
		if res.Source == nil || res.Source.Title == "" || res.Source.Modified == "" {
			t.Errorf("%s: source = %+v, want the hub item's title and modified date", svc, res.Source)
		}
		t.Logf("%s: %s, %s to %s, source %+v", svc, res.Name, res.DataRange.Earliest, res.DataRange.Latest, res.Source)
	}
	_, res, err := tl.layerInfo(context.Background(), nil, LayerInfoInput{Service: "POPULATION_ESTIMATES", LayerID: 0})
	if err != nil {
		t.Fatalf("population layer_info: %v", err)
	}
	if res.DataRange != nil || res.Source == nil || !strings.Contains(res.Source.Summary, "estimates") {
		t.Errorf("population: data_range=%+v source=%+v, want no data_range and a source summary", res.DataRange, res.Source)
	}
}

// TestLiveSummarizeWithoutGroupBy checks a single overall total works on an
// ArcGIS Online table, which rejects paging parameters on ungrouped statistics.
func TestLiveSummarizeWithoutGroupBy(t *testing.T) {
	tl := liveTools(t)
	_, res, err := tl.summarizeLayer(context.Background(), nil, SummarizeLayerInput{Service: "SERVICE_REQUESTS", LayerID: 0, Where: "Ward = '062'"})
	if err != nil || res.Count != 1 {
		t.Fatalf("ungrouped summarize: count=%d err=%v", res.Count, err)
	}
	t.Logf("ward 062 total: %v", res.Groups[0])
}
