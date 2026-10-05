package tools

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestSummarizeLayerDefaultsToCountDescending(t *testing.T) {
	var query string
	body := `{"fields":[{"name":"ObjectId","type":"esriFieldTypeOID"}],"features":[
		{"attributes":{"Ward":"062","count":965}},
		{"attributes":{"Ward":"001","count":12}}
	]}`
	tools := New(capturingServerBody(t, &query, body))

	_, res, err := tools.summarizeLayer(context.Background(), nil, SummarizeLayerInput{
		Service: "SERVICE_REQUESTS", LayerID: 0, GroupBy: []string{"Ward"},
	})
	if err != nil {
		t.Fatalf("summarizeLayer: %v", err)
	}
	q, _ := url.ParseQuery(query)
	var stats []map[string]string
	if err := json.Unmarshal([]byte(q.Get("outStatistics")), &stats); err != nil {
		t.Fatalf("outStatistics: %v (%q)", err, q.Get("outStatistics"))
	}
	if len(stats) != 1 || stats[0]["statisticType"] != "count" || stats[0]["onStatisticField"] != "ObjectId" || stats[0]["outStatisticFieldName"] != "count" {
		t.Errorf("outStatistics = %v, want a count of ObjectId named count", stats)
	}
	if q.Get("groupByFieldsForStatistics") != "Ward" {
		t.Errorf("groupBy = %q", q.Get("groupByFieldsForStatistics"))
	}
	if q.Get("orderByFields") != "count DESC" {
		t.Errorf("orderByFields = %q, want count DESC with no object-ID tiebreaker", q.Get("orderByFields"))
	}
	if res.Count != 2 || res.Groups[0]["count"] != float64(965) {
		t.Errorf("groups = %v", res.Groups)
	}
}

func TestSummarizeLayerNamedStatistics(t *testing.T) {
	var query string
	tools := New(capturingServerBody(t, &query, `{"features":[{"attributes":{"sum_Building_Work_Value":1}}]}`))

	_, _, err := tools.summarizeLayer(context.Background(), nil, SummarizeLayerInput{
		Service: "BUILDING_PLANS", LayerID: 0,
		Statistics: []StatisticInput{{Type: "SUM", Field: "Building_Work_Value"}, {Type: "avg", Field: "Number_of_Units"}},
		OrderBy:    []string{"Financial_Year"},
	})
	if err != nil {
		t.Fatalf("summarizeLayer: %v", err)
	}
	q, _ := url.ParseQuery(query)
	for _, want := range []string{`"sum_Building_Work_Value"`, `"avg_Number_of_Units"`} {
		if !strings.Contains(q.Get("outStatistics"), want) {
			t.Errorf("outStatistics %q missing %s", q.Get("outStatistics"), want)
		}
	}
	if q.Get("orderByFields") != "Financial_Year" {
		t.Errorf("orderByFields = %q, want the caller's ordering", q.Get("orderByFields"))
	}
}

func TestSummarizeLayerRejectsBadInput(t *testing.T) {
	var query string
	tools := New(capturingServerBody(t, &query, `{"features":[]}`))
	ctx := context.Background()

	cases := map[string]SummarizeLayerInput{
		"unknown type":      {Service: "ODP_SPLIT_4", Statistics: []StatisticInput{{Type: "median", Field: "X"}}},
		"sum without field": {Service: "ODP_SPLIT_4", Statistics: []StatisticInput{{Type: "sum"}}},
		"unknown service":   {Service: "NOPE"},
	}
	for name, in := range cases {
		if _, _, err := tools.summarizeLayer(ctx, nil, in); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestHubTableToolsSendNoGeometry(t *testing.T) {
	var query string
	tools := New(capturingServerBody(t, &query, `{"features":[{"properties":{"Ward":"062"}}]}`))

	_, res, err := tools.serviceRequests(context.Background(), nil, ServiceRequestsInput{TableQuery{Where: "Ward = '062'", Limit: 5}})
	if err != nil {
		t.Fatalf("serviceRequests: %v", err)
	}
	q, _ := url.ParseQuery(query)
	if q.Get("returnGeometry") != "false" || q.Has("geometry") {
		t.Errorf("query = %q, want no geometry", query)
	}
	if q.Get("orderByFields") != "ObjectId DESC" {
		t.Errorf("orderByFields = %q, want ObjectId DESC", q.Get("orderByFields"))
	}
	if res.Count != 1 {
		t.Errorf("count = %d", res.Count)
	}

	if _, _, err := tools.buildingPlans(context.Background(), nil, BuildingPlansInput{TableQuery{Where: "Ward_No = 62"}}); err != nil {
		t.Fatalf("buildingPlans: %v", err)
	}
	q, _ = url.ParseQuery(query)
	if !strings.HasPrefix(q.Get("orderByFields"), "Submission_Date DESC") {
		t.Errorf("orderByFields = %q, want Submission_Date DESC first", q.Get("orderByFields"))
	}
}

func TestValidateServiceAcceptsHubServices(t *testing.T) {
	for _, s := range []string{"SERVICE_REQUESTS", "BUILDING_PLANS", "ODP_SPLIT_12"} {
		if err := validateService(s); err != nil {
			t.Errorf("validateService(%s): %v", s, err)
		}
	}
	if err := validateService("ODP_SPLIT_13"); err == nil || !strings.Contains(err.Error(), "SERVICE_REQUESTS") {
		t.Errorf("want unknown-service error listing the hub services, got %v", err)
	}
}
