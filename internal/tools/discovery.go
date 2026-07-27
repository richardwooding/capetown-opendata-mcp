package tools

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cct"
)

// ServiceInfoInput is the input for the service_info tool.
type ServiceInfoInput struct {
	NameContains string `json:"name_contains,omitempty" jsonschema:"case-insensitive substring; when set, only layers and tables whose name contains it are returned"`
}

// ServiceInfoResult is the merged layer catalogue across every split service.
type ServiceInfoResult struct {
	Layers      []cct.ServiceLayer       `json:"layers" jsonschema:"all layers and tables across the ODP_SPLIT_* services, each tagged with the service that hosts it (feed service + id to layer_info and query_layer)"`
	Unavailable []cct.UnavailableService `json:"unavailable,omitempty" jsonschema:"split services that could not be listed right now (e.g. stopped or mid-restructure upstream)"`
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
	Service string `json:"service" jsonschema:"the ODP_SPLIT_* feature service that hosts the layer (e.g. \"ODP_SPLIT_5\"); use service_info to discover it"`
	LayerID int    `json:"layer_id" jsonschema:"the layer ID within its service to describe"`
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
		Description: "List every layer and table across the Cape Town Open Data portal, each tagged with the ODP_SPLIT_* service that hosts it. The portal is split across a dozen services; this aggregates them into one catalogue. Use it to discover the service + layer_id to pass to layer_info and query_layer. The portal publishes 150+ layers; pass name_contains to filter by name (e.g. \"water\").",
	}, t.serviceInfo)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "layer_info",
		Description: "Describe a single layer: its field names/types, geometry type, and maximum page size. Use this to learn which fields are valid for where/fields/order_by. Requires the layer's service (see service_info).",
	}, t.layerInfo)
}
