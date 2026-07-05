# 0945-playbook

A compact Go dashboard for the 09:45 first-15 playbook. It scans a CSV watchlist, computes the TradingView-style first-15 average close signal, applies the B1-B5 branch rules, and serves a stable real-time dashboard for likely setups and actual signals.

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

- The default source is 5-second bars reduced into one-minute candles so the first-15 signal matches the Pine logic while keeping the feed lighter than 1-second bars.
- `scan.max_symbols: 0` means use the entire CSV watchlist, regardless of the file name.
- `scan.min_first15_volume_filter` controls the dashboard volume filter; default is `400000`.
- Replay runs at 1x by default, so one replay second advances one market second.
- Replay chart links use `polygon-charts` deep links like `/api/open-chart/AAPL/2026-07-02/0945`.
- The lower table defaults to signal-relevant rows only. The enlarged top strip keeps likely, active, and completed signal rows visible by EV score.
- Replay uses cached JSON under `data/YYYY-MM-DD/`; run `download` before replaying a market day.
