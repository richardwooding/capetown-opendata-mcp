package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/richardwooding/capetown-opendata-mcp/internal/cct"
)

// pagedServer serves a layer whose object IDs are 1..total, honouring an
// "OID > n" / "OID < n" where clause and resultOffset, and records queries.
type pagedServer struct {
	mu      sync.Mutex
	queries []url.Values
	oid     string
	total   int
}

func (s *pagedServer) handler(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/query") {
		fmt.Fprintf(w, `{"fields":[{"name":%q,"type":"esriFieldTypeOID"},{"name":"NAME","type":"esriFieldTypeString"}]}`, s.oid)
		return
	}
	q := r.URL.Query()
	s.mu.Lock()
	s.queries = append(s.queries, q)
	s.mu.Unlock()

	desc := strings.HasSuffix(q.Get("orderByFields"), "DESC")
	lo, hi := 1, s.total
	where := q.Get("where")
	if i := strings.LastIndex(where, s.oid); i >= 0 {
		var n int
		if _, err := fmt.Sscanf(where[i:], s.oid+" > %d", &n); err == nil {
			lo = n + 1
		} else if _, err := fmt.Sscanf(where[i:], s.oid+" < %d", &n); err == nil {
			hi = n - 1
		}
	}
	var ids []int
	for i := lo; i <= hi; i++ {
		ids = append(ids, i)
	}
	if desc {
		for a, b := 0, len(ids)-1; a < b; a, b = a+1, b-1 {
			ids[a], ids[b] = ids[b], ids[a]
		}
	}
	var off, size int
	fmt.Sscanf(q.Get("resultOffset"), "%d", &off)
	fmt.Sscanf(q.Get("resultRecordCount"), "%d", &size)
	if off > len(ids) {
		off = len(ids)
	}
	ids = ids[off:]
	more := len(ids) > size
	if more {
		ids = ids[:size]
	}
	var rows []string
	for _, id := range ids {
		rows = append(rows, fmt.Sprintf(`{"properties":{%q:%d,"NAME":"n%d"}}`, s.oid, id, id))
	}
	fmt.Fprintf(w, `{"features":[%s],"exceededTransferLimit":%t}`, strings.Join(rows, ","), more)
}

func (s *pagedServer) last() url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queries[len(s.queries)-1]
}

func newPaged(t *testing.T, oid string, total int) (*Tools, *pagedServer) {
	t.Helper()
	ps := &pagedServer{oid: oid, total: total}
	srv := httptest.NewServer(http.HandlerFunc(ps.handler))
	t.Cleanup(srv.Close)
	c := cct.New(cct.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})
	t.Cleanup(c.Close)
	return New(c), ps
}

func collectIDs(t *testing.T, feats []Feature, oid string) []int {
	t.Helper()
	var ids []int
	for _, f := range feats {
		ids = append(ids, int(f.Attributes[oid].(float64)))
	}
	return ids
}

func TestKeysetPagingByObjectID(t *testing.T) {
	tl, ps := newPaged(t, "OBJECTID", 7)
	ctx := context.Background()
	seen := map[int]bool{}
	in := QueryLayerInput{Service: "ODP_SPLIT_3", LayerID: 2, Limit: 3}
	for page := range 5 {
		_, res, err := tl.queryLayer(ctx, nil, in)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		for _, id := range collectIDs(t, res.Features, "OBJECTID") {
			if seen[id] {
				t.Fatalf("id %d repeated", id)
			}
			seen[id] = true
		}
		if page == 1 {
			q := ps.last()
			if !strings.Contains(q.Get("where"), "OBJECTID > 3") || q.Get("resultOffset") != "0" {
				t.Errorf("second page should use OBJECTID > 3 with no offset, got where=%q offset=%q", q.Get("where"), q.Get("resultOffset"))
			}
		}
		if res.NextCursor == "" {
			break
		}
		in.Cursor = res.NextCursor
	}
	if len(seen) != 7 {
		t.Errorf("collected %d ids, want 7", len(seen))
	}
}

func TestKeysetPagingDescending(t *testing.T) {
	tl, ps := newPaged(t, "ObjectId", 5)
	ctx := context.Background()
	in := ServiceRequestsInput{TableQuery{Limit: 2}}
	_, res, err := tl.serviceRequests(ctx, nil, in)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if got := collectIDs(t, res.Features, "ObjectId"); got[0] != 5 || got[1] != 4 {
		t.Fatalf("page 1 ids = %v, want [5 4]", got)
	}
	in.Cursor = res.NextCursor
	if _, _, err := tl.serviceRequests(ctx, nil, in); err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if w := ps.last().Get("where"); !strings.Contains(w, "ObjectId < 4") {
		t.Errorf("descending page 2 where = %q, want ObjectId < 4", w)
	}
}

func TestOffsetPagingForOtherSorts(t *testing.T) {
	tl, ps := newPaged(t, "OBJECTID", 7)
	ctx := context.Background()
	in := QueryLayerInput{Service: "ODP_SPLIT_12", LayerID: 12, OrderBy: []string{"NAME"}, Limit: 3}
	_, res, err := tl.queryLayer(ctx, nil, in)
	if err != nil || res.NextCursor == "" {
		t.Fatalf("page 1: %v cursor=%q", err, res.NextCursor)
	}
	in.Cursor = res.NextCursor
	if _, _, err := tl.queryLayer(ctx, nil, in); err != nil {
		t.Fatalf("page 2: %v", err)
	}
	q := ps.last()
	if q.Get("resultOffset") != "3" || strings.Contains(q.Get("where"), "OBJECTID >") {
		t.Errorf("offset mode page 2: where=%q offset=%q, want offset 3 and no id filter", q.Get("where"), q.Get("resultOffset"))
	}
}

func TestCursorRejectedForDifferentQuery(t *testing.T) {
	tl, _ := newPaged(t, "OBJECTID", 7)
	ctx := context.Background()
	in := QueryLayerInput{Service: "ODP_SPLIT_3", LayerID: 2, Limit: 3, Where: "NAME <> 'x'"}
	_, res, err := tl.queryLayer(ctx, nil, in)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	in.Cursor, in.Where = res.NextCursor, "NAME <> 'y'"
	if _, _, err := tl.queryLayer(ctx, nil, in); err == nil || !strings.Contains(err.Error(), "different query") {
		t.Errorf("want a different-query error, got %v", err)
	}
	in.Cursor, in.Where = "not-a-cursor", "NAME <> 'x'"
	if _, _, err := tl.queryLayer(ctx, nil, in); err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Errorf("want an invalid-cursor error, got %v", err)
	}
}

func TestCursorWorksWhenFieldsOmitObjectID(t *testing.T) {
	tl, ps := newPaged(t, "OBJECTID", 5)
	ctx := context.Background()
	_, res, err := tl.queryLayer(ctx, nil, QueryLayerInput{Service: "ODP_SPLIT_3", LayerID: 2, Limit: 2, Fields: []string{"NAME"}})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if of := ps.last().Get("outFields"); of != "NAME,OBJECTID" {
		t.Errorf("outFields = %q, want the object ID appended", of)
	}
	if res.NextCursor == "" {
		t.Error("expected a next_cursor")
	}
}
