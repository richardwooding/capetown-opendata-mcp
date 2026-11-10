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

// --- Official suburbs ---

// OfficialSuburbsInput is the input for the official_suburbs tool.
type OfficialSuburbsInput struct {
	CommonQuery
	Suburb string `json:"suburb,omitempty" jsonschema:"suburb name to look up (case-insensitive)"`
}

func (t *Tools) officialSuburbs(ctx context.Context, _ *mcp.CallToolRequest, in OfficialSuburbsInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.OfficialSuburbs()
	if in.Suburb != "" {
		q = capetown.OfficialSuburbByName(in.Suburb)
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

// --- ArcGIS Online tables ---

// TableQuery holds the filters for non-spatial tables, which take no spatial
// filter or geometry.
type TableQuery struct {
	Limit      int      `json:"limit,omitempty" jsonschema:"max rows (default 25, max 2000)"`
	Cursor     string   `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page, unchanged, with the same filters; never add your own ObjectId or offset filters to page"`
	Where      string   `json:"where,omitempty" jsonschema:"SQL filter; dates as DATE 'YYYY-MM-DD'"`
	Fields     []string `json:"fields,omitempty" jsonschema:"columns to return; omit for all"`
	OmitNulls  *bool    `json:"omit_nulls,omitempty" jsonschema:"drop null and empty values (default true)"`
	UseAliases bool     `json:"use_aliases,omitempty" jsonschema:"use readable field aliases as keys"`
}

func (q TableQuery) common() CommonQuery {
	return CommonQuery{Limit: q.Limit, Cursor: q.Cursor, Where: q.Where, Fields: q.Fields, OmitNulls: q.OmitNulls, UseAliases: q.UseAliases}
}

// ServiceRequestsInput is the input for the service_requests tool.
type ServiceRequestsInput struct {
	TableQuery
}

func (t *Tools) serviceRequests(ctx context.Context, _ *mcp.CallToolRequest, in ServiceRequestsInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.ServiceRequests()
	return t.run(ctx, q.Service, q.Params, in.common())
}

// ServiceRequestsSpatialInput is the input for the service_requests_spatial
// tool. It is a point layer, so the full spatial filter set applies.
type ServiceRequestsSpatialInput struct {
	CommonQuery
}

func (t *Tools) serviceRequestsSpatial(ctx context.Context, _ *mcp.CallToolRequest, in ServiceRequestsSpatialInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.ServiceRequestsSpatial()
	return t.run(ctx, q.Service, q.Params, in.CommonQuery)
}

// BuildingPlansInput is the input for the building_plan_approvals tool.
type BuildingPlansInput struct {
	TableQuery
}

func (t *Tools) buildingPlans(ctx context.Context, _ *mcp.CallToolRequest, in BuildingPlansInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.BuildingPlanApprovals()
	return t.run(ctx, q.Service, q.Params, in.common())
}

// PopulationEstimatesInput is the input for the population_estimates tool.
type PopulationEstimatesInput struct {
	TableQuery
	Suburb string `json:"suburb,omitempty" jsonschema:"suburb name to look up (case-insensitive), e.g. Athlone"`
}

func (t *Tools) populationEstimates(ctx context.Context, _ *mcp.CallToolRequest, in PopulationEstimatesInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.PopulationEstimates()
	if in.Suburb != "" {
		q = capetown.PopulationEstimateForSuburb(in.Suburb)
	}
	return t.run(ctx, q.Service, q.Params, in.common())
}

// TaxiFaresInput is the input for the taxi_fares tool.
type TaxiFaresInput struct {
	TableQuery
	OriginRank string `json:"origin_rank,omitempty" jsonschema:"origin rank reference code, e.g. MBT019 (Origin_Rank_Ref_No); use field_values or a where on Origin_Rank_Name to find one"`
}

func (t *Tools) taxiFares(ctx context.Context, _ *mcp.CallToolRequest, in TaxiFaresInput) (*mcp.CallToolResult, FeatureResult, error) {
	q := capetown.TaxiFares()
	if in.OriginRank != "" {
		q = capetown.TaxiFaresFromRank(in.OriginRank)
	}
	return t.run(ctx, q.Service, q.Params, in.common())
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
		Name: "official_suburbs",
		Description: "Official suburb boundary polygons (about 780) with the suburb name in OFC_SBRB_NAME (upper case) and its key. " +
			"Optionally look one up by suburb (case-insensitive). Use geometry_detail \"centroid\" to place suburbs on a map. " +
			"It is the shape behind suburb-level tables: join population_estimates by name (case-insensitive), and land_parcels shares the same suburb column.",
	}, t.officialSuburbs)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "taxi_routes",
		Description: "Registered minibus taxi route lines, each with an origin (ORGN) and destination (DSTN) name. For surveyed fares between ranks use taxi_fares.",
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

	mcp.AddTool(s, &mcp.Tool{
		Name: "service_requests",
		Description: "Citizen service requests (C3 notifications) logged with the City since 2023, most recently loaded first: about 4.5 million rows, refreshed regularly. " +
			"A non-spatial table with Sub_Council, Ward, Suburb and Suburb_Clean, C3_Complaint_Type, Work_Center, Notification_type, Longitude/Latitude columns, " +
			"and CreatedOnDT, ChangedOnDT and CompletedDT dates. " +
			"Ward is a zero-padded string (Ward = '062'); many rows carry a placeholder ward such as '000' or none. " +
			"For how current the data is, use layer_info's data_range, not the table's name. " +
			"Filter dates with DATE 'YYYY-MM-DD'. For counts or rankings (e.g. top complaint types in a ward) use summarize_layer with service=SERVICE_REQUESTS, layer_id=0 rather than paging rows. " +
			"For location questions (near a place, inside an area, on a map) use service_requests_spatial.",
	}, t.serviceRequests)

	mcp.AddTool(s, &mcp.Tool{
		Name: "service_requests_spatial",
		Description: "The geocoded subset of service_requests as points: about 3.3 million of the 4.5 million notifications, newest first, with the same attribute columns " +
			"except that its dates are Created_On_Date, Changed_on and Completed_Date (not CreatedOnDT). " +
			"Use it for \"near\", \"in this area\" or map questions: filter with bbox or polygon, and plot with include_geometry and geometry_detail \"centroid\". " +
			"For counts inside an area use summarize_layer with service=SERVICE_REQUESTS_SPATIAL, layer_id=0 and the same bbox-sized where, or query_layer with count_only. " +
			"For every notification regardless of coordinates, use service_requests.",
	}, t.serviceRequestsSpatial)

	mcp.AddTool(s, &mcp.Tool{
		Name: "building_plan_approvals",
		Description: "Building plan applications submitted to the City since 2014, newest submission first: about 250,000 rows. " +
			"A non-spatial table with case type, suburb, erf number, plan category, work description, number of units, area and value of new work, " +
			"submission and approval dates, and financial year. Ward_No is an integer (Ward_No = 62). " +
			"For how current the data is, use layer_info's data_range, not the table's name. " +
			"Filter dates with DATE 'YYYY-MM-DD'. For totals or trends (e.g. approvals per financial year) use summarize_layer with service=BUILDING_PLANS, layer_id=0.",
	}, t.buildingPlans)

	mcp.AddTool(s, &mcp.Tool{
		Name: "population_estimates",
		Description: "The City's modelled population and household estimates for about 190 official suburbs (Suburb_Name, Estimated_population, Estimated_households), ordered by suburb. " +
			"An annual planning estimate built from the 2022 census, not an official count; values are fractional, so round them. " +
			"Look one suburb up with suburb (case-insensitive). A non-spatial table: to map it, join by name to official_suburbs (OFC_SBRB_NAME, upper case). " +
			"For the city total or rankings use summarize_layer with service=POPULATION_ESTIMATES, layer_id=0 and a sum of Estimated_population. " +
			"layer_info's source says when the City last updated the estimates.",
	}, t.populationEstimates)

	mcp.AddTool(s, &mcp.Tool{
		Name: "taxi_fares",
		Description: "Minibus taxi fares surveyed at ranks, one row per origin-destination rank pair with the average fare in rands (Fare) and the survey date, newest first: about 1,100 rows. " +
			"Each end has a rank reference (Origin_Rank_Ref_No / Destination_Rank_Ref_No, e.g. MBT019), a name and WGS84 coordinates as plain columns " +
			"(O___X_Coordinate is longitude, O___Y_Coordinate latitude; likewise D___*); the table has no geometry. Filter departures with origin_rank. " +
			"For the surveyed period use layer_info's data_range. For average fares by origin or destination use summarize_layer with service=TAXI_FARES, layer_id=0. " +
			"taxi_routes holds the registered route lines.",
	}, t.taxiFares)
}
