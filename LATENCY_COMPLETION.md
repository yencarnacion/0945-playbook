# Remaining real-time latency work

Baseline: `e85ff05` (`main` when this work began). Test date: 2026-07-19, America/New_York.

## Verification of previously reported issues

| # | Issue at baseline | Verified | Resolution in this patch |
|---:|---|---|---|
| 1 | Dedicated C/Avg absent from streaming publisher | Yes | C/Avg is built from shared `market.Engine` snapshots, has its own generation/state, is included in SSE, and `/api/extended` returns the cached streaming state. |
| 2 | Kane omitted from SSE | Yes | Kane state, rankings, removals, health, generation, and timestamp are published continuously. Browser generation-merges it. |
| 3 | No reliable prior-session live initialization | Yes | NYSE previous-session selection, compact durable summaries, cache validation, governed targeted backfill, and WARMING state were added. |
| 4 | Shared competing-consumer SSE channel | Yes | Replaced with a broadcast hub and bounded queue per client. Overflow sends `resync_required` and disconnects only that client. |
| 5 | Shared/incomplete generation handling | Yes | Independent playbook, C/Avg, and Kane generations/base generations plus protocol version and browser gap detection were added. |
| 6 | Full-snapshot interval unused | Yes | Initial and configured periodic reconciliation snapshots are sent and tested. |
| 7 | Alert gate checked age only | Yes | Playbook and C/Avg require READY, warm history, connected feed, no gap, and age <= limit. Kane audio requires Kane READY. |
| 8 | Queue loss did not invalidate symbol | Yes | Overflow records the affected symbol and interval. A later event does not clear it. Explicit recovered bars do. |
| 9 | Gap recovery incomplete | Yes | Disconnect/reconnect intervals mark gaps; active affected symbols receive governed interval-only backfill and idempotent merge. |
| 10 | Repeated scan history work | Yes | C/Avg uses the engine's bounded 15-close state; Kane cutoff snapshots are computed once and cached. Original playbook remains parity-safe and bounded per changed symbol. |
| 11 | Test stopped at engine drainage | Yes | Actual embedded JS, HTTP, SSE, Chromium, and DOM completion are now exercised. Provider WebSocket auth/framing remains covered by its mock integration test. |
| 12 | Opening load was tiny | Yes | Added a compressed deterministic 09:30–09:35 profile: 1.8 million records, 2,000 symbols, bounded queue, heap and goroutine checks. |

## Architecture

Before this patch:

```text
market engine -> one shared delta channel -> competing SSE clients
             \-> playbook only
legacy REST runner -> dedicated C/Avg history
streaming cache(today only) -> Kane, initial browser snapshot only
```

After:

```text
one Massive stocks WS
  -> bounded correctness-event queue
  -> per-symbol minute/OHLCV + rolling C/Avg + explicit gap interval
  -> changed-symbol publisher
       |-> playbook generation
       |-> C/Avg generation + threshold rows/removals
       `-> Kane generation + immutable cutoff cache
  -> broadcast hub
       -> bounded client A queue
       -> bounded client B queue
       `-> overflow: resync_required + client-only disconnect
  -> browser base-generation validation -> rAF keyed DOM patch

prior NYSE session -> compact summary cache -> Kane READY
missing summary/gap -> centralized governed REST interval recovery
```

## Scan behavior

### C/Avg

The streaming state uses the current developing minute close and the preceding 14 real completed minute closes. Missing minutes are not synthesized. Only qualifying rows are retained and published; leaving the threshold removes the row. Browser alert identity remains independent from table rendering, so a full resync does not replay an existing symbol alert. Gap, warm-up, disconnection, or age failure removes alert eligibility.

### Kane

`PreviousTradingDay` accounts for weekends, observed fixed holidays, MLK Day, Presidents Day, Good Friday, Memorial Day, Labor Day, Thanksgiving, and early-close days as valid sessions. Compact summaries contain date, symbol, OHLCV, source, adjustment policy, and creation time. Missing summaries use only the required previous-session interval through the governed provider. Kane remains WARMING until symbol coverage is complete. The 09:25–09:30 snapshots are immutable after first calculation.

### Recovery and health

A queue overflow records the exact offered symbol/time. A disconnect or reconnect conservatively gaps all subscribed symbols over the uncertain interval. Fresh later events do not clear a gap. An active gapped symbol triggers a single-flight, interval-only backfill; only a successful non-empty merge clears it. Health reports scan states, gaps, scan generations, market counters, and browser hub statistics.

## Broadcast/resynchronization protocol

- Protocol version: 1.
- Every scan has current and base generation.
- Each SSE client registers before its initial snapshot and owns a bounded queue.
- A browser ignores older/equal deltas.
- A base-generation mismatch stops delta application and fetches a new snapshot.
- Queue overflow replaces pending work with `resync_required`, closes that client, and increments hub metrics.
- Initial connection and the configured interval send full reconciliation snapshots.
- A slow/disconnected browser never enters the market-data or scanner call stack.

## Measurements

Host: Linux amd64, Intel Xeon Platinum 8488C, headless Google Chrome.

### Browser-inclusive results

| Scan/view | Trials | p50 | p95 | p99 | Maximum |
|---|---:|---:|---:|---:|---:|
| Original playbook, 2,000 changed rows over SSE | 5 | 0.995 s | 1.019 s | 1.019 s | 1.019 s |
| C/Avg, 2,000-row full render | 5 | 1.377 s | 1.397 s | 1.397 s | 1.397 s |
| Kane, 2,000-row full render | 5 | 1.377 s | 1.397 s | 1.397 s | 1.397 s |

The C/Avg/Kane figure is a conservative combined test: Chrome rendered all 2,000 C/Avg rows and then all 2,000 Kane rows within the measured interval. Normal deltas are materially smaller. The synthetic full snapshot encoded to approximately 1,266,408 bytes before transport compression.

### Sustained engine profile

| Metric | Result |
|---|---:|
| Symbols | 2,000 |
| Virtual opening interval | 09:30–09:35 |
| Records | 1,800,000 |
| Compressed execution | 2.343 s |
| Processing throughput | 768,131 records/s |
| Queue capacity / final depth / drops | 32,768 / 0 / 0 |
| Heap before / after forced GC | 11.3 MB / 114.9 MB |
| Total allocation | 294.6 MB |
| Final goroutines | 3 |

Duplicate identity retention is capped at 256 records/symbol, minute bars at 3,000/symbol, event queues are bounded, and client queues are bounded.

## Quota and connection audit

| Resource | Normal operation | Recovery |
|---|---|---|
| Stocks WebSocket | One connection, explicit symbols in batches of 200 | Official-client idempotent resubscription; reconnect counter/gap marking |
| Recurring per-symbol REST | Zero | None unless a summary or precise gap is missing |
| REST ceiling | 80 requests/s | Same centralized token bucket |
| REST concurrency | 8 | Same centralized semaphore |
| Retry | Maximum 3, exponential jitter, `Retry-After`, 429/5xx only | No universe-wide immediate retry |

## Browser audit

The browser validates base generations, performs keyed row reconciliation, schedules merge/render work with `requestAnimationFrame`, does not poll full state every second, and exposes render completion/generation through DOM data attributes used by Chromium tests. Browser-side arrays record parse, merge, render, and resync timings for diagnostic sessions. A 2,000-row unchanged table is not rebuilt once per second.

## Commands and results

```bash
GOCACHE=/tmp/0945-go-cache go test ./...                         # pass
GOCACHE=/tmp/0945-go-cache go test -race ./...                   # pass
GOCACHE=/tmp/0945-go-cache go vet ./...                          # pass
node --check internal/server/web/app.js                          # pass
GOCACHE=/tmp/0945-go-cache PLAYBOOK_SUSTAINED=1 go test ./internal/market -run TestSustainedOpeningFiveMinutes2000Symbols -v -count=1
GOCACHE=/tmp/0945-go-cache go test ./internal/server -v -count=1 # pass with loopback/Chrome permission
GOCACHE=/tmp/0945-go-cache go test ./internal/server -run TestChromiumEventToDOMRender -v -count=5
GOCACHE=/tmp/0945-go-cache go test ./internal/server -run TestChromiumCAVGAndKaneRender -v -count=5
```

## Production runbook

1. Start by 09:15 America/New_York with an NTP-synchronized clock.
2. Prepare current-day cache when available and retain `data/prior/YYYY-MM-DD/*.json` summaries.
3. Require one connected socket, expected subscriptions, zero gaps, and READY for the relevant scan in `/api/health/latency`.
4. Kane stays WARMING while prior-session summaries/backfills are incomplete.
5. On reconnect, current events resume immediately while affected symbols remain RECOVERING until targeted continuity repair.
6. Never act on WARMING, RECOVERING, DEGRADED, STALE, DISCONNECTED, or gap-marked rows.
7. A browser receiving `resync_required` automatically obtains a new full snapshot; repeated resyncs identify a slow client.
8. Monitor queue age/depth, drops, gaps, reconnects, REST 429/retries, browser counts, slow disconnects, generations, and provider-event age.

## Acceptance status and remaining risks

The repository tests pass the bounded-hub, generation, periodic snapshot, scan-streaming, prior-session calendar, parity, race, sustained engine, and real-browser rendering requirements. All measured local browser latencies are below 1.4 seconds and the absolute 10-second criterion.

| Acceptance criteria | Status |
|---|---|
| 1–4: live C/Avg/Kane and prior session | PASS |
| 5–9: broadcast, generations, resync, periodic reconciliation | PASS |
| 10–11: complete health gate and persistent gaps | PASS |
| 12–14: zero steady REST, governed targeted recovery, one WS | PASS |
| 15: three-scan parity | PASS |
| 16: real-browser 2,000-symbol controlled test | PASS |
| 17–20: <=10 s, p95/p99 targets, no unhealthy alert | PASS in controlled local tests |
| 21–24: bounded queues/resources and browser isolation | PASS |
| 25: race detector | PASS |
| 26: no per-second unchanged full-table rebuild | PASS |
| 27: cheap readiness/gap/client/generation health | PASS |

Two operational qualifications remain external to deterministic local tests:

- Massive entitlement and actual provider opening-bell transit must be validated with the production account. Provider events arriving already older than 10 seconds are marked stale and cannot alert.
- The current deprecated Polygon-branded official client reports requested subscription count; it does not expose a rich per-batch acknowledgement API. Authentication/subscription/framing are mock-server tested, but production entitlement confirmation remains an operational health check.

## Changed files

- `README.md`
- `LATENCY_COMPLETION.md`
- `cmd/0945-playbook/main.go`
- `go.mod`, `go.sum`
- `internal/dashboard/state.go`
- `internal/data/governor.go`, `governor_test.go`, `session.go`, `session_test.go`
- `internal/market/engine.go`, `sustained_test.go`
- `internal/playbook/engine.go`
- `internal/runner/streaming.go`, `streaming_live_test.go`
- `internal/server/server.go`, `hub.go`, `hub_test.go`, `browser_e2e_test.go`
- `internal/server/web/app.js`
