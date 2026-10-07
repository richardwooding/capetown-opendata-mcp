package tools

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
	if n := fitCount(out.Layers); n < len(out.Layers) {
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
	Service string `json:"service" jsonschema:"ODP_SPLIT_*, SERVICE_REQUESTS or BUILDING_PLANS (see service_info)"`
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
		Name:           info.Name,
		Type:           info.Type,
		Description:    info.Description,
		GeometryType:   info.GeometryType,
		MaxRecordCount: info.MaxRecordCount,
		Fields:         make([]FieldInfo, 0, len(info.Fields)),
	}
	for _, f := range info.Fields {
		out.Fields = append(out.Fields, FieldInfo{Name: f.Name, Type: f.Type, Alias: f.Alias})
	}
	return nil, out, nil
}

func (t *Tools) registerDiscovery(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "service_info",
		Description: "List every layer and table in the Cape Town Open Data portal with the service that hosts it: thirteen ODP_SPLIT_* services plus the SERVICE_REQUESTS and BUILDING_PLANS tables. Use it to find the service + layer_id for other tools; name_contains filters by name.",
	}, t.serviceInfo)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "layer_info",
		Description: "Describe a layer's fields, geometry type and page size, to find valid names for where, fields and order_by.",
	}, t.layerInfo)
}
