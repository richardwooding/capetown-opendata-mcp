package cct

import (
	"fmt"
	"net/url"
	"strings"

	capetown "github.com/richardwooding/capetown-opendata"
)

// DefaultServer names the City server used when none is configured.
const DefaultServer = "esapqa"

var namedServers = map[string]string{
	"esapqa":   capetown.FolderESAPQA,
	"citymaps": capetown.FolderCityMaps,
}

// ResolveServer maps a server name ("esapqa", "citymaps") or an https URL of
// an ArcGIS REST services folder to the folder the ODP_SPLIT services live in.
func ResolveServer(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		n = DefaultServer
	}
	if f, ok := namedServers[strings.ToLower(n)]; ok {
		return f, nil
	}
	u, err := url.Parse(n)
	if err != nil || u.Scheme != "https" || u.Host == "" || !strings.Contains(u.Path, "/rest/services") {
		return "", fmt.Errorf("unknown server %q; use esapqa, citymaps, or an https URL of an ArcGIS REST services folder", name)
	}
	return strings.TrimRight(n, "/"), nil
}
