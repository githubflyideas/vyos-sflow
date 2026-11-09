# Third-party notices

vyos-sflow is licensed under the Apache License 2.0 (see `LICENSE` and `NOTICE`). Third-party components keep their own licenses:

- Go module dependencies, listed in `go.mod` and `go.sum`, each under its own license. The embedded database is DuckDB, distributed under the MIT license through `github.com/duckdb/duckdb-go`.

## Historical upstream notices

The sections below are retained from the upstream Traffic66 notices. The DB-IP and Natural Earth data files they describe are not included in vyos-sflow builds (see also `internal/geo/dbip/README.md`).

### Third-party data in traffic66

traffic66 itself contains no third-party code beyond its Go modules (see
`go.mod`). It includes the following data.

### DB-IP Lite

`internal/geo/dbip/country.mmdb.gz` and `internal/geo/dbip/asn.mmdb.gz`:
DB-IP "IP to Country Lite" and "IP to ASN Lite", in MaxMind DB format as
packaged by the ip-location-db project.

IP Geolocation by DB-IP (https://db-ip.com), licensed under the Creative
Commons Attribution 4.0 International License
(https://creativecommons.org/licenses/by/4.0/). traffic66 shows this
attribution on the pages that use the data.

### Natural Earth

`internal/web/static/world.json`: country outlines derived from Natural
Earth 1:50m Admin 0 – Countries (https://www.naturalearthdata.com), public
domain, via world-atlas (https://github.com/topojson/world-atlas, ISC
license), simplified and projected by `tools/worldmap/build.mjs`.
