# Local deterministic acceptance evidence

- Source commit: `58b3586032dc4376194af8ec886b73dea8178529`
- Date: 2026-07-31 CST
- Host: `Darwin 25.5.0 x86_64`
- Go: `go1.26.5 darwin/amd64`
- Git: `2.55.0`
- Source input count: 18 enabled source IDs

## Clean runtime boundary

The source tree was created with `git archive HEAD` in a new temporary directory. The command environment used `env -i`, an empty Go module/build cache, `CGO_ENABLED=0`, and a purpose-built `PATH` containing only Go, Git, and required standard shell tools. It did not expose Python, Python 3, Node, or npm.

```text
runtime-check: python/python3/node/npm absent
fresh-bootstrap: OK
```

Docker was not used because the installed Docker CLI had no running daemon. This local proof therefore establishes the no-Python/Node runtime contract, but does not stand in for a GitHub-hosted Ubuntu Actions log.

## Commands and exit status

All commands below ran from the clean archive and exited 0:

```text
go run ./cmd/geodata-build bootstrap --cache-root .cache
go test -count=1 ./...
go vet ./...
go test -count=1 -tags=integration ./internal/app
go run ./cmd/geodata-build build --repo example/clash-rules-srs --release-tag acceptance-20260731
go run ./cmd/geodata-build verify --dat publish/geosite.dat --manifest build/expected_tags.json --side geosite
go run ./cmd/geodata-build verify --dat publish/geosite.dat --manifest build/expected_tags.json --side geosite --forbid
go run ./cmd/geodata-build verify --dat publish/geoip.dat --manifest build/expected_tags.json --side geoip
go run ./cmd/geodata-build verify --dat publish/geoip.dat --manifest build/expected_tags.json --side geoip --forbid
sha256sum -c geosite.dat.sha256sum
sha256sum -c geoip.dat.sha256sum
sha256sum -c install_passwall2_rules.sh.sha256sum
```

Key output:

```text
unit: OK
vet: OK
integration: OK
production-build: OK
four-tag-probes: OK
geosite.dat: OK
geoip.dat: OK
install_passwall2_rules.sh: OK
```

The production build downloaded and parsed all 18 configured remote sources. The generated manifest contained 16 required GeoSite tags, 5 required GeoIP tags, 16 forbidden GeoSite tags, 5 forbidden GeoIP tags, and 18 source provenance records.

## Exact asset set

```text
geoip.dat                                      17,483,046 bytes  0644
geoip.dat.sha256sum                                    76 bytes  0644
geosite.dat                                     9,117,274 bytes  0644
geosite.dat.sha256sum                                  78 bytes  0644
install_passwall2_rules.sh                          8,289 bytes  0755
install_passwall2_rules.sh.sha256sum                   93 bytes  0644
```

SHA-256 values from this run:

```text
221763f037576a96963bf6700a3270782035ea0078e3455de928343cae347609  geosite.dat
9662f248b12972ae7ffbfbb785d1b006b51d533bf6697e86245e83dd50b983d0  geoip.dat
843d0616b957099f12aa98af258712534ddca644eebdfb93a30d2d1ca4729e74  install_passwall2_rules.sh
```

## Additional review-fix verification

The worktree also passed these final-HEAD commands before the clean archive run:

```text
go test -race -count=1 ./internal/workspace ./internal/passwall ./internal/rules ./internal/app
GOOS=linux GOARCH=amd64 go test -c ./internal/workspace
```

The first command covers the concurrent workspace switch, installer recovery, YAML parsing, and production orchestration fixes under the race detector. The second confirms the Unix lock implementation compiles for the Ubuntu/OpenWrt Linux target.
