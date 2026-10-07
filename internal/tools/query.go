package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	arcgis "github.com/richardwooding/go-arcgis"
)

// QueryLayerInput is the input for the generic query_layer tool.
type QueryLayerInput struct {
	CommonQuery
	Service   string   `json:"service" jsonschema:"ODP_SPLIT_*, SERVICE_REQUESTS or BUILDING_PLANS (see service_info)"`
	LayerID   int      `json:"layer_id" jsonschema:"layer ID within the service"`
	OrderBy   []string `json:"order_by,omitempty" jsonschema:"sort, e.g. [\"CREATED_DATE DESC\"]"`
	CountOnly bool     `json:"count_only,omitempty" jsonschema:"return only the matching count"`
}

// QueryLayerResult is returned for count-only queries; feature queries return a FeatureResult.
type QueryLayerResult struct {
	Count         int       `json:"count" jsonschema:"features returned, or total matching for count_only"`
	Features      []Feature `json:"features" jsonschema:"the features (empty for count_only)"`
	ExceededLimit bool      `json:"exceeded_limit" jsonschema:"true if more features are available"`
	NextOffset    *int      `json:"next_offset,omitempty" jsonschema:"offset for the next page"`
	Note          string    `json:"note,omitempty" jsonschema:"set when the page was trimmed to fit the size budget"`
	CountOnly     bool      `json:"count_only" jsonschema:"echoes count_only"`
}

func (t *Tools) queryLayer(ctx context.Context, _ *mcp.CallToolRequest, in QueryLayerInput) (*mcp.CallToolResult, QueryLayerResult, error) {
	if err := validateService(in.Service); err != nil {
		return nil, QueryLayerResult{}, err
	}
	base := arcgis.QueryParams{
		LayerID:       in.LayerID,
		OrderByFields: in.OrderBy,
	}
	if in.CountOnly {
		p := applyCommon(base, in.CommonQuery)
		n, err := t.client.Count(ctx, in.Service, p)
		if err != nil {
			return nil, QueryLayerResult{}, annotateErr(err, in.Service, in.LayerID)
		}
		return nil, QueryLayerResult{Count: n, Features: []Feature{}, CountOnly: true}, nil
	}
	_, fr, err := t.run(ctx, in.Service, base, in.CommonQuery)
	if err != nil {
		return nil, QueryLayerResult{}, err
	}
	return nil, QueryLayerResult{
		Count:         fr.Count,
		Features:      fr.Features,
		ExceededLimit: fr.ExceededLimit,
		NextOffset:    fr.NextOffset,
		Note:          fr.Note,
	}, nil
}

// FieldValuesInput is the input for the field_values tool.
type FieldValuesInput struct {
	Service string `json:"service" jsonschema:"ODP_SPLIT_*, SERVICE_REQUESTS or BUILDING_PLANS (see service_info)"`
	LayerID int    `json:"layer_id" jsonschema:"layer ID within the service"`
	Field   string `json:"field" jsonschema:"field to list values of (see layer_info)"`
	Where   string `json:"where,omitempty" jsonschema:"optional SQL filter"`
	Limit   int    `json:"limit,omitempty" jsonschema:"max values (default 25, max 2000)"`
}

// FieldValuesResult lists the distinct values of a field.
type FieldValuesResult struct {
	Field         string `json:"field" jsonschema:"the field"`
	Count         int    `json:"count" jsonschema:"values returned"`
	Values        []any  `json:"values" jsonschema:"distinct non-null values, ascending"`
	ExceededLimit bool   `json:"exceeded_limit" jsonschema:"true if more values are available"`
	Note          string `json:"note,omitempty" jsonschema:"set when trimmed to fit the size budget"`
}

func (t *Tools) fieldValues(ctx context.Context, _ *mcp.CallToolRequest, in FieldValuesInput) (*mcp.CallToolResult, FieldValuesResult, error) {
	if err := validateService(in.Service); err != nil {
		return nil, FieldValuesResult{}, err
	}
	limit := effectiveLimit(in.Limit)
	no := false
	p := arcgis.QueryParams{
		LayerID:              in.LayerID,
		Fields:               []string{in.Field},
		OrderByFields:        []string{in.Field},
		ReturnDistinctValues: true,
		ReturnGeometry:       &no,
		PageSize:             limit,
		Where:                in.Where,
	}
	feats, more, err := t.client.QueryLimit(ctx, in.Service, p, limit)
	if err != nil {
		return nil, FieldValuesResult{}, annotateErr(err, in.Service, in.LayerID)
	}
	values := make([]any, 0, len(feats))
	for _, f := range feats {
		if v := f.Attrs()[in.Field]; v != nil {
			values = append(values, v)
		}
	}
	res := FieldValuesResult{Field: in.Field, Values: values, ExceededLimit: more}
	if n := fitCount(values); n < len(values) {
		res.Values, res.ExceededLimit, res.Note = values[:n], true, trimmedNote
	}
	res.Count = len(res.Values)
	return nil, res, nil
}

func (t *Tools) registerQuery(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "query_layer",
		Description: "Query any layer by service and layer_id (see service_info). Prefer the dataset tools, and summarize_layer for counts. " +
			"Supports where, fields, order_by, bbox/polygon, offset paging and count_only.",
	}, t.queryLayer)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "field_values",
		Description: "List a field's distinct values on a layer, to find valid filter values (e.g. suburb or ward names).",
	}, t.fieldValues)

	mcp.AddTool(s, &mcp.Tool{
		Name: "summarize_layer",
		Description: "Server-side count/sum/avg/min/max over any layer, optionally grouped. Use for \"how many\" or \"which most\" questions instead of paging rows; " +
			"essential for the 5-million-row SERVICE_REQUESTS table. Example: service=SERVICE_REQUESTS, layer_id=0, group_by=[\"C3_Complaint_Type\"], " +
			"where=\"Ward = '062' AND Created_On_Date >= DATE '2026-01-01'\". Default: record_count per group, largest first.",
	}, t.summarizeLayer)
}
