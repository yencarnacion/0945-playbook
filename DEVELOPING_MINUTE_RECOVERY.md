# Developing-minute recovery audit

Audit date: 2026-07-19
Starting HEAD: `bc5bf989da043dc3c2a9acca7e31cb9c470131ae`

## Verification

| Concern | Result | Evidence |
|---|---|---|
| A partial REST bar could freeze the developing minute | Confirmed | `ApplyAuthoritativeBars` marked every returned minute in `authoritative`, including the current minute |
| Later WebSocket seconds could update a recovered current minute | Confirmed defect | `Engine.apply` rejects all events for an authoritative minute |
| Gap remained until current-minute continuity was proven | Partially fixed | version checks were correct, but a single current-minute REST record could clear the gap |
| Completed recovery recomputed engine-derived OHLCV/VWAP state | Already fixed | `recomputeDerivedLocked` runs after replacement |
| Downstream scans are republished after recovery | Already fixed | successful runner recovery dirties the symbol; publication reevaluates playbook, C/Avg, and Kane |

## Implemented semantics

Massive's minute aggregate REST response does not provide a per-second coverage watermark. The implementation therefore uses completed-minute recovery only:

1. Recovery calculates the minute currently developing at the supplied/injected clock.
2. Only minutes strictly before that boundary may become authoritative.
3. A REST record for the developing minute is ignored; it is never merged or marked authoritative.
4. The symbol remains `RECOVERING`, so every dependent alert remains suppressed.
5. Recovery records the next minute boundary, preventing every later second from starting another REST request.
6. Later WebSocket seconds continue to update close, high, low, volume, and VWAP normally.
7. At minute close, the next recovery fetch treats that minute as completed, verifies coverage, replaces the local bar atomically, recomputes derived state, and clears only the current gap version.
8. A mixed completed/developing gap commits only its completed prefix. The unresolved suffix gets a new version, invalidating the older recovery token.

## Source precedence

| Source/event | Precedence |
|---|---|
| Verified completed REST minute | Replaces the incomplete local minute atomically and becomes immutable |
| Partial REST developing minute | Ignored because no precise coverage watermark exists |
| WebSocket second newer than partial recovery | Continues merging into the provisional developing minute |
| Duplicate WebSocket second | Rejected by the timestamp-pair duplicate key |
| Out-of-order live second before finalization | May update OHLC/volume under existing aggregate semantics; counted as out of order |
| Late live event after completed authority | Rejected; cannot corrupt or double-count the completed minute |
| Older recovery version | Rejected before any mutation |
| Corrected completed REST response | Requires a new explicit gap/version; a consumed token is not reusable |

An authoritative replacement rebuilds session volume, premarket volume, high, low, VWAP inputs, and current price. C/Avg reads the replaced rolling window on the next dirty-symbol publication. The same dirty publication reevaluates original playbook and Kane rows before their generations advance.

## Deterministic sequence tested

`TestDevelopingMinuteRecoveryNeverFreezesLiveUpdates` covers:

```text
09:30:08 event lost
09:30:09 recovery begins
09:30:10 partial minute returned and rejected for authority
09:30:11 and 09:30:59 live seconds continue updating OHLCV
09:31:00 minute becomes completed
09:31:01 completed minute replaces the provisional bar
late 09:30 event is rejected after finalization
```

It asserts that partial REST volume is not double-counted, the gap remains visible, and late delivery cannot overwrite the final minute. `TestAuthoritativeReplacementRecomputesCAVGWindow` verifies the rolling average and ratio change after replacement. Existing runner tests verify gap alert suppression and dirty-symbol scan publication.

## Measurements

```text
go test ./...                 PASS
go test -race ./...           PASS
go vet ./...                  PASS
node --check internal/server/web/app.js PASS
```

Production queue soak (`2,000` symbols, `6,000` events/second, `8,192` capacity):

```text
events=30000 elapsed=5.000s rate=6000/s dropped=0 ending_queue=0
heap_before=8,095,920 heap_after=9,344,432 total_alloc=1,728,688 goroutines=3
```

Opening-burst benchmark, 6,000 events per operation:

```text
5.56–5.67 ms/op
12.27–12.56 MB/op
22,006–24,006 allocs/op
```

## Remaining limitation

A legitimate no-trade or halted interval cannot be proven complete from a partial current-minute response. It remains recovering until a completed authoritative response or provider-supported continuity evidence is available. This deliberately favors suppressed alerts over false readiness.
