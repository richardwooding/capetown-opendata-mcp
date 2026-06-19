package cct

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	arcgis "github.com/richardwooding/go-arcgis"
)

// newTestClient builds a Client pointed at a test server, with caching off by
// default unless ttl > 0.
func newTestClient(t *testing.T, h http.Handler, ttl time.Duration) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(Options{BaseURL: srv.URL, HTTPClient: srv.Client(), CacheTTL: ttl})
	t.Cleanup(c.Close)
	return c
}

func TestQueryLimitCapsResults(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Always report more available; return 3 features per page.
		fmt.Fprint(w, `{"features":[
			{"properties":{"id":1}},{"properties":{"id":2}},{"properties":{"id":3}}
		],"exceededTransferLimit":true}`)
	})
	c := newTestClient(t, h, 0)

	feats, more, err := c.QueryLimit(context.Background(), arcgis.QueryParams{LayerID: 7}, 2)
	if err != nil {
		t.Fatalf("QueryLimit: %v", err)
	}
	if len(feats) != 2 {
		t.Fatalf("want 2 features (capped), got %d", len(feats))
	}
	if !more {
		t.Fatal("want more=true when results exceed the limit")
	}
}

func TestQueryLimitPaginates(t *testing.T) {
	var calls int32
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Query().Get("resultOffset") == "0" {
			fmt.Fprint(w, `{"features":[{"properties":{"id":1}},{"properties":{"id":2}}],"exceededTransferLimit":true}`)
			return
		}
		fmt.Fprint(w, `{"features":[{"properties":{"id":3}}],"exceededTransferLimit":false}`)
	})
	c := newTestClient(t, h, 0)

	feats, more, err := c.QueryLimit(context.Background(), arcgis.QueryParams{LayerID: 7}, 100)
	if err != nil {
		t.Fatalf("QueryLimit: %v", err)
	}
	if len(feats) != 3 {
		t.Fatalf("want 3 features across pages, got %d", len(feats))
	}
	if more {
		t.Fatal("want more=false once the final page is reached")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("want 2 upstream calls, got %d", got)
	}
}

func TestCacheAvoidsSecondCall(t *testing.T) {
	var calls int32
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		fmt.Fprint(w, `{"features":[{"properties":{"id":1}}],"exceededTransferLimit":false}`)
	})
	c := newTestClient(t, h, time.Minute)

	for i := 0; i < 3; i++ {
		if _, _, err := c.QueryLimit(context.Background(), arcgis.QueryParams{LayerID: 7}, 10); err != nil {
			t.Fatalf("QueryLimit: %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("want 1 upstream call with caching, got %d", got)
	}
}

func TestCountAndReturnGeometryParams(t *testing.T) {
	var gotCount, gotNoGeom bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("returnCountOnly") == "true" {
			gotCount = true
			fmt.Fprint(w, `{"count":42}`)
			return
		}
		if q.Get("returnGeometry") == "false" {
			gotNoGeom = true
		}
		fmt.Fprint(w, `{"features":[],"exceededTransferLimit":false}`)
	})
	c := newTestClient(t, h, 0)

	n, err := c.Count(context.Background(), arcgis.QueryParams{LayerID: 7})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 42 {
		t.Fatalf("want count 42, got %d", n)
	}
	if !gotCount {
		t.Fatal("expected returnCountOnly=true to be sent")
	}

	no := false
	if _, _, err := c.QueryLimit(context.Background(), arcgis.QueryParams{LayerID: 7, ReturnGeometry: &no}, 10); err != nil {
		t.Fatalf("QueryLimit: %v", err)
	}
	if !gotNoGeom {
		t.Fatal("expected returnGeometry=false to be sent")
	}
}

func TestAPIErrorSurfaced(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// ArcGIS reports failures with HTTP 200 and an error envelope.
		fmt.Fprint(w, `{"error":{"code":400,"message":"Invalid layer","details":["bad id"]}}`)
	})
	c := newTestClient(t, h, 0)

	_, _, err := c.QueryLimit(context.Background(), arcgis.QueryParams{LayerID: 999}, 10)
	if err == nil {
		t.Fatal("want error from ArcGIS error envelope")
	}
	if !strings.Contains(err.Error(), "Invalid layer") {
		t.Fatalf("error should surface upstream message, got %v", err)
	}
}

func TestRetryRecoversFromTransient(t *testing.T) {
	var hits atomic.Int32
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			http.Error(w, "upstream busy", http.StatusServiceUnavailable) // 503 -> transient
			return
		}
		fmt.Fprint(w, `{"features":[{"properties":{"id":1}}],"exceededTransferLimit":false}`)
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(Options{BaseURL: srv.URL, HTTPClient: srv.Client(), RetryBackoff: time.Millisecond, MaxRetries: 3})
	t.Cleanup(c.Close)

	feats, _, err := c.QueryLimit(context.Background(), arcgis.QueryParams{LayerID: 1}, 10)
	if err != nil {
		t.Fatalf("QueryLimit after retry: %v", err)
	}
	if len(feats) != 1 {
		t.Fatalf("want 1 feature after retry, got %d", len(feats))
	}
	if got := hits.Load(); got < 2 {
		t.Fatalf("expected a retry (>=2 hits), got %d", got)
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	var hits atomic.Int32
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		http.Error(w, "bad field", http.StatusBadRequest) // 400 -> deterministic, no retry
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(Options{BaseURL: srv.URL, HTTPClient: srv.Client(), RetryBackoff: time.Millisecond, MaxRetries: 3})
	t.Cleanup(c.Close)

	if _, _, err := c.QueryLimit(context.Background(), arcgis.QueryParams{LayerID: 1}, 10); err == nil {
		t.Fatal("expected an error for HTTP 400")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("400 should not be retried, got %d hits", got)
	}
}
