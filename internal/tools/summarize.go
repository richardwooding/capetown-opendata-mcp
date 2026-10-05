package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	arcgis "github.com/richardwooding/go-arcgis"
)

// StatisticInput is one aggregate to compute.
type StatisticInput struct {
	Type  string `json:"type" jsonschema:"aggregate function: count, sum, avg, min, max, stddev or var"`
	Field string `json:"field,omitempty" jsonschema:"field to aggregate; may be omitted for count, which then counts records"`
}

// SummarizeLayerInput is the input for the summarize_layer tool.
type SummarizeLayerInput struct {
	Service    string           `json:"service" jsonschema:"the service that hosts the layer: an ODP_SPLIT_* service, SERVICE_REQUESTS or BUILDING_PLANS; use service_info to discover it"`
	LayerID    int              `json:"layer_id" jsonschema:"the layer ID within its service"`
	GroupBy    []string         `json:"group_by,omitempty" jsonschema:"fields to group by, e.g. [\"Ward\"]; omit for a single overall total"`
	Statistics []StatisticInput `json:"statistics,omitempty" jsonschema:"aggregates to compute; defaults to a record count named \"count\""`
	Where      string           `json:"where,omitempty" jsonschema:"ArcGIS SQL WHERE filter applied before aggregating; dates use DATE 'YYYY-MM-DD'"`
	OrderBy    []string         `json:"order_by,omitempty" jsonschema:"ordering over group or output fields, e.g. [\"count DESC\"]; defaults to the first statistic descending"`
	Limit      int              `json:"limit,omitempty" jsonschema:"maximum number of groups to return (default 200, max 2000)"`
}

// SummarizeLayerResult holds one row per group.
type SummarizeLayerResult struct {
	Count         int              `json:"count" jsonschema:"number of groups returned"`
	Groups        []map[string]any `json:"groups" jsonschema:"one entry per group: the group_by values plus each statistic under its output name"`
	ExceededLimit bool             `json:"exceeded_limit" jsonschema:"true if more groups were available beyond the requested limit"`
}

var statisticTypes = map[string]arcgis.StatisticType{
	"count":  arcgis.StatCount,
	"sum":    arcgis.StatSum,
	"avg":    arcgis.StatAvg,
	"min":    arcgis.StatMin,
	"max":    arcgis.StatMax,
	"stddev": arcgis.StatStddev,
	"var":    arcgis.StatVar,
}

func (t *Tools) summarizeLayer(ctx context.Context, _ *mcp.CallToolRequest, in SummarizeLayerInput) (*mcp.CallToolResult, SummarizeLayerResult, error) {
	if err := validateService(in.Service); err != nil {
		return nil, SummarizeLayerResult{}, err
	}
	stats, err := t.buildStatistics(ctx, in)
	if err != nil {
		return nil, SummarizeLayerResult{}, err
	}
	order := in.OrderBy
	if len(order) == 0 {
		order = []string{stats[0].OutName + " DESC"}
	}
	limit := effectiveLimit(in.Limit)
	no := false
	p := arcgis.QueryParams{
		LayerID:        in.LayerID,
		Where:          in.Where,
		GroupByFields:  in.GroupBy,
		OutStatistics:  stats,
		OrderByFields:  order,
		PageSize:       limit,
		ReturnGeometry: &no,
	}
	fs, err := t.client.Statistics(ctx, in.Service, p)
	if err != nil {
		return nil, SummarizeLayerResult{}, annotateErr(err, in.Service, in.LayerID)
	}
	groups := make([]map[string]any, 0, len(fs.Features))
	for _, f := range fs.Features {
		groups = append(groups, f.Attrs())
	}
	more := fs.ExceededTransferLimit
	if len(groups) > limit {
		groups = groups[:limit]
		more = true
	}
	return nil, SummarizeLayerResult{Count: len(groups), Groups: groups, ExceededLimit: more}, nil
}

func (t *Tools) buildStatistics(ctx context.Context, in SummarizeLayerInput) ([]arcgis.Statistic, error) {
	specs := in.Statistics
	if len(specs) == 0 {
		specs = []StatisticInput{{Type: "count"}}
	}
	out := make([]arcgis.Statistic, 0, len(specs))
	for _, s := range specs {
		kind := strings.ToLower(strings.TrimSpace(s.Type))
		st, ok := statisticTypes[kind]
		if !ok {
			return nil, fmt.Errorf("unknown statistic type %q; use count, sum, avg, min, max, stddev or var", s.Type)
		}
		field := strings.TrimSpace(s.Field)
		name := kind + "_" + field
		if field == "" {
			if st != arcgis.StatCount {
				return nil, fmt.Errorf("statistic %q needs a field; call layer_info to list numeric fields", kind)
			}
			field = t.client.OIDField(ctx, in.Service, in.LayerID)
			if field == "" {
				return nil, fmt.Errorf("could not determine the record ID field to count; pass a field explicitly")
			}
			name = "count"
		}
		out = append(out, arcgis.Statistic{Type: st, OnField: field, OutName: name})
	}
	return out, nil
}
