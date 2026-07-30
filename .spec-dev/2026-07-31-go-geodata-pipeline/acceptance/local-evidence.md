# Local deterministic acceptance evidence

- Verified code commit: `da5a5680c19eebfe3e910b9fb969030c2a122d3e`
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
5736ab2f3e0428b5ef7564f82c7f7d9e3d37fd29212dc04e5d2c4a406cf16e5e  geosite.dat
9662f248b12972ae7ffbfbb785d1b006b51d533bf6697e86245e83dd50b983d0  geoip.dat
b9120625e67c5e797f367447e7e62f8c7c66bcbdd9bfe41dcfcda3dcf32cb3ba  install_passwall2_rules.sh
```

## Additional review-fix verification

The worktree also passed these final-HEAD commands before the clean archive run:

```text
go test -race -count=1 ./internal/workspace ./internal/passwall ./internal/rules ./internal/app
GOOS=linux GOARCH=amd64 go test -c ./internal/workspace
```

The first command covers the concurrent workspace switch, installer recovery, YAML parsing, and production orchestration fixes under the race detector. It includes injected post-publish lock-release failure and faithful UCI `-P` NOCOMMIT modeling while the generated installer uses `-t`. The second confirms the Unix lock implementation compiles for the Ubuntu/OpenWrt Linux target.
