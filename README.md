# 0945-playbook

A compact Go dashboard for the 09:45 playbook. It scans a CSV watchlist, computes a TC2000-style rolling `C/AVGC15` signal, applies the B1-B5 branch rules, and serves a stable real-time dashboard for likely setups and actual signals.

## Setup

1. Put your key in `.env`:

```bash
MASSIVE_API_KEY=...
```

2. Start live mode:

```bash
./go.sh live
```

3. Open `http://localhost:8080`.

Ticker links open `http://localhost:8081/?ticker=SYMBOL` for a separately running `polygon-charts` instance.

## Modes

```bash
./go.sh live
./go.sh download --date 2026-07-02
./go.sh replay --date 2026-07-02 --start 09:30
./go.sh demo
```

Live mode polls Massive aggregate bars using the official Go client dependency. The Massive module currently resolves through the legacy declared Go module path `github.com/polygon-io/client-go`, even though the module proxy marks it as rebranded to `github.com/massive-com/client-go`.

## Notes

- The default source is 5-second bars reduced into one-minute candles so the rolling `C/AVGC15` signal stays close to TC2000 while keeping the feed lighter than 1-second bars.
- Live mode anchors intraday indicators at the regular-session open. Starting after 09:30 backfills from 09:30, then each poll merges newer bars into the in-memory RTH buffer used for HOD, LOD, VWAP, first-15 volume/range filters, and rolling `C/Avg15`.
- Live mode also exposes a `C/Avg Live` tab from 04:00 through 20:00 New York time. It keeps per-minute in-memory history from process startup and alerts when a symbol newly crosses the configured rolling ratio thresholds. Browser audio must be started once from the tab.
- `KaneScan` freezes the rules researched in `local/kane.txt`: it shows every qualifying gap-down and gap-up, ranks each strategy independently, marks the leading name in each setup as preferred, and marks pre-09:30 results as preliminary. Its EV and clean-win columns are sample evidence, not ticker-specific predictions. The tab is also populated in replay.
- KaneScan includes cumulative traded volume from 04:00 through the displayed live or replay timestamp. Downloads cache data beginning at 04:00 so the value is reproducible in replay.
- KaneScan defaults to showing candidates with at least 100K volume since 04:00. The header provides an immediately editable numeric threshold plus ALL, 100K, 250K, and 500K presets; KaneScan sound alerts respect this filter.
- Preferred markers are selected from the currently visible candidates: the highest research-ranked name remaining in each strategy becomes `PREFERRED ON SCREEN` when the volume threshold changes.
- KaneScan freezes reproducible snapshots at 09:25, 09:26, 09:27, 09:28, 09:29, and 09:30. Timestamp buttons revisit them without stopping the current scan; `LIVE` returns to the continuously updating post-09:30 view.
- During replay, KaneScan's `+1 MIN` button advances the replay clock one minute, returns to the current view, and refreshes candidates immediately.
- Replay also exposes the C/Avg tab at the selected minute. Its `+1 MIN` control advances and recalculates the scan, while opening gap, gap/ATR, prior-close position, and Kane setup fit provide additional decision context.
- The C/Avg industry panel breaks each industry's matches into green up-threshold and orange down-threshold counts, alongside the total.
- Clicking an industry filters the C/Avg candidate table to that group; clicking the highlighted industry again clears the filter while preserving the full-panel counts.
- Sound is opt-in and independently controllable on `C/Avg Live` and `KaneScan`; `hey.mp3` plays only when a new symbol enters an enabled scan.
- Configure the extended scanner under `extended_scan`. `avg_close_bars` controls its rolling length independently of the 09:45 playbook, and `sound_dir` plus `sound_file` select the alert audio (default `sounds/hey.mp3`).
- `scan.max_symbols: 0` means use the entire CSV watchlist, regardless of the file name.
- `scan.min_first15_volume_filter` controls the dashboard volume filter; default is `400000`.
- Replay is manually controlled from the dashboard. Use the arrow buttons to step one minute at a time, or enter a specific time to hold that replay timestamp.
- Chart links use `polygon-charts` deep links with the dashboard's current minute, for example `/api/open-chart/AAPL/2026-07-02/0933`.
- The lower table defaults to signal-relevant rows only. The enlarged top strip keeps likely, active, and completed signal rows visible by EV score.
- Replay uses cached JSON under `data/YYYY-MM-DD/`; run `download` before replaying a market day.
