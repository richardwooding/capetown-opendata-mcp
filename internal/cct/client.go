// Package cct is a thin, cache-aware wrapper around the go-arcgis client scoped
// to the City of Cape Town Open Data Feature Service. It centralises client
// construction, capped pagination, and response caching so the MCP tool layer
// can stay declarative.
package cct

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	capetown "github.com/richardwooding/capetown-opendata"
	arcgis "github.com/richardwooding/go-arcgis"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cache"
)

// BaseURL is the upstream Feature Service endpoint, re-exported for convenience.
const BaseURL = capetown.BaseURL

// Options configures a Client.
type Options struct {
	// Timeout bounds each HTTP request. Ignored when HTTPClient is set.
	Timeout time.Duration
	// Token is an optional ArcGIS token for authenticated services.
	Token string
	// CacheTTL is the lifetime of cached responses. Zero disables caching.
	CacheTTL time.Duration
	// CacheCapacity bounds the number of cached entries (0 = unbounded).
	CacheCapacity uint64
	// HTTPClient overrides the default HTTP client (used by tests).
	HTTPClient *http.Client
	// BaseURL overrides the upstream endpoint (used by tests).
	BaseURL string
}

// Client wraps an *arcgis.Client with a TTL cache.
type Client struct {
	arc   *arcgis.Client
	cache *cache.Cache
}

// New constructs a Client from Options.
func New(opts Options) *Client {
	var aopts []arcgis.ClientOption
	switch {
	case opts.HTTPClient != nil:
		aopts = append(aopts, arcgis.WithHTTPClient(opts.HTTPClient))
	case opts.Timeout > 0:
		aopts = append(aopts, arcgis.WithTimeout(opts.Timeout))
	}
	if opts.Token != "" {
		aopts = append(aopts, arcgis.WithToken(opts.Token))
	}
	base := opts.BaseURL
	if base == "" {
		base = capetown.BaseURL
	}
	return &Client{
		arc:   arcgis.NewClient(base, aopts...),
		cache: cache.New(opts.CacheTTL, opts.CacheCapacity),
	}
}

// Close releases background resources held by the client's cache.
func (c *Client) Close() { c.cache.Stop() }

// QueryLimit fetches up to limit features for p, paginating as needed. The
// boolean return reports whether more features were available beyond the limit.
func (c *Client) QueryLimit(ctx context.Context, p arcgis.QueryParams, limit int) ([]arcgis.Feature, bool, error) {
	var out []arcgis.Feature
	more := false
	for {
		fs, err := c.queryPage(ctx, p)
		if err != nil {
			return nil, false, err
		}
		out = append(out, fs.Features...)
		if len(out) >= limit {
			more = fs.ExceededTransferLimit || len(out) > limit
			break
		}
		if !fs.ExceededTransferLimit || len(fs.Features) == 0 {
			break
		}
		p.ResultOffset += len(fs.Features)
	}
	if len(out) > limit {
		out = out[:limit]
		more = true
	}
	return out, more, nil
}

// Count returns the number of features matching p.
func (c *Client) Count(ctx context.Context, p arcgis.QueryParams) (int, error) {
	return cache.Fetch(c.cache, cacheKey("count", p), func() (int, error) {
		return c.arc.QueryCount(ctx, p)
	})
}

// ServiceInfo returns metadata for the feature service.
func (c *Client) ServiceInfo(ctx context.Context) (*arcgis.ServiceInfo, error) {
	return cache.Fetch(c.cache, "service-info", func() (*arcgis.ServiceInfo, error) {
		return c.arc.ServiceInfo(ctx)
	})
}

// LayerInfo returns metadata for a single layer.
func (c *Client) LayerInfo(ctx context.Context, layerID int) (*arcgis.LayerInfo, error) {
	return cache.Fetch(c.cache, cacheKey("layer-info", arcgis.QueryParams{LayerID: layerID}), func() (*arcgis.LayerInfo, error) {
		return c.arc.LayerInfo(ctx, layerID)
	})
}

// queryPage runs a single cached page query.
func (c *Client) queryPage(ctx context.Context, p arcgis.QueryParams) (*arcgis.FeatureSet, error) {
	return cache.Fetch(c.cache, cacheKey("page", p), func() (*arcgis.FeatureSet, error) {
		return c.arc.Query(ctx, p)
	})
}

// cacheKey derives a stable cache key from a prefix and query params.
func cacheKey(prefix string, p arcgis.QueryParams) string {
	b, _ := json.Marshal(p)
	return prefix + ":" + string(b)
}
