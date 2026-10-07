package tools

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	arcgis "github.com/richardwooding/go-arcgis"
)

// Paging modes. Keyset paging on the object ID survives rows being added or
// removed between calls; offset paging is the fallback when the sort uses
// other fields, where a keyset would need a composite comparison.
const (
	pageByID     = "id"
	pageByOffset = "offset"
)

type cursor struct {
	Mode string `json:"m"`
	Key  int64  `json:"k"`
	Hash string `json:"h"`
}

var errForeignCursor = errors.New("this cursor belongs to a different query; repeat the original filters, or start again without cursor")

func encodeCursor(c cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s, hash string) (cursor, error) {
	var c cursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	if err != nil || (c.Mode != pageByID && c.Mode != pageByOffset) {
		return cursor{}, fmt.Errorf("cursor %q is not valid; pass next_cursor back exactly as returned", s)
	}
	if c.Hash != hash {
		return cursor{}, errForeignCursor
	}
	return c, nil
}

// queryHash fingerprints what determines the row sequence, so a cursor cannot
// be replayed against a query with different filters or ordering.
func queryHash(service string, p arcgis.QueryParams, bbox []float64, polygon [][][]float64) string {
	b, _ := json.Marshal([]any{service, p.LayerID, p.Where, p.OrderByFields, bbox, polygon})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:6])
}

// pagingMode chooses keyset paging when the sort is empty or only the object
// ID, and reports whether that sort is descending.
func pagingMode(order []string, oid string) (mode string, desc bool) {
	if oid == "" {
		return pageByOffset, false
	}
	switch len(order) {
	case 0:
		return pageByID, false
	case 1:
		col, dir, _ := strings.Cut(strings.TrimSpace(order[0]), " ")
		if strings.EqualFold(col, oid) {
			return pageByID, strings.EqualFold(strings.TrimSpace(dir), "DESC")
		}
	}
	return pageByOffset, false
}

func idCondition(oid string, desc bool, last int64) string {
	op := ">"
	if desc {
		op = "<"
	}
	return fmt.Sprintf("%s %s %d", oid, op, last)
}

func andWhere(a, b string) string {
	if a == "" {
		return b
	}
	return "(" + a + ") AND (" + b + ")"
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}
