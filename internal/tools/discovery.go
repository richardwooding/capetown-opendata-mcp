package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LayerRef is a lightweight layer reference in a service listing.
type LayerRef struct {
	ID   int    `json:"id" jsonschema:"the layer ID used by query_layer and layer_info"`
	Name string `json:"name" jsonschema:"the layer's display name"`
}

// ServiceInfoResult describes the feature service and its layers.
type ServiceInfoResult struct {
	Description string     `json:"description" jsonschema:"the service description"`
	Layers      []LayerRef `json:"layers" jsonschema:"queryable feature layers"`
	Tables      []LayerRef `json:"tables" jsonschema:"non-spatial tables"`
}

func (t *Tools) serviceInfo(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ServiceInfoResult, error) {
	info, err := t.client.ServiceInfo(ctx)
	if err != nil {
		return nil, ServiceInfoResult{}, err
	}
	out := ServiceInfoResult{
		Description: info.ServiceDescription,
		Layers:      make([]LayerRef, 0, len(info.Layers)),
		Tables:      make([]LayerRef, 0, len(info.Tables)),
	}
	for _, l := range info.Layers {
		out.Layers = append(out.Layers, LayerRef{ID: l.ID, Name: l.Name})
	}
	for _, tbl := range info.Tables {
		out.Tables = append(out.Tables, LayerRef{ID: tbl.ID, Name: tbl.Name})
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
	LayerID int `json:"layer_id" jsonschema:"the ArcGIS layer ID to describe"`
}

// LayerInfoResult describes a single layer's schema.
type LayerInfoResult struct {
	ID             int         `json:"id"`
	Name           string      `json:"name"`
	Type           string      `json:"type"`
	Description    string      `json:"description"`
	GeometryType   string      `json:"geometry_type"`
	MaxRecordCount int         `json:"max_record_count" jsonschema:"the server's maximum features per page"`
	Fields         []FieldInfo `json:"fields"`
}

func (t *Tools) layerInfo(ctx context.Context, _ *mcp.CallToolRequest, in LayerInfoInput) (*mcp.CallToolResult, LayerInfoResult, error) {
	info, err := t.client.LayerInfo(ctx, in.LayerID)
	if err != nil {
		return nil, LayerInfoResult{}, err
	}
	out := LayerInfoResult{
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
		Description: "List the layers and tables published by the Cape Town Open Data Feature Service, with their IDs. Use this to discover what is queryable and to verify layer IDs.",
	}, t.serviceInfo)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "layer_info",
		Description: "Describe a single layer: its field names/types, geometry type, and maximum page size. Use this to learn which fields are valid for where/fields/order_by.",
	}, t.layerInfo)
}
