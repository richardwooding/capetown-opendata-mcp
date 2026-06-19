package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	arcgis "github.com/richardwooding/go-arcgis"
)

// QueryLayerInput is the input for the generic query_layer tool.
type QueryLayerInput struct {
	CommonQuery
	LayerID   int      `json:"layer_id" jsonschema:"the ArcGIS layer ID to query; use service_info to discover available layer IDs"`
	Fields    []string `json:"fields,omitempty" jsonschema:"attribute field names to return; omit for all fields"`
	OrderBy   []string `json:"order_by,omitempty" jsonschema:"fields to sort by, e.g. [\"CREATED_DATE DESC\"]"`
	CountOnly bool     `json:"count_only,omitempty" jsonschema:"return only the matching feature count rather than the features"`
}

// QueryLayerResult is returned for count-only queries; feature queries return a FeatureResult.
type QueryLayerResult struct {
	Count         int       `json:"count" jsonschema:"number of features returned (or total matching when count_only is true)"`
	Features      []Feature `json:"features" jsonschema:"the returned features (empty when count_only is true)"`
	ExceededLimit bool      `json:"exceeded_limit" jsonschema:"true if more features were available beyond the requested limit"`
	CountOnly     bool      `json:"count_only" jsonschema:"echoes whether this was a count-only query"`
}

func (t *Tools) queryLayer(ctx context.Context, _ *mcp.CallToolRequest, in QueryLayerInput) (*mcp.CallToolResult, QueryLayerResult, error) {
	base := arcgis.QueryParams{
		LayerID:       in.LayerID,
		Fields:        in.Fields,
		OrderByFields: in.OrderBy,
	}
	if in.CountOnly {
		p := applyCommon(base, in.CommonQuery)
		n, err := t.client.Count(ctx, p)
		if err != nil {
			return nil, QueryLayerResult{}, err
		}
		return nil, QueryLayerResult{Count: n, Features: []Feature{}, CountOnly: true}, nil
	}
	_, fr, err := t.run(ctx, base, in.CommonQuery)
	if err != nil {
		return nil, QueryLayerResult{}, err
	}
	return nil, QueryLayerResult{
		Count:         fr.Count,
		Features:      fr.Features,
		ExceededLimit: fr.ExceededLimit,
	}, nil
}

func (t *Tools) registerQuery(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "query_layer",
		Description: "Generic escape hatch to query any layer of the Cape Town Open Data Feature Service by its ID. " +
			"Prefer the dedicated dataset tools when one exists. Use service_info and layer_info to discover layer IDs and field names " +
			"(published layer IDs occasionally drift). Supports a SQL WHERE filter, field selection, ordering, a bounding box, and count-only mode.",
	}, t.queryLayer)
}
