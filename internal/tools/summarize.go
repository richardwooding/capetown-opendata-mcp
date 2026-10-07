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
	Type  string `json:"type" jsonschema:"count, sum, avg, min, max, stddev or var"`
	Field string `json:"field,omitempty" jsonschema:"field to aggregate; optional for count"`
}

// SummarizeLayerInput is the input for the summarize_layer tool.
type SummarizeLayerInput struct {
	Service    string           `json:"service" jsonschema:"ODP_SPLIT_*, SERVICE_REQUESTS or BUILDING_PLANS (see service_info)"`
	LayerID    int              `json:"layer_id" jsonschema:"layer ID within the service"`
	GroupBy    []string         `json:"group_by,omitempty" jsonschema:"fields to group by; omit for one total"`
	Statistics []StatisticInput `json:"statistics,omitempty" jsonschema:"aggregates; default is record_count"`
	Where      string           `json:"where,omitempty" jsonschema:"SQL filter applied first; dates as DATE 'YYYY-MM-DD'"`
	OrderBy    []string         `json:"order_by,omitempty" jsonschema:"default: first statistic descending"`
	Limit      int              `json:"limit,omitempty" jsonschema:"max groups (default 25, max 2000)"`
}

// SummarizeLayerResult holds one row per group.
type SummarizeLayerResult struct {
	Count         int              `json:"count" jsonschema:"groups returned"`
	Groups        []map[string]any `json:"groups" jsonschema:"group_by values plus each statistic"`
	ExceededLimit bool             `json:"exceeded_limit" jsonschema:"true if more groups are available"`
	Note          string           `json:"note,omitempty" jsonschema:"set when trimmed to fit the size budget"`
}

// esapqa rejects "count" as an output field name, treating it as reserved.
const recordCountName = "record_count"

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
	res := SummarizeLayerResult{Groups: groups, ExceededLimit: fs.ExceededTransferLimit}
	if len(res.Groups) > limit {
		res.Groups, res.ExceededLimit = res.Groups[:limit], true
	}
	if n := fitCount(res.Groups); n < len(res.Groups) {
		res.Groups, res.ExceededLimit, res.Note = res.Groups[:n], true, trimmedNote
	}
	res.Count = len(res.Groups)
	return nil, res, nil
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
			name = recordCountName
		}
		out = append(out, arcgis.Statistic{Type: st, OnField: field, OutName: name})
	}
	return out, nil
}
