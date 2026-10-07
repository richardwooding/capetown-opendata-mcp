// Package cct is a thin, cache-aware wrapper around the go-arcgis client scoped
// to the City of Cape Town Open Data feature services. It centralises client
// construction, capped pagination, and response caching so the MCP tool layer
// can stay declarative.
//
// The City spreads its Open Data layers across several themed split services
// (ODP_SPLIT_*, see the capetown package), so every method takes the split
// service name to address; the wrapper keeps one underlying *arcgis.Client per
// service, built lazily.
package cct

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	capetown "github.com/richardwooding/capetown-opendata"
	arcgis "github.com/richardwooding/go-arcgis"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cache"
)

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
	// BaseURL overrides the upstream endpoint for EVERY service (used by
	// tests to point all split services at a single test server). When empty,
	// each service resolves under ServerFolder.
	BaseURL string
	// PortalURL is where hub item IDs are resolved; empty means ArcGIS Online.
	PortalURL string
	// ServerFolder is the ArcGIS REST folder hosting the ODP_SPLIT services;
	// empty means capetown.BaseFolder. See ResolveServer.
	ServerFolder string
}

// Client wraps per-service *arcgis.Client instances behind a shared TTL cache.
type Client struct {
	newArc  func(service string) *arcgis.Client
	mu      sync.Mutex
	clients map[string]*arcgis.Client

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
	if t := usableToken(opts.Token); t != "" {
		aopts = append(aopts, arcgis.WithToken(t))
	}
	override := opts.BaseURL
	folder := opts.ServerFolder
	if folder == "" {
		folder = capetown.BaseFolder
	}
	resolve := hubResolver(opts)
	baseFor := func(service string) string {
		if override != "" {
			return override
		}
		if u, ok := resolve(service); ok {
			return u
		}
		return capetown.ServiceURLIn(folder, service)
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
		newArc:     func(service string) *arcgis.Client { return arcgis.NewClient(baseFor(service), aopts...) },
		clients:    make(map[string]*arcgis.Client),
		cache:      cache.New(opts.CacheTTL, opts.CacheCapacity),
		maxRetries: retries,
		backoff:    backoff,
	}
}

// hubResolver looks up the current URL of an ArcGIS Online dataset from its
// hub item ID. The City republishes these under new date-stamped names, so the
// item ID outlives any URL. A failed lookup falls back to the known URL rather
// than blocking queries. The token is deliberately not forwarded: it belongs
// to the City's services, not to the public portal.
func hubResolver(opts Options) func(service string) (string, bool) {
	portal := opts.PortalURL
	if portal == "" {
		portal = arcgis.ArcGISOnline
	}
	var popts []arcgis.ClientOption
	switch {
	case opts.HTTPClient != nil:
		popts = append(popts, arcgis.WithHTTPClient(opts.HTTPClient))
	case opts.Timeout > 0:
		popts = append(popts, arcgis.WithTimeout(opts.Timeout))
	}
	return func(service string) (string, bool) {
		id, ok := capetown.HubItemID(service)
		if !ok {
			return "", false
		}
		ctx, cancel := context.WithTimeout(context.Background(), hubLookupTimeout)
		defer cancel()
		u, err := arcgis.ItemURL(ctx, portal, id, popts...)
		if err != nil {
			return "", false
		}
		return u, true
	}
}

const hubLookupTimeout = 10 * time.Second

// arc returns the (lazily built, cached) underlying client for a split service.
func (c *Client) arc(service string) *arcgis.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cl, ok := c.clients[service]; ok {
		return cl
	}
	cl := c.newArc(service)
	c.clients[service] = cl
	return cl
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

// usableToken returns the ArcGIS token to use, or "" if it should be ignored.
// A blank, whitespace-only, or unsubstituted "${...}" placeholder (e.g. an MCP
// Bundle user_config value left empty) counts as no token.
func usableToken(tok string) string {
	t := strings.TrimSpace(tok)
	if strings.HasPrefix(t, "${") && strings.HasSuffix(t, "}") {
		return ""
	}
	return t
}

// Close releases background resources held by the client's cache.
func (c *Client) Close() { c.cache.Stop() }

// QueryLimit fetches up to limit features for p on the given split service,
// paginating as needed. The boolean return reports whether more features were
// available beyond the limit.
//
// ArcGIS only guarantees deterministic pagination when an orderByFields is
// supplied. To make paging safe, the layer's object-ID field is appended as a
// stable tiebreaker when one is available, and features are de-duplicated by
// object ID across pages so an unstable upstream order can't yield duplicates.
func (c *Client) QueryLimit(ctx context.Context, service string, p arcgis.QueryParams, limit int) ([]arcgis.Feature, bool, error) {
	oid := ""
	// A distinct query returns only the selected fields, so there is no object
	// ID to dedupe on, and esapqa rejects ordering by a field not returned.
	if !p.ReturnDistinctValues {
		oid = c.oidField(ctx, service, p.LayerID)
	}
	if oid != "" && !containsField(p.OrderByFields, oid) {
		p.OrderByFields = append(append([]string{}, p.OrderByFields...), oid)
	}

	var out []arcgis.Feature
	seen := make(map[any]struct{})
	more := false
	for {
		fs, err := c.queryPage(ctx, service, p)
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
func (c *Client) oidField(ctx context.Context, service string, layerID int) string {
	info, err := c.LayerInfo(ctx, service, layerID)
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

// Count returns the number of features matching p on the given split service.
func (c *Client) Count(ctx context.Context, service string, p arcgis.QueryParams) (int, error) {
	return cache.Fetch(c.cache, cacheKey(service, "count", p), func() (int, error) {
		var n int
		err := c.retry(ctx, func() error { var e error; n, e = c.arc(service).QueryCount(ctx, p); return e })
		return n, err
	})
}

// ServiceInfo returns metadata for a single split feature service.
func (c *Client) ServiceInfo(ctx context.Context, service string) (*arcgis.ServiceInfo, error) {
	return cache.Fetch(c.cache, "service-info:"+service, func() (*arcgis.ServiceInfo, error) {
		var info *arcgis.ServiceInfo
		err := c.retry(ctx, func() error { var e error; info, e = c.arc(service).ServiceInfo(ctx); return e })
		return info, err
	})
}

// LayerInfo returns metadata for a single layer within a split service.
func (c *Client) LayerInfo(ctx context.Context, service string, layerID int) (*arcgis.LayerInfo, error) {
	return cache.Fetch(c.cache, cacheKey(service, "layer-info", arcgis.QueryParams{LayerID: layerID}), func() (*arcgis.LayerInfo, error) {
		var info *arcgis.LayerInfo
		err := c.retry(ctx, func() error { var e error; info, e = c.arc(service).LayerInfo(ctx, layerID); return e })
		return info, err
	})
}

// ServiceLayer identifies one layer or table within a split service.
type ServiceLayer struct {
	Service string `json:"service" jsonschema:"the service that hosts this layer (ODP_SPLIT_*, SERVICE_REQUESTS or BUILDING_PLANS); pass it to layer_info/query_layer"`
	ID      int    `json:"id" jsonschema:"the layer (or table) ID within its service"`
	Name    string `json:"name" jsonschema:"the layer's human-readable name"`
	IsTable bool   `json:"is_table,omitempty" jsonschema:"true when this is a non-spatial table rather than a feature layer"`
}

// UnavailableService records a split service that could not be listed (e.g. it
// is stopped or mid-restructure upstream).
type UnavailableService struct {
	Service string `json:"service" jsonschema:"the service that could not be listed"`
	Error   string `json:"error" jsonschema:"a short reason it was unavailable"`
}

// AggregatedServiceInfo is the merged layer listing across every split service.
type AggregatedServiceInfo struct {
	Layers      []ServiceLayer
	Unavailable []UnavailableService
}

// ServiceInfoAll lists every layer and table across all split services,
// tagging each with the service that hosts it. Services that cannot be listed
// are recorded in Unavailable rather than failing the whole call, so a single
// stopped split does not blind the caller to the rest of the portal.
func (c *Client) ServiceInfoAll(ctx context.Context) AggregatedServiceInfo {
	services := append(capetown.Services(), capetown.HubServices()...)
	type slot struct {
		layers  []ServiceLayer
		unavail *UnavailableService
	}
	slots := make([]slot, len(services))
	var wg sync.WaitGroup
	for i, svc := range services {
		wg.Add(1)
		go func(i int, svc string) {
			defer wg.Done()
			info, err := c.ServiceInfo(ctx, svc)
			if err != nil {
				slots[i].unavail = &UnavailableService{Service: svc, Error: cleanErrMsg(err)}
				return
			}
			for _, l := range info.Layers {
				slots[i].layers = append(slots[i].layers, ServiceLayer{Service: svc, ID: l.ID, Name: hubLabel(svc, l.Name)})
			}
			for _, tb := range info.Tables {
				slots[i].layers = append(slots[i].layers, ServiceLayer{Service: svc, ID: tb.ID, Name: hubLabel(svc, tb.Name), IsTable: true})
			}
		}(i, svc)
	}
	wg.Wait()

	var out AggregatedServiceInfo
	for _, s := range slots {
		out.Layers = append(out.Layers, s.layers...)
		if s.unavail != nil {
			out.Unavailable = append(out.Unavailable, *s.unavail)
		}
	}
	return out
}

// KnownService reports whether name is a split service or an ArcGIS Online dataset.
func KnownService(name string) bool {
	return slices.Contains(capetown.Services(), name) || slices.Contains(capetown.HubServices(), name)
}

// The upstream table names embed the date of a past refresh, which misleads.
var hubLabels = map[string]string{
	capetown.ServiceServiceRequests: "Service Requests (2023 onwards)",
	capetown.ServiceBuildingPlans:   "Building Plan Approvals (2014 onwards)",
}

// LayerLabel returns a readable name for a layer, replacing the stale
// upstream names of the ArcGIS Online tables.
func LayerLabel(service, upstream string) string { return hubLabel(service, upstream) }

func hubLabel(service, upstream string) string {
	if l, ok := hubLabels[service]; ok {
		return l
	}
	return upstream
}

// Statistics runs one aggregate query; it bypasses QueryLimit because ArcGIS
// rejects the object-ID tiebreaker that QueryLimit appends to the ordering.
func (c *Client) Statistics(ctx context.Context, service string, p arcgis.QueryParams) (*arcgis.FeatureSet, error) {
	return c.queryPage(ctx, service, p)
}

// OIDField returns the layer's object-ID field name, or "" if unknown.
func (c *Client) OIDField(ctx context.Context, service string, layerID int) string {
	return c.oidField(ctx, service, layerID)
}

// cleanErrMsg collapses a raw upstream error (which may embed a large HTML body
// on a hard 5xx) to a short, human-readable summary.
func cleanErrMsg(err error) string {
	s := err.Error()
	if strings.Contains(s, "HTTP 5") || strings.Contains(s, "error 500") {
		return "upstream server error (HTTP 5xx); the service may be stopped or mid-restructure"
	}
	if strings.Contains(s, "<html") || strings.Contains(s, "<HTML") {
		return "upstream returned an error page (service may be unavailable)"
	}
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// queryPage runs a single cached page query against a split service.
func (c *Client) queryPage(ctx context.Context, service string, p arcgis.QueryParams) (*arcgis.FeatureSet, error) {
	return cache.Fetch(c.cache, cacheKey(service, "page", p), func() (*arcgis.FeatureSet, error) {
		var fs *arcgis.FeatureSet
		err := c.retry(ctx, func() error { var e error; fs, e = c.arc(service).Query(ctx, p); return e })
		return fs, err
	})
}

// cacheKey derives a stable cache key from a service, a prefix, and query params.
func cacheKey(service, prefix string, p arcgis.QueryParams) string {
	b, _ := json.Marshal(p)
	return prefix + ":" + service + ":" + string(b)
}
