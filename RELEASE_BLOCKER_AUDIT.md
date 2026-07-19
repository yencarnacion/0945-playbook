# Real-time release-blocker audit

Audit date: 2026-07-19 (America/New_York)
Audited HEAD: `b136dc25667f86d54a363213bfd242b914f3e984`

## Executive result

This patch fixes the four correctness defects that were reproducible at HEAD: prior-session contamination of C/Avg, non-authoritative/non-versioned gap repair, loss of real provider HTTP status metadata, and event-driven-only health aging. It also adds one backend alert-safety boundary and removes three string allocations per ingested event.

The Go suite, race detector, vet, provider HTTP integration tests, recovery tests, and the existing headless-browser tests pass. The corrected fixed-rate engine soak uses the production queue and does not inspect queue depth. A real Massive market-open validation was not run because no entitled credentials/open session were available.

The repository still does **not** contain one test that combines the production WebSocket client, all scanners, SSE, and Chromium in a single topology. Existing tests prove those stages separately. Consequently, mandatory criteria 29 and the per-scan provider-event-to-DOM percentiles are not claimed as passed.

## Verification of reported concerns

| Concern at audited HEAD | Finding | Root cause | Patch result |
|---|---|---|---|
| Prior summaries entered C/Avg bars | Confirmed | `seedPriorSummary` inserted two synthetic bars into the shared engine and `Snapshot` selected the last 15 without a session boundary | Fixed: summaries live in `StreamingLive.prior`; C/Avg accepts only 04:00–20:00 New York bars from its current market date |
| Gap recovery accepted any nonempty response | Confirmed | `ResolveGap` called `Seed`, where existing streaming bars won, then unconditionally cleared the boolean gap | Fixed: ID/version tokens, complete minute coverage, atomic authoritative replacement, stale-token rejection, derived-state rebuild |
| Real Massive errors bypassed governor | Confirmed | SDK iterator errors were returned unchanged | Fixed: response-metadata transport plus SDK error adapter yields internal `HTTPError`; transport integration tests cover 429/5xx/auth |
| Health changed only on events | Confirmed | publication returned when the dirty set was empty | Fixed: every publication tick runs an allocation-free status sweep and dirties only visible transitions |
| Alert safety spread across frontend/backend | Partially fixed | backend suppressed some main signals, while browser inferred new C/Avg/Kane membership | Fixed for live paths: one eligibility function, stable logical IDs, backend dedupe, browser consumes eligibility and separately remembers IDs |
| Full snapshots on ingest hot path | Confirmed | gap check called `Snapshot`, copying retained bars | Fixed: ingestion uses `GapState`; health uses `SymbolStatus` |
| Scanner history copying | Partially fixed | original playbook still receives a copied bar slice on dirty-symbol evaluation; Kane clones current bars when its one-second result is rebuilt | Remaining limitation; correctness retained pending a recorded-session incremental parity fixture |
| Sustained test used oversized queue/throttle | Confirmed | capacity 32,768 and producer waited on queue depth | Fixed: capacity 8,192 and a fixed 6,000/s pacer independent of queue state |
| One unified WS-to-Chromium test | Confirmed missing | WebSocket, engine, and Chromium tests were separate | Still missing; explicitly failed below |

## Architecture

Before:

```text
prior summary -> synthetic minute bars --+
live aggregates -> shared bars ----------+-> full Snapshot copies -> all scans
                                             |
gap -> any nonempty Seed(existing wins) -> clear
SDK error (opaque) -> governor misses status
new event -> health calculation -> SSE -> browser infers alerts
```

After:

```text
                         +-> session-bounded O(1)-sized C/Avg window
Massive WS -> bounded -> symbol bars -> dirty symbols -> playbook/Kane -> SSE hub
               queue          |               ^                       -> clients
                              +-> narrow health view -> timed sweep

prior summaries -> immutable Kane-only store

gap ID/version -> begin -> governed interval backfill -> verify every minute
              -> atomic authoritative replace + rebuild -> CAS commit

Massive HTTP -> response metadata + SDK adapter -> normalized HTTPError -> governor

all scan conditions -> backend alert eligibility + logical-ID dedupe -> browser
```

## Semantics and recovery protocol

C/Avg uses the current developing close divided by the mean of that close and the previous 14 eligible minute closes. Eligibility is the same New York market date and the configured strategy’s extended session boundary, 04:00 inclusive through 20:00 exclusive. Prior summaries are never engine bars. The snapshot exposes session, eligible count, window endpoints, last completed minute, and developing minute.

Each gap has an ID and increasing version. Recovery captures a token, requests only that interval, rejects empty/partial/wrong-interval results, checks the token again under the symbol lock, replaces every covered minute, marks those completed minutes authoritative, rebuilds OHLCV/VWAP-derived state, and clears only the matching version. A late streaming aggregate cannot overwrite or double-count a recovered minute. If loss expands version 4 to version 5, version 4 cannot commit.

Coverage is deliberately conservative: every minute in a declared gap must be represented. A legitimate halt/no-trade interval therefore remains recovering unless the provider/recovery layer supplies authoritative evidence for that empty minute. This is safe for alerts but is an operational limitation.

HTTP response status, `Retry-After`, request ID, method, endpoint, provider message, and SDK error are normalized. Only 429, 500, 502, 503, 504, and temporary repeatable network failures retry. 401/403 do not retry. Both integer and HTTP-date `Retry-After` forms are parsed.

Health precedence implemented by the live scheduler is `DISCONNECTED`, `RECOVERING`, `WARMING`, `STALE`, `READY`. It evaluates connectivity, full subscription count, gap state, history/prior readiness, and event age without copying bar history. A visible transition advances publication generations even without a new event. Kane incorporates prior readiness, feed state, subscription state, gaps, and global event age.

## Measurements

### Ingestion benchmark

| Metric | Before hot-key optimization | After | Change |
|---|---:|---:|---:|
| 6,000-event benchmark time | 7.12–7.29 ms | 5.69–5.95 ms | about 20% faster |
| allocations | 40,006/op | 22,006/op | 45% fewer |
| bytes | 13,137,634–13,137,723/op | 12,273,580–12,273,587/op | 6.6% fewer |

The profile identified timestamp/string duplicate keys and allocation/GC as a hot path. Replacing string keys with two `int64` timestamps removed three string allocations per record. The before CPU top included `runtime.scanobject` 13.94%, `runtime.mallocgc` cumulative 27.99%, and `Engine.apply` cumulative 31.11%.

### Load/browser evidence

| Test | Result |
|---|---|
| Fixed 6,000/s, 2,000-symbol, production-queue short soak | 30,000/30,000 processed in 5.000 s; 0 dropped; 0 queued; heap 8,100,736 -> 11,269,248 bytes; 5,088,688 allocated; 3 goroutines |
| Fixed 6,000/s, 2,000-symbol, production-queue 30-second soak | 180,000/180,000 processed in 30.001 s; 0 dropped; 0 queued; heap 8,100,960 -> 13,074,384 bytes; 9,053,408 allocated; 3 goroutines |
| Chromium 2,000-row C/Avg and Kane full render | 1.419 s including browser startup; JSON snapshot 1,392,408 bytes |
| Existing SSE-to-DOM Chromium test | 1.010 s including startup |
| Unified provider WebSocket-to-DOM | Not implemented/proven |

The separate browser test does not pass precomputed rows through scanners and is therefore not reported as complete event-to-render evidence. Per-scan p50/p95/p99/max are unavailable for the unified path.

## REST and WebSocket audit

| Path | Purpose | Startup | Steady state | Recovery/failure behavior |
|---|---|---:|---:|---|
| Stocks aggregate WebSocket | all three live scans | one connection; explicit batched symbols | one connection, zero REST polling | reconnect and resubscribe; gaps remain recovering |
| Per-symbol minute aggregate REST | targeted prior/gap data | only missing cache/summary | approximately zero | global 80 rps default, concurrency 8, strict retry limit, circuit breaker |
| Prior summary cache | Kane immutable input | one compact file/symbol | none | missing symbols keep Kane WARMING |

Provider integration tests made real loopback HTTP requests through the Massive SDK adapter for 429, 500, 502, 503, 504, 401, and 403. A 503,503,200 sequence made exactly three governed requests and two retries.

## Validation commands

```bash
git rev-parse HEAD
GOCACHE=/tmp/0945-go-cache go test ./...
GOCACHE=/tmp/0945-go-cache go test -race ./...
GOCACHE=/tmp/0945-go-cache go vet ./...
node --check internal/server/web/app.js
GOCACHE=/tmp/0945-go-cache go test ./internal/data -run 'TestMassiveProvider' -v -count=1
GOCACHE=/tmp/0945-go-cache go test ./internal/server -run 'TestChromium(CAVGAndKaneRender|EventToDOMRender)' -v -count=1
GOCACHE=/tmp/0945-go-cache PLAYBOOK_SUSTAINED=1 PLAYBOOK_SUSTAINED_SECONDS=5 go test ./internal/market -run TestSustainedOpeningFiveMinutes2000Symbols -v -count=1
GOCACHE=/tmp/0945-go-cache PLAYBOOK_SUSTAINED=1 go test ./internal/market -run TestSustainedOpeningFiveMinutes2000Symbols -v -count=1
GOCACHE=/tmp/0945-go-cache go test ./internal/market -run '^$' -bench BenchmarkOpeningBurst2000 -benchmem -count=3 -cpuprofile /tmp/0945-cpu.out -memprofile /tmp/0945-mem.out
GOCACHE=/tmp/0945-go-cache go tool pprof -top /tmp/0945-cpu.out
```

## Production runbook

Start before 09:15 America/New_York. Confirm one WebSocket connection, expected subscription count, synchronized host clock, current prior-session summary date/coverage, and zero gaps. C/Avg needs 14 completed eligible minutes plus a developing minute; Kane stays WARMING until every required prior summary exists. Do not enable alerts until `/api/health/latency` reports READY for the feed and relevant scan.

On disconnect, the engine versions the affected intervals and rows become RECOVERING/DISCONNECTED. Current streaming resumes first; only precise missing intervals use governed REST. Do not manually clear a gap. Investigate persistent partial coverage, authentication/entitlement errors, 429s, queue loss, or a provider halt. A recovered interval commits only after authoritative coverage. Browser clients that miss a generation are closed/resynchronized by the existing per-client hub protocol.

For market-open shadow validation, record `/api/health/latency` and browser Performance API telemetry from 09:29:30–09:35:00, including event/publication/render ages, generations, queue/client depths, gaps, reconnects, REST metrics, and alert IDs. Archive the report with build hash, config, entitlement/feed, watchlist size, and host clock status. Local evidence is not a substitute for this entitled live run.

## Acceptance status

| Criteria group | Status | Evidence/limitation |
|---|---|---|
| C/Avg isolation/warm-up/crossing safety (1–6) | Pass | session and alert tests; prior store physically separate |
| Gap authority/versioning (7–13) | Pass, conservative no-trade handling | deterministic replacement, partial, stale-version, idempotence tests |
| Provider HTTP/governor (14–19) | Pass locally | actual adapter transport tests; steady-state architecture has no REST loop |
| Time health/alerts (20–28) | Pass for implemented live paths | injectable-clock stale/disconnect/generation tests and centralized eligibility matrix |
| Unified topology (29–32, 39–41) | **Fail / not proven** | production WS and Chromium are tested separately, not in one test; no per-scan percentiles |
| Boundedness/stability (33–38) | Partial | 30-second fixed-rate soak and race pass; the full five-minute wall-clock soak command exists but was not completed in this audit |

## Changed files

- `internal/market/engine.go`
- `internal/market/session_cavg_test.go`
- `internal/market/recovery_test.go`
- `internal/market/sustained_test.go`
- `internal/data/governor.go`
- `internal/data/massive.go`
- `internal/data/massive_http_test.go`
- `internal/runner/streaming.go`
- `internal/runner/alerts.go`
- `internal/runner/alerts_test.go`
- `internal/runner/streaming_live_test.go`
- `internal/dashboard/state.go`
- `internal/playbook/engine.go`
- `internal/server/web/app.js`
- `RELEASE_BLOCKER_AUDIT.md`

## Remaining risks

- Entitlement, provider transit latency, adjustment parity, and actual 09:30 behavior require a real Massive shadow run.
- The unified production WebSocket-to-scanner-to-Chromium acceptance topology and per-scan distributions remain absent.
- Original playbook and Kane still copy/traverse more history than ideal; changing that safely requires recorded-session parity fixtures.
- Alert dedupe is process-memory durable, not persisted across process restart.
- The strict minute-coverage verifier needs explicit provider no-trade/halt evidence before it can safely clear such intervals.
