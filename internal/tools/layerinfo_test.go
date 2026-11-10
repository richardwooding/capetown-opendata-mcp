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
	srv := layerInfoData(t, name)
	c := cct.New(cct.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})
	t.Cleanup(c.Close)
	return New(c)
}

func layerInfoData(t *testing.T, name string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/query") {
			// Local midnight (UTC+2) on 2023-01-01 and 2026-10-06, as the City stores them.
			fmt.Fprint(w, `{"features":[{"attributes":{"earliest":1672524000000,"latest":1791237600000}}]}`)
			return
		}
		fmt.Fprintf(w, `{"id":0,"name":%q,"type":"Table","fields":[{"name":"ObjectId","type":"esriFieldTypeOID"},{"name":"CreatedOnDT","type":"esriFieldTypeDate"}]}`, name)
	}))
	t.Cleanup(srv.Close)
	return srv
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
	if res.DataRange == nil || res.DataRange.Field != "CreatedOnDT" || res.DataRange.Earliest != "2023-01-01" || res.DataRange.Latest != "2026-10-06" {
		t.Errorf("data_range = %+v, want CreatedOnDT 2023-01-01..2026-10-06", res.DataRange)
	}
	if res.Source != nil {
		t.Errorf("source = %+v, want none without a portal", res.Source)
	}
}

func TestLayerInfoTaxiFaresDataRangeAndPopulationNone(t *testing.T) {
	tl := layerInfoServer(t, "MBT(AD)_Survey 2023 24 25")
	_, res, err := tl.layerInfo(context.Background(), nil, LayerInfoInput{Service: "TAXI_FARES", LayerID: 0})
	if err != nil {
		t.Fatalf("layerInfo: %v", err)
	}
	if res.DataRange == nil || res.DataRange.Field != "Survey_Date" {
		t.Errorf("taxi fares data_range = %+v, want Survey_Date", res.DataRange)
	}
	_, res, err = tl.layerInfo(context.Background(), nil, LayerInfoInput{Service: "POPULATION_ESTIMATES", LayerID: 0})
	if err != nil {
		t.Fatalf("layerInfo: %v", err)
	}
	if res.DataRange != nil || res.Name != "Population Estimates (Suburb Level)" {
		t.Errorf("population: name=%q data_range=%+v, want the fallback label and no data_range", res.Name, res.DataRange)
	}
}

func TestLayerInfoSourceFromHubItem(t *testing.T) {
	data := layerInfoData(t, "Estimates")
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/sharing/rest/content/items/")
		fmt.Fprintf(w, `{"id": %q, "title": "Population Estimates 2025 (Suburb Level)", "snippet": " <p>This dataset provides&nbsp;modelled estimates.</p>\n", "modified": 1794298382000, "url": %q}`, id, data.URL+"/Pop/FeatureServer")
	}))
	t.Cleanup(portal.Close)
	c := cct.New(cct.Options{PortalURL: portal.URL, HTTPClient: http.DefaultClient})
	t.Cleanup(c.Close)

	_, res, err := New(c).layerInfo(context.Background(), nil, LayerInfoInput{Service: "POPULATION_ESTIMATES", LayerID: 0})
	if err != nil {
		t.Fatalf("layerInfo: %v", err)
	}
	if res.Name != "Population Estimates 2025 (Suburb Level)" {
		t.Errorf("name = %q, want the hub item title", res.Name)
	}
	want := SourceInfo{ItemID: "61bd1a6c374d44a093862784b79f6c23", Title: "Population Estimates 2025 (Suburb Level)", Summary: "This dataset provides modelled estimates.", Modified: "2026-11-10"}
	if res.Source == nil || *res.Source != want {
		t.Errorf("source = %+v, want %+v", res.Source, want)
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
