package cct

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	capetown "github.com/richardwooding/capetown-opendata"
	arcgis "github.com/richardwooding/go-arcgis"
)

func TestResolveServer(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", capetown.FolderESAPQA},
		{"esapqa", capetown.FolderESAPQA},
		{"  CityMaps ", capetown.FolderCityMaps},
		{"https://gis.example.org/arcgis/rest/services/Open/", "https://gis.example.org/arcgis/rest/services/Open"},
	}
	for _, tc := range cases {
		got, err := ResolveServer(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ResolveServer(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"bogus", "http://insecure.example.org/rest/services/X", "https://example.org/not-arcgis", "https://"} {
		if _, err := ResolveServer(bad); err == nil {
			t.Errorf("ResolveServer(%q) succeeded, want error", bad)
		}
	}
}

func TestServerFolderRoutesODPServices(t *testing.T) {
	var path atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path.Store(r.URL.Path)
		fmt.Fprint(w, `{"count": 3}`)
	}))
	t.Cleanup(srv.Close)

	c := New(Options{ServerFolder: srv.URL + "/agsext/rest/services/Theme_Based", HTTPClient: srv.Client()})
	t.Cleanup(c.Close)
	if _, err := c.Count(context.Background(), "ODP_SPLIT_13", arcgis.QueryParams{LayerID: 2}); err != nil {
		t.Fatalf("Count: %v", err)
	}
	if got, want := path.Load(), "/agsext/rest/services/Theme_Based/ODP_SPLIT_13/FeatureServer/2/query"; got != want {
		t.Errorf("path = %v, want %s", got, want)
	}
}

func TestKnownServiceIncludesSplit13(t *testing.T) {
	if !KnownService("ODP_SPLIT_13") {
		t.Error("ODP_SPLIT_13 should be a known service")
	}
}
