package tools

import (
	"context"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	capetown "github.com/richardwooding/capetown-opendata"
	arcgis "github.com/richardwooding/go-arcgis"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cct"
)

// ServiceInfoInput is the input for the service_info tool.
type ServiceInfoInput struct {
	NameContains string `json:"name_contains,omitempty" jsonschema:"case-insensitive name filter, e.g. \"water\""`
}

// ServiceInfoResult is the merged layer catalogue across every split service.
type ServiceInfoResult struct {
	Layers      []cct.ServiceLayer       `json:"layers" jsonschema:"layers and tables, each with its service and id"`
	Unavailable []cct.UnavailableService `json:"unavailable,omitempty" jsonschema:"services that could not be listed right now"`
	Note        string                   `json:"note,omitempty" jsonschema:"set when trimmed; narrow with name_contains"`
}

func (t *Tools) serviceInfo(ctx context.Context, _ *mcp.CallToolRequest, in ServiceInfoInput) (*mcp.CallToolResult, ServiceInfoResult, error) {
	agg := t.client.ServiceInfoAll(ctx)
	needle := strings.ToLower(strings.TrimSpace(in.NameContains))
	out := ServiceInfoResult{
		Layers:      make([]cct.ServiceLayer, 0, len(agg.Layers)),
		Unavailable: agg.Unavailable,
	}
	for _, l := range agg.Layers {
		if needle == "" || strings.Contains(strings.ToLower(l.Name), needle) {
			out.Layers = append(out.Layers, l)
		}
	}
	if n := fitCount(out.Layers, t.budgetChars()); n < len(out.Layers) {
		out.Layers, out.Note = out.Layers[:n], "Trimmed to fit the response size budget; narrow the listing with name_contains."
	}
	return nil, out, nil
}

// FieldInfo describes a single attribute field of a layer.
type FieldInfo struct {
	Name  string `json:"name" jsonschema:"the field's name, usable in where/fields/order_by"`
	Type  string `json:"type" jsonschema:"the Esri field type"`
	Alias string `json:"alias" jsonschema:"the field's human-readable alias"`
}

// LayerInfoInput is the input for the layer_info tool.
type LayerInfoInput struct {
	Service string `json:"service" jsonschema:"ODP_SPLIT_* or an ArcGIS Online table key: SERVICE_REQUESTS, BUILDING_PLANS, POPULATION_ESTIMATES or TAXI_FARES (see service_info)"`
	LayerID int    `json:"layer_id" jsonschema:"layer ID within the service"`
}

// LayerInfoResult describes a single layer's schema.
type LayerInfoResult struct {
	Service        string      `json:"service"`
	ID             int         `json:"id"`
	Name           string      `json:"name"`
	Type           string      `json:"type"`
	Description    string      `json:"description"`
	GeometryType   string      `json:"geometry_type"`
	MaxRecordCount int         `json:"max_record_count" jsonschema:"the server's maximum features per page"`
	Fields         []FieldInfo `json:"fields"`
	DataRange      *DataRange  `json:"data_range,omitempty" jsonschema:"earliest and latest record dates for time-bound tables; judge how current the data is from this, never from a table name"`
	Source         *SourceInfo `json:"source,omitempty" jsonschema:"the hub item behind an ArcGIS Online table: the City's title and summary for the data and when it last updated the item"`
}

// SourceInfo describes the ArcGIS Online hub item that publishes a table.
type SourceInfo struct {
	ItemID   string `json:"item_id"`
	Title    string `json:"title"`
	Summary  string `json:"summary,omitempty" jsonschema:"the City's one-paragraph description of the data"`
	Modified string `json:"modified,omitempty" jsonschema:"YYYY-MM-DD the City last updated the item"`
}

// DataRange is the span of a table's records on its main date field.
type DataRange struct {
	Field    string `json:"field"`
	Earliest string `json:"earliest" jsonschema:"YYYY-MM-DD"`
	Latest   string `json:"latest" jsonschema:"YYYY-MM-DD"`
}

// capeTown is SAST (UTC+2, no daylight saving). The City stores dates as local
// midnight, so formatting in UTC would shift every date back a day.
var capeTown = time.FixedZone("SAST", 2*60*60)

// The ArcGIS Online tables are refreshed in place under names that embed old
// dates, so their real coverage is measured from the data itself.
var dateFields = map[string]string{
	capetown.ServiceServiceRequests: "CreatedOnDT",
	capetown.ServiceBuildingPlans:   "Submission_Date",
	capetown.ServiceTaxiFares:       "Survey_Date",
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// plainText flattens the HTML the portal stores in item snippets to one line.
func plainText(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(htmlTag.ReplaceAllString(s, " "))), " ")
}

func (t *Tools) source(service string) *SourceInfo {
	item := t.client.HubItem(service)
	if item == nil {
		return nil
	}
	out := &SourceInfo{ItemID: item.ID, Title: plainText(item.Title), Summary: plainText(item.Snippet)}
	if m := item.ModifiedTime(); !m.IsZero() {
		out.Modified = m.In(capeTown).Format(time.DateOnly)
	}
	return out
}

func (t *Tools) dataRange(ctx context.Context, service string, layerID int) *DataRange {
	field, ok := dateFields[service]
	if !ok {
		return nil
	}
	fs, err := t.client.Statistics(ctx, service, arcgis.QueryParams{
		LayerID: layerID,
		OutStatistics: []arcgis.Statistic{
			{Type: arcgis.StatMin, OnField: field, OutName: "earliest"},
			{Type: arcgis.StatMax, OnField: field, OutName: "latest"},
		},
	})
	if err != nil || len(fs.Features) == 0 {
		return nil
	}
	attrs := fs.Features[0].Attrs()
	lo, okLo := asInt64(attrs["earliest"])
	hi, okHi := asInt64(attrs["latest"])
	if !okLo || !okHi {
		return nil
	}
	day := func(ms int64) string { return time.UnixMilli(ms).In(capeTown).Format(time.DateOnly) }
	return &DataRange{Field: field, Earliest: day(lo), Latest: day(hi)}
}

func (t *Tools) layerInfo(ctx context.Context, _ *mcp.CallToolRequest, in LayerInfoInput) (*mcp.CallToolResult, LayerInfoResult, error) {
	if err := validateService(in.Service); err != nil {
		return nil, LayerInfoResult{}, err
	}
	info, err := t.client.LayerInfo(ctx, in.Service, in.LayerID)
	if err != nil {
		return nil, LayerInfoResult{}, annotateErr(err, in.Service, in.LayerID)
	}
	out := LayerInfoResult{
		Service:        in.Service,
		ID:             info.ID,
		Name:           t.client.LayerLabel(in.Service, info.Name),
		Type:           info.Type,
		Description:    info.Description,
		GeometryType:   info.GeometryType,
		MaxRecordCount: info.MaxRecordCount,
		Fields:         make([]FieldInfo, 0, len(info.Fields)),
	}
	for _, f := range info.Fields {
		out.Fields = append(out.Fields, FieldInfo{Name: f.Name, Type: f.Type, Alias: f.Alias})
	}
	out.DataRange = t.dataRange(ctx, in.Service, in.LayerID)
	out.Source = t.source(in.Service)
	return nil, out, nil
}

func (t *Tools) registerDiscovery(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "service_info",
		Description: "List every layer and table in the Cape Town Open Data portal with the service that hosts it: thirteen ODP_SPLIT_* services plus the ArcGIS Online tables SERVICE_REQUESTS, BUILDING_PLANS, POPULATION_ESTIMATES and TAXI_FARES. Use it to find the service + layer_id for other tools; name_contains filters by name.",
	}, t.serviceInfo)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "layer_info",
		Description: "Describe a layer's fields, geometry type and page size, to find valid names for where, fields and order_by. For ArcGIS Online tables it adds data_range (actual record dates) and source (the City's title, summary and last update of the hub item).",
	}, t.layerInfo)
}
