# Runtime Rule Residuals Review Report

Review range: `f2276afe7a7204d1d6d47294812bb7079c0e2944...b74c025f0498d7d3b134926e57fafe9fc4a57483` plus uncommitted tracked patch `8317a72d938ad62c3a551292090e54de7264d39f65484d7f0e686804b27eb77e` and the untracked source files enumerated in `static-metrics.json`.

## Result

All confirmed high and medium implementation findings were fixed under TDD. The final 13-group affected-dimension re-reviews closed their findings. Device-backed RSS subsequently exposed a real acceptance failure in both Xray matrices; route equivalence remains blocked for lack of per-request outbound traces.

## Fixed findings

- Replaced the production-scale `basis × coverage` residual scan with indexed conservative coverage and context cancellation. Complete production-sized replay builds now finish normally.
- Derived residual topology from the actual groups configuration. The unique proxy group references `crs-proxy-residual` plus full `tld-not-cn`; only same-group companion tags lend later-equivalent coverage, so `proxy → direct → tld` remains conservative.
- Made generated-only tag identity case-insensitive across source, custom, community and generated writes. Community lookup and write-side defense recurse through nested files using the pinned compiler's basename semantics.
- Added exact public-tag fingerprints and generated semantic fingerprints for kind, value, normalized attributes and conservative coverage. Full `gfw`, `proxy`, `tld-not-cn`, `direct` and unchanged `reject` remain guarded.
- Made PassWall2 legacy detection node-wide. If any historical gfw/tld field remains, main, `_fakedns` and `_proxy_tag` are all preflighted; any proxy/tld disagreement fails before live UCI modification. Canonical proxy-only nodes remain idempotent, and successful migration removes gfw/tld/BilibiliHMT families.
- Removed the independent tld/BilibiliHMT Shadowrocket policies. Full tld rules now use proxy remarks and the generated config has exactly 13 groups.
- Canonicalized supported compiler GeoSite output before publication. This closes nondeterministic map/tag/rule/attribute ordering; two consecutive sealed-input builds produced byte-identical GeoSite DAT, Shadowrocket config and installer hashes. A final review found that tolerant decoding could otherwise strip future protobuf extensions, so canonicalization now rejects unknown document/tag/domain fields and the transaction preserves old publish.

## Deferred low findings

- Cancellation is not polled while the residual coverage index itself is being built; current production-scale construction finishes within the verified build envelope.
- The expected public-tag map remains live until Shadowrocket expansion, increasing peak lifetime without changing semantics.
- App and GeoSite validation perform separate recursive community scans.
- PassWall preflight and migration retain two AWK state machines.

These are maintainability or bounded-efficiency items with no reproduced correctness failure. They were not expanded beyond the approved optimization scope.

## Verification

- `go test ./... -count=1`, integration tests, `go vet ./...`, release comparison, four checksum checks and `git diff --check` pass.
- Baseline and candidate complete against one immutable source snapshot; every replayed URL hash is logged.
- Combined proxy rules fall from 32,208 to 18,938 (58.80% of baseline). Total active rules, Shadowrocket rule lines and rule bytes all fall; static acceptance is PASS.
- Real-router RSS results are Xray AsIs FAIL (3.6049% reduction), Xray IPIfNonMatch FAIL (0.7585%), and sing-box PASS after five independent cold starts (9.1015%). Because all three must pass, overall RSS is FAIL.
- Router route equivalence remains BLOCKED because the RSS warmup did not capture per-request rule-hit/outbound evidence. Shadowrocket device import remains DEFERRED.
