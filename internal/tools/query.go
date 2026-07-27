package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	arcgis "github.com/richardwooding/go-arcgis"
)

// QueryLayerInput is the input for the generic query_layer tool.
type QueryLayerInput struct {
	CommonQuery
	Service   string   `json:"service" jsonschema:"the ODP_SPLIT_* feature service that hosts the layer (e.g. \"ODP_SPLIT_5\"); use service_info to discover which service a layer lives on"`
	LayerID   int      `json:"layer_id" jsonschema:"the layer ID within its service; use service_info to discover available service/layer_id pairs"`
	Fields    []string `json:"fields,omitempty" jsonschema:"attribute field names to return; omit for all fields"`
	OrderBy   []string `json:"order_by,omitempty" jsonschema:"fields to sort by, e.g. [\"CREATED_DATE DESC\"]"`
	CountOnly bool     `json:"count_only,omitempty" jsonschema:"return only the matching feature count rather than the features"`
}

// QueryLayerResult is returned for count-only queries; feature queries return a FeatureResult.
type QueryLayerResult struct {
	Count         int       `json:"count" jsonschema:"number of features returned (or total matching when count_only is true)"`
	Features      []Feature `json:"features" jsonschema:"the returned features (empty when count_only is true)"`
	ExceededLimit bool      `json:"exceeded_limit" jsonschema:"true if more features were available beyond the requested limit"`
	NextOffset    *int      `json:"next_offset,omitempty" jsonschema:"offset to pass on the next call to fetch the following page; present only when exceeded_limit is true"`
	CountOnly     bool      `json:"count_only" jsonschema:"echoes whether this was a count-only query"`
}

func (t *Tools) queryLayer(ctx context.Context, _ *mcp.CallToolRequest, in QueryLayerInput) (*mcp.CallToolResult, QueryLayerResult, error) {
	if err := validateService(in.Service); err != nil {
		return nil, QueryLayerResult{}, err
	}
	base := arcgis.QueryParams{
		LayerID:       in.LayerID,
		Fields:        in.Fields,
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
	}, nil
}

// FieldValuesInput is the input for the field_values tool.
type FieldValuesInput struct {
	Service string `json:"service" jsonschema:"the ODP_SPLIT_* feature service that hosts the layer (e.g. \"ODP_SPLIT_4\"); use service_info to discover it"`
	LayerID int    `json:"layer_id" jsonschema:"the layer ID within its service; use service_info to discover available service/layer_id pairs"`
	Field   string `json:"field" jsonschema:"the attribute field whose distinct values to list; use layer_info to find valid field names"`
	Where   string `json:"where,omitempty" jsonschema:"optional ArcGIS SQL WHERE filter to scope the values (e.g. \"WARD_NAME = '21'\")"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum number of distinct values to return (default 200, max 2000)"`
}

// FieldValuesResult lists the distinct values of a field.
type FieldValuesResult struct {
	Field         string `json:"field" jsonschema:"the field the values belong to"`
	Count         int    `json:"count" jsonschema:"number of distinct values returned"`
	Values        []any  `json:"values" jsonschema:"the distinct non-null values, sorted ascending"`
	ExceededLimit bool   `json:"exceeded_limit" jsonschema:"true if more distinct values were available beyond the requested limit"`
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
	return nil, FieldValuesResult{Field: in.Field, Count: len(values), Values: values, ExceededLimit: more}, nil
}

func (t *Tools) registerQuery(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "query_layer",
		Description: "Generic escape hatch to query any layer of the Cape Town Open Data portal by its service and layer ID. " +
			"Prefer the dedicated dataset tools when one exists. The portal is split across services named ODP_SPLIT_1..ODP_SPLIT_12; " +
			"use service_info to discover which service and layer_id you need. Supports a SQL WHERE filter, field selection, ordering, a bounding box, offset pagination, and count-only mode.",
	}, t.queryLayer)

	mcp.AddTool(s, &mcp.Tool{
		Name: "field_values",
		Description: "List the distinct values of a field on a layer. Use this to discover valid filter values before " +
			"querying — e.g. the suburb names available for land_parcels' suburb filter, or the set of ward names. Requires the layer's service (see service_info).",
	}, t.fieldValues)
}
