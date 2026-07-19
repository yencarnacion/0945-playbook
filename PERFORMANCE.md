# 09:45 Playbook streaming latency refactor

Date: 2026-07-18. Baseline commit: `41bdf57`.

## Outcome and scope

Live mode now defaults to one Massive stocks WebSocket connection and explicit per-second aggregate subscriptions. A shared provider-neutral market engine constructs bounded minute bars once and publishes only changed symbols. REST mode remains available as `massive.mode: rest` for diagnosis and recovery. The browser uses SSE in live mode instead of fetching and rerendering the full state every second.

This repository-only test run proves backend ingest and calculation headroom, bounded queues, duplicate/out-of-order handling, C/Avg semantics, REST governor behavior, and race safety without requiring an open market. Live Massive entitlement, provider transit latency, and real browser long-task behavior must still be validated in the production account before relying on the 10-second trading SLO; the application reports stale data and suppresses stale signals rather than replacing provider timestamps.

## Verified baseline diagnosis

| Observation | Checked result at `41bdf57` |
|---|---|
| Live source | `config.yaml`: five-second REST aggregates |
| Polling | 10 seconds |
| REST concurrency / timeout | 8 / 20 seconds |
| Cold range | every symbol from current 04:00 minus seven calendar days |
| Publication barrier | one goroutine per symbol followed by a global `WaitGroup.Wait` |
| C/Avg | every historical snapshot walks backward through bars and sums 15 closes |
| Developing minute | included in the 15 values; parity test preserves this |
| Kane | `/api/state` called `kaneState`, which traversed every symbol and rebuilt all 09:25–09:30 snapshots |
| Browser | `/api/state` every second and `/api/extended` as part of refresh; keyed DOM patching existed, but full JSON still crossed the wire |
| API snapshot | copied/sorted all rows and recomputed Kane under the live runner read lock |

The lower bound for 2,000 cold REST requests at concurrency 8 is 250 request waves. At only 250 ms/provider call that is 62.5 seconds; one 20-second tail wave delays the whole global publication. Steady 10-second polling implies 200 requests/second, above the requested conservative provider ceiling. Increasing concurrency cannot remove either quota risk or the global tail barrier.

## Architecture

Before:

```text
timer -> 2,000 goroutines -> semaphore(8) -> per-symbol REST
                                      \-> merge full history -> scan
all goroutines -> global WaitGroup -> replace all rows
/api/state -> copy/sort + rebuild Kane history -> full JSON -> browser poll/render
```

After:

```text
one Massive stocks WebSocket -> decode/receipt -> bounded event queue
                                             -> per-symbol locked minute state
                                             -> changed-symbol set (coalesced scan work)
                                             -> 250 ms publisher
                                                |-> C/Avg + playbook affected symbol
                                                |-> cached Kane snapshot
                                                `-> immutable dashboard snapshot
                                                      -> SSE changed rows -> rAF DOM patch

central REST governor -> cache warm / explicit legacy REST / targeted recovery
```

There is no cross-symbol publication barrier in streaming mode. A silent symbol retains its provider event timestamp; it is never assigned a synthetic fresh market timestamp.

## Measurements

Environment: Linux amd64, Intel Xeon Platinum 8488C, Go 1.23 toolchain. Synthetic burst: 2,000 symbols, three records each plus duplicates.

| Measurement | Baseline | Streaming result |
|---|---:|---:|
| Cold request shape | 2,000 REST requests / global barrier | zero recurring REST requests |
| Refresh period | 10 s backend; 1 s full browser fetch | 250 ms changed-symbol publish |
| 6,063 record asynchronous bounded-queue ingest | not applicable to REST architecture | 4.29–6.88 ms across five runs |
| Observed ingest rate | constrained by REST | 881k–1.41M records/s |
| Queue loss in controlled direct replay | n/a | 0 |
| Duplicate records detected | n/a | 63/63 |
| Corrected burst benchmark | n/a | 7.16 ms/op; 12.4 MB/op; 32,006 allocs/op |
| Race detector | legacy tests only | `go test -race ./...` passed |
| Mock WebSocket auth/subscribe/aggregate | none | passed against loopback Gorilla server |

The absolute maximum backend queue-drain latency in the final controlled five-run burst was 6.88 ms. The first benchmark iteration reported 334,705,597 B/op because duplicate maps were over-preallocated; the corrected benchmark is 12.4 MB/op. Event-to-browser percentiles require the included SSE path to be exercised with a real browser in the target deployment; backend-only elapsed time must not be presented as browser latency.

## Rate and quota audit

| Endpoint/channel | Purpose | Entitlement | Normal startup | Steady state | Worst recovery | Ceiling | Degradation |
|---|---|---|---:|---:|---:|---:|---|
| Stocks WebSocket `A.*` | one-second aggregates | real-time stocks WS and ticker entitlement | one connection; 10 batches for 2,000 symbols at batch 200 | one connection, zero REST | idempotent resubscribe | one connection | DISCONNECTED/STALE; alerts suppressed |
| Aggregate REST | legacy mode, explicit history/recovery | stocks aggregates | local cache first; misses governed | zero in WS mode | affected symbol/window only | 80 req/s, concurrency 8 | bounded retry/backoff, DEGRADED |
| Local cache | warm state | filesystem | up to one local read/symbol | none | local reads | n/a | WARMING when absent |

The governor honors `Retry-After`, retries only 429/5xx represented as transient HTTP errors, has a strict retry limit, rate tokens, and a concurrency semaphore. The old SDK does not expose every response detail uniformly; status mapping from SDK errors remains provider-client dependent.

## Operations runbook

### Preferred pre-open start

1. Synchronize the host clock with NTP and verify `MASSIVE_API_KEY` is present.
2. Start no later than 09:15 America/New_York: `./go.sh live`.
3. Open `/api/health/latency`; require `websocket.connected=true`, expected subscription count, no drops, and a recent provider event.
4. Keep the application WARMING until required history exists. Cached same-day bars are merged after the socket connects, so live records are not intentionally missed.
5. Do not act on alerts while status is WARMING, DEGRADED, STALE, or DISCONNECTED.

### Cold start at/after 09:30

The socket connects before local cache warm. Symbols lacking 15 real eligible minute closes remain incomplete; missing minutes are not fabricated. The operator must not claim the latency SLO until health is READY. Use `massive.mode: rest` only for explicit diagnosis; it is not the normal 2,000-symbol path.

### Failure and recovery

- Disconnect: the official client reconnects and restores its recorded subscription set. Health exposes connection/reconnect state. No crossing alert is produced for duplicate aggregate identities.
- Queue full: the queue remains bounded, the drop counter increases, and health becomes DEGRADED. Restarting or targeted gap fill is required before trusting continuity.
- 429/5xx: the central governor applies capped exponential backoff with jitter and `Retry-After`; it does not fan out immediate retries.
- Provider clock or transit delay: event age is computed from provider time. At more than `scan.max_event_age`, candidates/signals are suppressed and rows are marked STALE.
- Silent symbol: display/use its true `market_event_time` and `event_age_ms`; silence is not freshness.
- Slow/disconnected browser: SSE writes are outside the market engine and cannot block ingestion. Reconnect begins with a full immutable snapshot.

## Configuration

Safe defaults are in `config.yaml`: WebSocket mode, explicit batch size 200, REST 80 req/s and concurrency 8, three retries, 10-second event age, 250 ms publication, bounded queue 8,192, SSE, and a five-minute recovery snapshot interval. API keys remain environment-only.

## Verification commands

```bash
go test ./...
go test -race ./...
GOCACHE=/tmp/0945-go-cache go vet ./...
GOCACHE=/tmp/0945-go-cache go test ./internal/market -run TestOpeningBurst2000SymbolsBoundedAndFast -count=5 -v
GOCACHE=/tmp/0945-go-cache go test ./internal/market -bench BenchmarkOpeningBurst2000 -benchtime=3x -benchmem
GOCACHE=/tmp/0945-go-cache go test ./internal/data -run TestMassiveStreamAgainstMockWebSocketServer -v -count=1
go build ./cmd/0945-playbook
./go.sh live
```

## Remaining risks and provider-dependent limitations

- A real-time Massive account and live opening burst were unavailable in the deterministic test; validate entitlement, subscription acknowledgements, actual peak traffic, and provider event delay before production use.
- The checked-in Polygon-branded SDK is deprecated and rebranded. This patch isolates it behind interfaces, but deliberately avoids an all-at-once REST migration.
- Cache warm currently uses available same-day cache files. Automated precise REST gap detection/backfill and durable prior-session summary compaction need provider error/status integration before unattended recovery can be certified.
- SSE fan-out is intentionally simple. A large number of simultaneous dashboards should use a per-client broadcast hub and a slow-client eviction policy.
- Browser virtualization and automated Chrome long-task/memory acceptance measurements are not included; keyed row updates and `requestAnimationFrame` reduce work, but must be measured on the production browser/hardware.
- Provider corporate-action adjustment parity between adjusted REST history and unadjusted live aggregates requires daily validation.

## Changed files

- `config.yaml`, `go.mod`, `go.sum`
- `cmd/0945-playbook/main.go`
- `internal/config/config.go`
- `internal/data/governor.go`, `governor_test.go`, `stream.go`, `stream_test.go`
- `internal/market/engine.go`, `engine_test.go`
- `internal/dashboard/state.go`
- `internal/playbook/engine.go`
- `internal/runner/live.go`, `streaming.go`
- `internal/server/server.go`, `internal/server/web/app.js`
- `PERFORMANCE.md`
