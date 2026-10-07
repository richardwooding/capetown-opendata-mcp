package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cct"
)

func layerInfoServer(t *testing.T, name string) *Tools {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/query") {
			// Local midnight (UTC+2) on 2023-01-01 and 2026-10-06, as the City stores them.
			fmt.Fprint(w, `{"features":[{"attributes":{"earliest":1672524000000,"latest":1791237600000}}]}`)
			return
		}
		fmt.Fprintf(w, `{"id":0,"name":%q,"type":"Table","fields":[{"name":"ObjectId","type":"esriFieldTypeOID"},{"name":"Created_On_Date","type":"esriFieldTypeDate"}]}`, name)
	}))
	t.Cleanup(srv.Close)
	c := cct.New(cct.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})
	t.Cleanup(c.Close)
	return New(c)
}

func TestLayerInfoHubTableLabelAndDataRange(t *testing.T) {
	tl := layerInfoServer(t, "Service_Requests_2023_until_20_May_2026")
	_, res, err := tl.layerInfo(context.Background(), nil, LayerInfoInput{Service: "SERVICE_REQUESTS", LayerID: 0})
	if err != nil {
		t.Fatalf("layerInfo: %v", err)
	}
	if res.Name != "Service Requests (2023 onwards)" {
		t.Errorf("name = %q, want the readable label, not the stale table name", res.Name)
	}
	if res.DataRange == nil || res.DataRange.Field != "Created_On_Date" || res.DataRange.Earliest != "2023-01-01" || res.DataRange.Latest != "2026-10-06" {
		t.Errorf("data_range = %+v, want Created_On_Date 2023-01-01..2026-10-06", res.DataRange)
	}
}

func TestLayerInfoODPLayerHasNoDataRange(t *testing.T) {
	tl := layerInfoServer(t, "Ward")
	_, res, err := tl.layerInfo(context.Background(), nil, LayerInfoInput{Service: "ODP_SPLIT_5", LayerID: 6})
	if err != nil {
		t.Fatalf("layerInfo: %v", err)
	}
	if res.Name != "Ward" || res.DataRange != nil {
		t.Errorf("got name=%q data_range=%+v, want Ward with no data_range", res.Name, res.DataRange)
	}
}
