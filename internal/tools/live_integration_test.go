//go:build integration

// Live integration tests exercise the MCP tool handlers against the real City
// of Cape Town Open Data services (split across ODP_SPLIT_1..12). They hit the
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
	"strings"
	"testing"
	"time"

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

	t.Run("wards", func(t *testing.T) {
		_, res, err := tl.wards(ctx, nil, WardsInput{CommonQuery: small})
		if err != nil {
			t.Fatalf("wards: %v", err)
		}
		if res.Count == 0 {
			t.Fatal("expected some ward features")
		}
	})

	t.Run("land_parcels_by_suburb", func(t *testing.T) {
		_, res, err := tl.landParcels(ctx, nil, LandParcelsInput{Suburb: "Newlands", CommonQuery: small})
		if err != nil {
			t.Fatalf("land_parcels: %v", err)
		}
		if res.Count == 0 {
			t.Fatal("expected Newlands parcels")
		}
	})

	t.Run("heritage_inventory", func(t *testing.T) {
		_, res, err := tl.heritageInventory(ctx, nil, HeritageInventoryInput{CommonQuery: small})
		if err != nil {
			t.Fatalf("heritage_inventory: %v", err)
		}
		if res.Count == 0 {
			t.Fatal("expected heritage features")
		}
	})

	t.Run("load_shedding_blocks", func(t *testing.T) {
		if _, _, err := tl.loadShedding(ctx, nil, LoadSheddingInput{CommonQuery: small}); err != nil {
			t.Fatalf("load_shedding_blocks: %v", err)
		}
	})

	t.Run("public_lighting", func(t *testing.T) {
		if _, _, err := tl.publicLighting(ctx, nil, PublicLightingInput{CommonQuery: small}); err != nil {
			t.Fatalf("public_lighting: %v", err)
		}
	})

	t.Run("taxi_routes", func(t *testing.T) {
		if _, _, err := tl.taxiRoutes(ctx, nil, TaxiRoutesInput{CommonQuery: small}); err != nil {
			t.Fatalf("taxi_routes: %v", err)
		}
	})

	t.Run("water_quality", func(t *testing.T) {
		_, res, err := tl.waterQuality(ctx, nil, WaterQualityInput{CommonQuery: small})
		if err != nil {
			t.Fatalf("water_quality: %v", err)
		}
		if res.Count == 0 {
			t.Fatal("expected water quality rows")
		}
	})
}

// TestLiveGenericTools exercises layer_info, field_values, and query_layer with
// an explicit service argument.
func TestLiveGenericTools(t *testing.T) {
	tl := liveTools(t)
	ctx := context.Background()

	t.Run("layer_info", func(t *testing.T) {
		_, res, err := tl.layerInfo(ctx, nil, LayerInfoInput{Service: "ODP_SPLIT_5", LayerID: 6})
		if err != nil {
			t.Fatalf("layer_info: %v", err)
		}
		if res.Name != "Ward" {
			t.Fatalf("expected Ward layer, got %q", res.Name)
		}
		var hasWardName bool
		for _, f := range res.Fields {
			if f.Name == "WARD_NAME" {
				hasWardName = true
			}
		}
		if !hasWardName {
			t.Error("expected WARD_NAME field on the ward layer")
		}
	})

	t.Run("field_values", func(t *testing.T) {
		_, res, err := tl.fieldValues(ctx, nil, FieldValuesInput{Service: "ODP_SPLIT_5", LayerID: 6, Field: "WARD_NAME", Limit: 10})
		if err != nil {
			t.Fatalf("field_values: %v", err)
		}
		if res.Count == 0 {
			t.Fatal("expected some distinct ward names")
		}
	})

	t.Run("query_layer_count_only", func(t *testing.T) {
		_, res, err := tl.queryLayer(ctx, nil, QueryLayerInput{Service: "ODP_SPLIT_5", LayerID: 6, CountOnly: true})
		if err != nil {
			t.Fatalf("query_layer count: %v", err)
		}
		if res.Count == 0 {
			t.Fatal("expected a positive ward count")
		}
	})
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
			CommonQuery: CommonQuery{Where: "NOT_A_FIELD = 1"},
		})
		if err == nil {
			t.Fatal("expected an error for an invalid where field")
		}
		if !strings.Contains(err.Error(), "layer_info") {
			t.Errorf("expected a hint pointing at layer_info, got %v", err)
		}
	})
}
