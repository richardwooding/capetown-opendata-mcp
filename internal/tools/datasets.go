package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	capetown "github.com/richardwooding/capetown-opendata"
	arcgis "github.com/richardwooding/go-arcgis"
)

// Field names used to build dataset-specific filters.
const (
	fieldSuburb = "SUBURB"
)

// --- Load shedding ---

// LoadSheddingInput is the input for the load_shedding_blocks tool.
type LoadSheddingInput struct {
	CommonQuery
	Stage int `json:"stage,omitempty" jsonschema:"load shedding stage to filter by (1-8); omit for all stages"`
}

func (t *Tools) loadShedding(ctx context.Context, _ *mcp.CallToolRequest, in LoadSheddingInput) (*mcp.CallToolResult, FeatureResult, error) {
	base := capetown.LoadSheddingBlocks()
	if in.Stage > 0 {
		base = capetown.LoadSheddingBlocksForStage(in.Stage)
	}
	return t.run(ctx, base, in.CommonQuery)
}

// --- Service requests ---

// ServiceRequestsInput is the input for the service_requests tool.
type ServiceRequestsInput struct {
	CommonQuery
	Suburb string `json:"suburb,omitempty" jsonschema:"suburb name to filter service requests by"`
}

func (t *Tools) serviceRequests(ctx context.Context, _ *mcp.CallToolRequest, in ServiceRequestsInput) (*mcp.CallToolResult, FeatureResult, error) {
	base := capetown.ServiceRequests()
	if in.Suburb != "" {
		base = capetown.ServiceRequestsBySuburb(in.Suburb)
	}
	return t.run(ctx, base, in.CommonQuery)
}

// --- Wards ---

// WardsInput is the input for the wards tool.
type WardsInput struct {
	CommonQuery
}

func (t *Tools) wards(ctx context.Context, _ *mcp.CallToolRequest, in WardsInput) (*mcp.CallToolResult, FeatureResult, error) {
	return t.run(ctx, capetown.Wards(), in.CommonQuery)
}

// --- Land parcels ---

// LandParcelsInput is the input for the land_parcels tool.
type LandParcelsInput struct {
	CommonQuery
	Suburb string `json:"suburb,omitempty" jsonschema:"suburb name to filter land parcels by"`
}

func (t *Tools) landParcels(ctx context.Context, _ *mcp.CallToolRequest, in LandParcelsInput) (*mcp.CallToolResult, FeatureResult, error) {
	base := capetown.LandParcels()
	if in.Suburb != "" {
		base.Where = eq(fieldSuburb, in.Suburb)
	}
	return t.run(ctx, base, in.CommonQuery)
}

// --- Taxi routes ---

// TaxiRoutesInput is the input for the taxi_routes tool.
type TaxiRoutesInput struct {
	CommonQuery
}

func (t *Tools) taxiRoutes(ctx context.Context, _ *mcp.CallToolRequest, in TaxiRoutesInput) (*mcp.CallToolResult, FeatureResult, error) {
	return t.run(ctx, capetown.TaxiRoutes(), in.CommonQuery)
}

// --- Water quality ---

// WaterQualityInput is the input for the water_quality tool.
type WaterQualityInput struct {
	CommonQuery
}

func (t *Tools) waterQuality(ctx context.Context, _ *mcp.CallToolRequest, in WaterQualityInput) (*mcp.CallToolResult, FeatureResult, error) {
	return t.run(ctx, capetown.WaterQualityResults(), in.CommonQuery)
}

// --- Datasets without a dedicated constructor (queried via layer-ID constant) ---

// PublicLightingInput is the input for the public_lighting tool.
type PublicLightingInput struct {
	CommonQuery
}

func (t *Tools) publicLighting(ctx context.Context, _ *mcp.CallToolRequest, in PublicLightingInput) (*mcp.CallToolResult, FeatureResult, error) {
	return t.run(ctx, arcgis.QueryParams{LayerID: capetown.LayerPublicLighting}, in.CommonQuery)
}

// HeritageInventoryInput is the input for the heritage_inventory tool.
type HeritageInventoryInput struct {
	CommonQuery
}

func (t *Tools) heritageInventory(ctx context.Context, _ *mcp.CallToolRequest, in HeritageInventoryInput) (*mcp.CallToolResult, FeatureResult, error) {
	return t.run(ctx, arcgis.QueryParams{LayerID: capetown.LayerHeritageInventory}, in.CommonQuery)
}

// registerDatasets registers all per-dataset tools.
func (t *Tools) registerDatasets(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "load_shedding_blocks",
		Description: "Load shedding (rolling blackout) zone polygons for the City of Cape Town. Optionally filter by stage (1-8) or suburb via the where filter.",
	}, t.loadShedding)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "service_requests",
		Description: "Municipal service requests (faults/complaints), most recent first. Optionally filter by suburb.",
	}, t.serviceRequests)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wards",
		Description: "Municipal ward boundaries with ward number and councillor.",
	}, t.wards)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "land_parcels",
		Description: "Cadastral land parcel (erf) polygons with legal status and area. Optionally filter by suburb.",
	}, t.landParcels)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "taxi_routes",
		Description: "Registered minibus taxi routes with origin/destination ranks and operator.",
	}, t.taxiRoutes)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "water_quality",
		Description: "Inland water quality sampling results, most recent first.",
	}, t.waterQuality)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "public_lighting",
		Description: "Public street lighting assets.",
	}, t.publicLighting)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "heritage_inventory",
		Description: "Heritage inventory sites and features.",
	}, t.heritageInventory)
}
