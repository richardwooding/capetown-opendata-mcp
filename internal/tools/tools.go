// Package tools defines the MCP tools exposed by the Cape Town Open Data server
// and registers them against an *mcp.Server. Each tool maps user-friendly input
// onto an ArcGIS query and returns structured output.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	arcgis "github.com/richardwooding/go-arcgis"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cct"
)

const (
	defaultLimit = 25
	maxLimit     = 2000
	// maxResponseChars keeps a result near 8,000 tokens, under the warning
	// threshold MCP clients such as Claude Code apply to tool output.
	maxResponseChars = 32_000
)

const trimmedNote = "Trimmed to fit the response size budget. Page with next_offset, select fewer fields, or use summarize_layer for counts."

// Tools holds the dependencies shared by all tool handlers.
type Tools struct {
	client *cct.Client
}

// New returns a Tools backed by the given client.
func New(client *cct.Client) *Tools { return &Tools{client: client} }

// CommonQuery holds filters shared by every feature-returning tool.
type CommonQuery struct {
	Limit           int           `json:"limit,omitempty" jsonschema:"max features (default 25, max 2000)"`
	Offset          int           `json:"offset,omitempty" jsonschema:"features to skip; pass next_offset to page"`
	Where           string        `json:"where,omitempty" jsonschema:"extra SQL filter; field names via layer_info"`
	Fields          []string      `json:"fields,omitempty" jsonschema:"columns to return; omit for all"`
	BBox            []float64     `json:"bbox,omitempty" jsonschema:"[minLon, minLat, maxLon, maxLat] in WGS84"`
	Polygon         [][][]float64 `json:"polygon,omitempty" jsonschema:"WGS84 rings [[[lon,lat],...]]; bbox wins if both set"`
	IncludeGeometry bool          `json:"include_geometry,omitempty" jsonschema:"include GeoJSON geometry (default false)"`
	OmitNulls       *bool         `json:"omit_nulls,omitempty" jsonschema:"drop null and empty values (default true)"`
	UseAliases      bool          `json:"use_aliases,omitempty" jsonschema:"use readable field aliases as keys"`
}

type rowOptions struct {
	geometry  bool
	omitNulls bool
	keepShape bool
}

func (c CommonQuery) rowOptions() rowOptions {
	return rowOptions{
		geometry:  c.IncludeGeometry,
		omitNulls: c.OmitNulls == nil || *c.OmitNulls,
		keepShape: c.IncludeGeometry || slices.ContainsFunc(c.Fields, isShapeColumn),
	}
}

// isShapeColumn matches the area and length columns ArcGIS derives from the
// geometry (Shape__Area, Shape.STArea(), ...), which are noise without it.
func isShapeColumn(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, "shape__") || strings.HasPrefix(n, "shape.st")
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
	ExceededLimit bool      `json:"exceeded_limit" jsonschema:"true if more features are available"`
	NextOffset    *int      `json:"next_offset,omitempty" jsonschema:"offset for the next page"`
	Note          string    `json:"note,omitempty" jsonschema:"set when the page was trimmed to fit the size budget"`
}

// run applies the common filters to a base query, executes it against the given
// split service, and shapes the result.
func (t *Tools) run(ctx context.Context, service string, base arcgis.QueryParams, c CommonQuery) (*mcp.CallToolResult, FeatureResult, error) {
	p := applyCommon(base, c)
	feats, more, err := t.client.QueryLimit(ctx, service, p, effectiveLimit(c.Limit))
	if err != nil {
		return nil, FeatureResult{}, annotateErr(err, service, base.LayerID)
	}
	res := toResult(feats, more, c.rowOptions())
	if c.UseAliases {
		t.applyAliases(ctx, service, base.LayerID, res.Features)
	}
	if n := fitCount(res.Features); n < len(res.Features) {
		res.Features, res.Count, res.ExceededLimit, res.Note = res.Features[:n], n, true, trimmedNote
	}
	if res.ExceededLimit {
		next := c.Offset + res.Count
		res.NextOffset = &next
	}
	return nil, res, nil
}

// fitCount returns how many leading items fit in maxResponseChars of JSON,
// always at least one so a single wide row still comes back.
func fitCount[T any](items []T) int {
	size := 0
	for i, it := range items {
		b, _ := json.Marshal(it)
		size += len(b) + 1
		if size > maxResponseChars && i > 0 {
			return i
		}
	}
	return len(items)
}

// applyAliases rewrites each feature's attribute keys from raw column names to
// their human-readable field aliases, looked up from the (cached) layer schema.
// Best-effort: if the schema can't be fetched, attributes are left unchanged.
func (t *Tools) applyAliases(ctx context.Context, service string, layerID int, feats []Feature) {
	info, err := t.client.LayerInfo(ctx, service, layerID)
	if err != nil {
		return
	}
	alias := make(map[string]string, len(info.Fields))
	for _, f := range info.Fields {
		if f.Alias != "" && f.Alias != f.Name {
			alias[f.Name] = f.Alias
		}
	}
	if len(alias) == 0 {
		return
	}
	for i := range feats {
		if feats[i].Attributes == nil {
			continue
		}
		renamed := make(map[string]any, len(feats[i].Attributes))
		for k, v := range feats[i].Attributes {
			if a, ok := alias[k]; ok {
				renamed[a] = v
			} else {
				renamed[k] = v
			}
		}
		feats[i].Attributes = renamed
	}
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
	} else if len(c.Polygon) > 0 {
		p.Polygon = &arcgis.Polygon{Rings: c.Polygon}
	}
	if len(c.Fields) > 0 {
		p.Fields = c.Fields
	}
	if c.Offset > 0 {
		p.ResultOffset = c.Offset
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
func toResult(feats []arcgis.Feature, more bool, opts rowOptions) FeatureResult {
	out := FeatureResult{Count: len(feats), ExceededLimit: more, Features: make([]Feature, 0, len(feats))}
	for _, f := range feats {
		fe := Feature{Attributes: leanAttrs(f.Attrs(), opts)}
		if opts.geometry && len(f.Geometry) > 0 {
			var g any
			if json.Unmarshal(f.Geometry, &g) == nil {
				fe.Geometry = roundCoords(g)
			}
		}
		out.Features = append(out.Features, fe)
	}
	return out
}

func leanAttrs(attrs map[string]any, opts rowOptions) map[string]any {
	out := make(map[string]any, len(attrs))
	for k, v := range attrs {
		if opts.omitNulls && (v == nil || v == "") {
			continue
		}
		if !opts.keepShape && isShapeColumn(k) {
			continue
		}
		out[k] = v
	}
	return out
}

// roundCoords trims GeoJSON coordinates to 6 decimal places (about 10 cm),
// which nearly halves polygon payloads without visible loss.
func roundCoords(v any) any {
	switch x := v.(type) {
	case float64:
		return math.Round(x*1e6) / 1e6
	case []any:
		for i := range x {
			x[i] = roundCoords(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = roundCoords(x[k])
		}
	}
	return v
}

// annotateErr wraps an upstream query error with guidance the caller can act on.
// The opaque ArcGIS messages ("Unable to complete operation") and bare timeouts
// give no hint at the cause, so we classify the common cases.
func annotateErr(err error, service string, layerID int) error {
	if err == nil {
		return nil
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "context deadline exceeded") || strings.Contains(s, "Client.Timeout"):
		return fmt.Errorf("%w — the upstream service timed out; retry with a smaller limit or a bbox filter", err)
	case strings.Contains(s, "HTTP 5") || strings.Contains(s, "arcgis error 5") || strings.Contains(s, "error 500"):
		// Don't wrap: a hard 5xx body may be a large HTML error page.
		return fmt.Errorf("the upstream Open Data service returned a server error; the %s service may be stopped or mid-restructure — run service_info to see which services are live", serviceLabel(service))
	case strings.Contains(s, "arcgis error 400") || strings.Contains(s, "Unable to complete operation") || strings.Contains(s, "Invalid Layer"):
		return fmt.Errorf("%w — the service rejected the query; a field name in where/fields/order_by may be invalid, or the layer/service may have drifted. Call layer_info(service=%q, layer_id=%d) to list valid fields, or service_info to verify the layer", err, service, layerID)
	}
	return err
}

// serviceLabel renders a service name for an error message, tolerating a blank
// (generic) service.
func serviceLabel(service string) string {
	if service == "" {
		return "requested"
	}
	return service
}

// validateService rejects a missing or unknown split-service name with a hint
// pointing at service_info, before an opaque upstream 404 can occur.
func validateService(service string) error {
	if strings.TrimSpace(service) == "" {
		return fmt.Errorf("service is required; call service_info to find which service hosts the layer you want")
	}
	if !cct.KnownService(service) {
		return fmt.Errorf("unknown service %q; valid services are ODP_SPLIT_1 … ODP_SPLIT_13, SERVICE_REQUESTS and BUILDING_PLANS — call service_info to list layers and their services", service)
	}
	return nil
}

// Register adds every tool to the server.
func (t *Tools) Register(s *mcp.Server) {
	t.registerDatasets(s)
	t.registerQuery(s)
	t.registerDiscovery(s)
}
