# capetown-opendata-mcp

A comprehensive [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server that
exposes the [City of Cape Town Open Data Portal](https://citymaps.capetown.gov.za) to MCP
clients such as Claude Desktop, Claude Code, and any other MCP-compatible host.

It wraps [`capetown-opendata`](https://github.com/richardwooding/capetown-opendata) and
[`go-arcgis`](https://github.com/richardwooding/go-arcgis), turning the City's ArcGIS Feature
Service into a set of LLM-callable tools with discovery, a generic query escape hatch, spatial
and attribute filtering, and in-memory response caching.

## Tools

| Tool | Description |
| --- | --- |
| `load_shedding_blocks` | Load shedding block polygons (block geometry and ID). |
| `wards` | Municipal ward boundaries (ward name, key, year). |
| `land_parcels` | Cadastral land parcels (erven); optional `suburb` filter. |
| `taxi_routes` | Registered minibus taxi routes. |
| `water_quality` | Inland water quality sampling results, newest first. |
| `public_lighting` | Public street lighting assets. |
| `heritage_inventory` | Heritage inventory sites and features. |
| `query_layer` | Generic query over any layer by ID (where/fields/order/bbox/offset/count-only). |
| `field_values` | List the distinct values of a field on a layer (discover valid filter values). |
| `service_info` | List the service's layers and tables with their IDs; `name_contains` filters the listing. |
| `layer_info` | Describe a layer's fields, geometry type, and page size. |

Every feature-returning tool accepts a shared set of filters: `limit` (default 200, max 2000),
`offset` (skip N features; pair with the `next_offset` in the response to page through a layer),
`where` (extra SQL filter, AND-combined), `bbox` (`[minLon, minLat, maxLon, maxLat]` in WGS84),
`polygon` (rings `[[[lon,lat],…],…]` in WGS84, for irregular areas like a ward boundary),
`include_geometry` (default `false`), `omit_nulls` (drop null-valued attributes), and
`use_aliases` (rename raw column names to their human-readable field aliases). Spatial filters are
sent as WGS84 (`inSR=4326`), so they work against layers stored in any projection.

> **Note:** Published ArcGIS layer IDs occasionally drift. Use `service_info` / `layer_info` to
> discover current IDs and field names, and `query_layer` to query a layer whose dedicated tool's
> ID has changed.

## Install

### MCP Bundle (Claude Desktop, one-click)

Every [release](https://github.com/richardwooding/capetown-opendata-mcp/releases) attaches
**MCP Bundles** (`.mcpb`) — one per platform. Download the bundle matching your OS and
architecture (e.g. `capetown-opendata-mcp_<version>_darwin_arm64.mcpb`) and open it with
Claude Desktop to install. The bundle's settings screen lets you set an optional ArcGIS
token, request timeout, and cache TTL.

### Homebrew (macOS / Linux)

```sh
brew install richardwooding/tap/capetown-opendata-mcp
```

### Container image

```sh
docker run -i --rm ghcr.io/richardwooding/capetown-opendata-mcp:latest
```

### From source

```sh
go install github.com/richardwooding/capetown-opendata-mcp@latest
```

## Usage

By default the server speaks the MCP **stdio** transport:

```sh
capetown-opendata-mcp
```

### Claude Desktop / Claude Code config

```json
{
  "mcpServers": {
    "capetown-opendata": {
      "command": "capetown-opendata-mcp"
    }
  }
}
```

### HTTP transport

For remote or containerized deployments, run the streamable-HTTP transport:

```sh
capetown-opendata-mcp --transport http --http-addr :8080
```

### Configuration

All flags can also be set via environment variables (prefix `CAPETOWN_MCP_`):

| Flag | Env | Default | Description |
| --- | --- | --- | --- |
| `--transport` | `CAPETOWN_MCP_TRANSPORT` | `stdio` | `stdio` or `http`. |
| `--http-addr` | `CAPETOWN_MCP_HTTP_ADDR` | `:8080` | Listen address for the HTTP transport. |
| `--timeout` | `CAPETOWN_MCP_TIMEOUT` | `30s` | Per-request upstream timeout. |
| `--cache-ttl` | `CAPETOWN_MCP_CACHE_TTL` | `5m` | Response cache TTL (`0` disables caching). |
| `--arcgis-token` | `CAPETOWN_MCP_ARCGIS_TOKEN` | _(none)_ | Optional ArcGIS token for authenticated services. |

## Development

```sh
go build ./...
go test -race ./...
golangci-lint run
goreleaser release --snapshot --clean   # local dry run
```

## Releasing

Pushing a `vX.Y.Z` tag triggers the release workflow, which uses
[GoReleaser](https://goreleaser.com) to:

- build binaries for linux/darwin/windows (amd64 + arm64),
- pack each binary into an **MCP Bundle** (`.mcpb`) and attach all six to the release,
- build and push a multi-arch OCI image to GHCR with [ko](https://ko.build),
- publish a Homebrew **cask** to [`richardwooding/homebrew-tap`](https://github.com/richardwooding/homebrew-tap).

The release requires a `HOMEBREW_TAP_GITHUB_TOKEN` repository secret (a PAT with `repo` scope on
the tap). GHCR uses the built-in `GITHUB_TOKEN`.

### MCP Bundles

`tools/mcpb` is a small, dependency-free (Go stdlib) packer that zips the server binary and a
generated `manifest.json` into a `.mcpb`. GoReleaser invokes it per build target, but you can
build a bundle for your current platform locally (no Node required):

```sh
go build -o capetown-opendata-mcp .
go run ./tools/mcpb pack -version dev
```

Because the manifest format selects a binary by OS only (not architecture), each release ships
one bundle per OS+arch.

## License

MIT — see [LICENSE](LICENSE).
