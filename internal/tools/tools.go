// Package tools defines the MCP tools exposed by the Cape Town Open Data server
// and registers them against an *mcp.Server. Each tool maps user-friendly input
// onto an ArcGIS query and returns structured output.
package tools

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	arcgis "github.com/richardwooding/go-arcgis"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cct"
)

const (
	defaultLimit = 200
	maxLimit     = 2000
)

// Tools holds the dependencies shared by all tool handlers.
type Tools struct {
	client *cct.Client
}

// New returns a Tools backed by the given client.
func New(client *cct.Client) *Tools { return &Tools{client: client} }

// CommonQuery holds filters shared by every feature-returning tool.
type CommonQuery struct {
	Limit           int       `json:"limit,omitempty" jsonschema:"maximum number of features to return (default 200, max 2000)"`
	Where           string    `json:"where,omitempty" jsonschema:"additional ArcGIS SQL WHERE filter, AND-combined with the dataset's default filter (e.g. \"OBJECTID > 100\"); use layer_info to find valid field names"`
	BBox            []float64 `json:"bbox,omitempty" jsonschema:"spatial bounding-box filter as [minLon, minLat, maxLon, maxLat] in WGS84 degrees"`
	IncludeGeometry bool      `json:"include_geometry,omitempty" jsonschema:"include feature geometry in the response (default false, which yields smaller attribute-only payloads)"`
}

// Feature is a single returned feature.
type Feature struct {
	Attributes map[string]any `json:"attributes" jsonschema:"the feature's attribute values keyed by field name"`
	Geometry   any            `json:"geometry,omitempty" jsonschema:"GeoJSON geometry, present only when include_geometry is true"`
}

// FeatureResult is the structured output of feature-returning tools.
type FeatureResult struct {
	Count         int       `json:"count" jsonschema:"number of features returned"`
	Features      []Feature `json:"features" jsonschema:"the returned features"`
	ExceededLimit bool      `json:"exceeded_limit" jsonschema:"true if more features were available beyond the requested limit"`
}

// run applies the common filters to a base query, executes it, and shapes the result.
func (t *Tools) run(ctx context.Context, base arcgis.QueryParams, c CommonQuery) (*mcp.CallToolResult, FeatureResult, error) {
	p := applyCommon(base, c)
	feats, more, err := t.client.QueryLimit(ctx, p, effectiveLimit(c.Limit))
	if err != nil {
		return nil, FeatureResult{}, err
	}
	return nil, toResult(feats, more, c.IncludeGeometry), nil
}

// applyCommon overlays CommonQuery filters onto a base query.
func applyCommon(p arcgis.QueryParams, c CommonQuery) arcgis.QueryParams {
	if c.Where != "" {
		if p.Where != "" {
			p.Where = "(" + p.Where + ") AND (" + c.Where + ")"
		} else {
			p.Where = c.Where
		}
	}
	if len(c.BBox) == 4 {
		p.Envelope = &arcgis.Envelope{MinX: c.BBox[0], MinY: c.BBox[1], MaxX: c.BBox[2], MaxY: c.BBox[3]}
	}
	limit := effectiveLimit(c.Limit)
	p.PageSize = limit
	if !c.IncludeGeometry {
		no := false
		p.ReturnGeometry = &no
	}
	return p
}

func effectiveLimit(n int) int {
	switch {
	case n <= 0:
		return defaultLimit
	case n > maxLimit:
		return maxLimit
	default:
		return n
	}
}

// toResult converts raw features into the structured tool output.
func toResult(feats []arcgis.Feature, more, includeGeometry bool) FeatureResult {
	out := FeatureResult{Count: len(feats), ExceededLimit: more, Features: make([]Feature, 0, len(feats))}
	for _, f := range feats {
		fe := Feature{Attributes: f.Attrs()}
		if includeGeometry && len(f.Geometry) > 0 {
			var g any
			if json.Unmarshal(f.Geometry, &g) == nil {
				fe.Geometry = g
			}
		}
		out.Features = append(out.Features, fe)
	}
	return out
}

// Register adds every tool to the server.
func (t *Tools) Register(s *mcp.Server) {
	t.registerDatasets(s)
	t.registerQuery(s)
	t.registerDiscovery(s)
}
