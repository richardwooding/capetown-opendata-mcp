package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	capetown "github.com/richardwooding/capetown-opendata"
)

// --- Load shedding ---

// LoadSheddingInput is the input for the load_shedding_blocks tool.
type LoadSheddingInput struct {
	CommonQuery
}

func (t *Tools) loadShedding(ctx context.Context, _ *mcp.CallToolRequest, in LoadSheddingInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.LoadSheddingBlocks()
	return t.run(ctx, q.Service, q.Params, in.CommonQuery)
}

// --- Wards ---

// WardsInput is the input for the wards tool.
type WardsInput struct {
	CommonQuery
}

func (t *Tools) wards(ctx context.Context, _ *mcp.CallToolRequest, in WardsInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.Wards()
	return t.run(ctx, q.Service, q.Params, in.CommonQuery)
}

// --- Land parcels ---

// LandParcelsInput is the input for the land_parcels tool.
type LandParcelsInput struct {
	CommonQuery
	Suburb string `json:"suburb,omitempty" jsonschema:"suburb name to filter land parcels by"`
}

func (t *Tools) landParcels(ctx context.Context, _ *mcp.CallToolRequest, in LandParcelsInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.LandParcels()
	if in.Suburb != "" {
		q = capetown.LandParcelsBySuburb(in.Suburb)
	}
	return t.run(ctx, q.Service, q.Params, in.CommonQuery)
}

// --- Taxi routes ---

// TaxiRoutesInput is the input for the taxi_routes tool.
type TaxiRoutesInput struct {
	CommonQuery
}

func (t *Tools) taxiRoutes(ctx context.Context, _ *mcp.CallToolRequest, in TaxiRoutesInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.TaxiRoutes()
	return t.run(ctx, q.Service, q.Params, in.CommonQuery)
}

// --- Water quality ---

// WaterQualityInput is the input for the water_quality tool.
type WaterQualityInput struct {
	CommonQuery
}

func (t *Tools) waterQuality(ctx context.Context, _ *mcp.CallToolRequest, in WaterQualityInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.WaterQualityResults()
	return t.run(ctx, q.Service, q.Params, in.CommonQuery)
}

// --- Public lighting ---

// PublicLightingInput is the input for the public_lighting tool.
type PublicLightingInput struct {
	CommonQuery
}

func (t *Tools) publicLighting(ctx context.Context, _ *mcp.CallToolRequest, in PublicLightingInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.PublicLighting()
	return t.run(ctx, q.Service, q.Params, in.CommonQuery)
}

// --- Heritage inventory ---

// HeritageInventoryInput is the input for the heritage_inventory tool.
type HeritageInventoryInput struct {
	CommonQuery
}

func (t *Tools) heritageInventory(ctx context.Context, _ *mcp.CallToolRequest, in HeritageInventoryInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.HeritageInventory()
	return t.run(ctx, q.Service, q.Params, in.CommonQuery)
}

// registerDatasets registers all per-dataset tools.
func (t *Tools) registerDatasets(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "load_shedding_blocks",
		Description: "Load shedding (rolling blackout) block polygons for the City of Cape Town. The layer carries block geometry and a block ID only; use a where filter or bbox to narrow results.",
	}, t.loadShedding)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "wards",
		Description: "Municipal ward boundaries with ward name and year.",
	}, t.wards)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "land_parcels",
		Description: "Cadastral land parcel (erf) polygons with legal status, zoning, and suburb. Optionally filter by suburb.",
	}, t.landParcels)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "taxi_routes",
		Description: "Registered minibus taxi routes.",
	}, t.taxiRoutes)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "water_quality",
		Description: "Inland water quality sampling results (sample point, date, parameter, value), most recent first. This is a non-spatial table, so results carry no geometry.",
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
