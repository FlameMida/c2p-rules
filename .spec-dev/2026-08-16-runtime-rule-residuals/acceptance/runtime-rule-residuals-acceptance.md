# Runtime Rule Residuals Acceptance

## Result

The 13-group implementation, repository regression and static-size contract pass. Real-router RSS acceptance fails because both Xray matrices miss the required 5% reduction; sing-box passes after independent cold-start diagnosis. Per-request route equivalence remains blocked because the RSS run did not collect rule-hit/outbound traces. Shadowrocket device import remains deferred by design.

- Baseline: `f2276afe7a7204d1d6d47294812bb7079c0e2944`
- Candidate: `b74c025f0498d7d3b134926e57fafe9fc4a57483` plus uncommitted working tree; tracked patch SHA-256 `8317a72d938ad62c3a551292090e54de7264d39f65484d7f0e686804b27eb77e`
- Input snapshot SHA-256: `ff4587b6d9da756e7264d5f9e01132219e095bd7584bf47df74d614887eda805`
- Overall status: **FAIL** (RSS threshold not met in two of three matrices)

## Acceptance matrix

| Scenario | Status | Evidence and exact result |
|---|---|---|
| Xray AsIs domain routing equivalence | BLOCKED | The router and core were exercised, but the RSS-only warmup did not record rule hit and final outbound per request. HTTP health codes are not counted as route-equivalence evidence. |
| Xray IPIfNonMatch second-pass IP equivalence | BLOCKED | No baseline/candidate four-column domain/IP second-pass route diff was captured. |
| sing-box native routing equivalence | BLOCKED | No native rule-set hit and final outbound trace was captured; successful HTTP warmup is insufficient. |
| Combined proxy rules reduce at least 30% and overall size does not rise | PASS | Baseline `gfw + proxy + tld_not_cn`: 32,208 → candidate unique `proxy`: 18,938 (58.80%, required ≤70%). Active domain rules: 316,761 → 297,589 (93.95%); Shadowrocket rule lines: 322,228 → 303,044 (94.05%); `[Rule]` bytes: 14,152,790 → 13,414,445 (94.78%). Each overall metric is strictly lower. See `static-metrics.json`. |
| Xray AsIs RSS reduction at least 5% | FAIL | Median `115,288 → 111,132 KiB`; candidate ratio 96.3951%, reduction 3.6049%, required ratio ≤95%. |
| Xray IPIfNonMatch RSS reduction at least 5% | FAIL | Median `115,488 → 114,612 KiB`; candidate ratio 99.2415%, reduction 0.7585%, required ratio ≤95%. |
| sing-box RSS reduction at least 5% | PASS | Initial single-process comparison was contradicted by A-B-A, proving cold-start heap noise. Five independent cold starts per side give median process RSS `55,112 → 50,096 KiB`, ratio 90.8985%, reduction 9.1015%. Candidate SRS also falls `2,144,539 → 2,036,918` bytes. |
| Repository complete regression | PASS | Full unit/integration tests, `go vet ./...`, production-sized immutable-replay candidate build, four checksums, release comparison, exact eight-file inventory and whitespace checks all exit 0. The candidate contains exactly 13 policy groups. Two consecutive candidate builds from the sealed input are byte-identical for `geosite.dat`, `shadowrocket.conf` and the installer. See `local-regression.log`, `static-build-replay.log` and `publish-inventory.txt`. |
| Shadowrocket device import | DEFERRED | The active spec explicitly defers iPhone import. This is not recorded as PASS. |

## Static-size result

The same-source comparison is valid: both sides consume the sealed snapshot identified above, and `static-build-replay.log` records each URL, byte count and SHA-256. The baseline combined proxy universe is the sum of its active `gfw`, `proxy` and `tld_not_cn` groups (4,337 + 27,042 + 829 = 32,208). The candidate's single proxy group expands `crs-proxy-residual` and full `tld-not-cn` to 18,938 group-deduplicated rules, a 41.20% reduction. Direct falls from 111,528 to 105,634. Reject remains intentionally unchanged at 165,372, so total active rules fall by 19,172 (6.05%); the active contract requires this total, Shadowrocket rule lines and rule bytes to be strictly lower, not each to fall by 30%.

## Requirement and test reconciliation

- Final affected-dimension review is rerun against the 13-group working tree; its final findings are recorded in `review-report.md`.
- The previous production-scale runaway is fixed: complete production and immutable-replay builds now finish and publish exactly eight valid assets.
- The pinned GeoSite compiler's map iteration made otherwise identical builds byte-unstable. The candidate now validates and canonicalizes compiler output before publication; two sealed-input builds produced identical SHA-256 values for all three GeoSite-derived assets. Unknown protobuf fields at document/tag/domain layers fail closed and preserve old publish instead of being stripped.
- The candidate has 13 groups. Full `tld-not-cn` remains in the DAT/public input contract but shares the unique proxy group; no independent PassWall2 or Shadowrocket tld group remains.
- BilibiliHMT removal is the approved contract exception and is excluded from the retained 13-group route-equivalence corpus.
- Router RSS is now verified: sing-box passes, while both Xray matrices fail the 5% threshold, so the combined RSS Requirement fails.
- Route equivalence remains unverified, not waived. The standardized SOCKS warmup was suitable for core load but did not instrument final outbound decisions from a LAN client.
- Browser, visual, accessibility, web performance and API load dimensions are not applicable to this Go CLI/router asset change. The environment detector found the Go test stack and no relevant web stack; no missing browser tooling was treated as a pass.
