// Package cct is a thin, cache-aware wrapper around the go-arcgis client scoped
// to the City of Cape Town Open Data Feature Service. It centralises client
// construction, capped pagination, and response caching so the MCP tool layer
// can stay declarative.
package cct

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	capetown "github.com/richardwooding/capetown-opendata"
	arcgis "github.com/richardwooding/go-arcgis"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cache"
)

// BaseURL is the upstream Feature Service endpoint, re-exported for convenience.
const BaseURL = capetown.BaseURL

const (
	defaultMaxRetries   = 2
	defaultRetryBackoff = 300 * time.Millisecond
)

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
	// MaxRetries is the number of extra attempts for a transient failure
	// (timeout or HTTP 5xx). Zero uses a sensible default; negative disables
	// retries. The live City service has variable latency, so a couple of
	// retries smooths over transient hiccups.
	MaxRetries int
	// RetryBackoff is the base delay between retries (doubled each attempt).
	RetryBackoff time.Duration
	// HTTPClient overrides the default HTTP client (used by tests).
	HTTPClient *http.Client
	// BaseURL overrides the upstream endpoint (used by tests).
	BaseURL string
}

// Client wraps an *arcgis.Client with a TTL cache.
type Client struct {
	arc        *arcgis.Client
	cache      *cache.Cache
	maxRetries int
	backoff    time.Duration
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
	retries := opts.MaxRetries
	switch {
	case retries < 0:
		retries = 0
	case retries == 0:
		retries = defaultMaxRetries
	}
	backoff := opts.RetryBackoff
	if backoff <= 0 {
		backoff = defaultRetryBackoff
	}
	return &Client{
		arc:        arcgis.NewClient(base, aopts...),
		cache:      cache.New(opts.CacheTTL, opts.CacheCapacity),
		maxRetries: retries,
		backoff:    backoff,
	}
}

// retry runs fn, retrying transient failures up to c.maxRetries times with
// exponential backoff. It stops early if the context is done.
func (c *Client) retry(ctx context.Context, fn func() error) error {
	var err error
	delay := c.backoff
	for attempt := 0; ; attempt++ {
		if err = fn(); err == nil || ctx.Err() != nil {
			return err
		}
		if attempt >= c.maxRetries || !transient(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}
		delay *= 2
	}
}

// transient reports whether err is worth retrying: a network timeout or an
// HTTP 5xx from the upstream. Deterministic errors (4xx, bad field/where) are
// not retried because they won't succeed on a second attempt.
func transient(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 5") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "Client.Timeout") ||
		strings.Contains(s, "context deadline exceeded")
}

// Close releases background resources held by the client's cache.
func (c *Client) Close() { c.cache.Stop() }

// QueryLimit fetches up to limit features for p, paginating as needed. The
// boolean return reports whether more features were available beyond the limit.
//
// ArcGIS only guarantees deterministic pagination when an orderByFields is
// supplied. To make paging safe, the layer's object-ID field is appended as a
// stable tiebreaker when one is available, and features are de-duplicated by
// object ID across pages so an unstable upstream order can't yield duplicates.
func (c *Client) QueryLimit(ctx context.Context, p arcgis.QueryParams, limit int) ([]arcgis.Feature, bool, error) {
	oid := c.oidField(ctx, p.LayerID)
	if oid != "" && !containsField(p.OrderByFields, oid) {
		p.OrderByFields = append(append([]string{}, p.OrderByFields...), oid)
	}

	var out []arcgis.Feature
	seen := make(map[any]struct{})
	more := false
	for {
		fs, err := c.queryPage(ctx, p)
		if err != nil {
			return nil, false, err
		}
		added := 0
		for _, f := range fs.Features {
			if oid != "" {
				if id, ok := f.Attrs()[oid]; ok {
					if _, dup := seen[id]; dup {
						continue
					}
					seen[id] = struct{}{}
				}
			}
			out = append(out, f)
			added++
		}
		if len(out) >= limit {
			more = fs.ExceededTransferLimit || len(out) > limit
			break
		}
		// Stop on the last page, an empty page, or a page that contributed
		// nothing new (the latter guards against a non-advancing upstream).
		if !fs.ExceededTransferLimit || len(fs.Features) == 0 || added == 0 {
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

// oidField returns the layer's object-ID field name, or "" if it can't be
// determined. The result is derived from the cached layer schema.
func (c *Client) oidField(ctx context.Context, layerID int) string {
	info, err := c.LayerInfo(ctx, layerID)
	if err != nil {
		return ""
	}
	for _, f := range info.Fields {
		if f.Type == "esriFieldTypeOID" {
			return f.Name
		}
	}
	return ""
}

// containsField reports whether name appears as a column in an orderByFields
// list (ignoring any trailing ASC/DESC direction and case).
func containsField(fields []string, name string) bool {
	for _, f := range fields {
		col := f
		if i := strings.IndexByte(col, ' '); i >= 0 {
			col = col[:i]
		}
		if strings.EqualFold(col, name) {
			return true
		}
	}
	return false
}

// Count returns the number of features matching p.
func (c *Client) Count(ctx context.Context, p arcgis.QueryParams) (int, error) {
	return cache.Fetch(c.cache, cacheKey("count", p), func() (int, error) {
		var n int
		err := c.retry(ctx, func() error { var e error; n, e = c.arc.QueryCount(ctx, p); return e })
		return n, err
	})
}

// ServiceInfo returns metadata for the feature service.
func (c *Client) ServiceInfo(ctx context.Context) (*arcgis.ServiceInfo, error) {
	return cache.Fetch(c.cache, "service-info", func() (*arcgis.ServiceInfo, error) {
		var info *arcgis.ServiceInfo
		err := c.retry(ctx, func() error { var e error; info, e = c.arc.ServiceInfo(ctx); return e })
		return info, err
	})
}

// LayerInfo returns metadata for a single layer.
func (c *Client) LayerInfo(ctx context.Context, layerID int) (*arcgis.LayerInfo, error) {
	return cache.Fetch(c.cache, cacheKey("layer-info", arcgis.QueryParams{LayerID: layerID}), func() (*arcgis.LayerInfo, error) {
		var info *arcgis.LayerInfo
		err := c.retry(ctx, func() error { var e error; info, e = c.arc.LayerInfo(ctx, layerID); return e })
		return info, err
	})
}

// queryPage runs a single cached page query.
func (c *Client) queryPage(ctx context.Context, p arcgis.QueryParams) (*arcgis.FeatureSet, error) {
	return cache.Fetch(c.cache, cacheKey("page", p), func() (*arcgis.FeatureSet, error) {
		var fs *arcgis.FeatureSet
		err := c.retry(ctx, func() error { var e error; fs, e = c.arc.Query(ctx, p); return e })
		return fs, err
	})
}

// cacheKey derives a stable cache key from a prefix and query params.
func cacheKey(prefix string, p arcgis.QueryParams) string {
	b, _ := json.Marshal(p)
	return prefix + ":" + string(b)
}
