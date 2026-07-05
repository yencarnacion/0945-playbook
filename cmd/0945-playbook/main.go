package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"0945-playbook/internal/config"
	"0945-playbook/internal/data"
	"0945-playbook/internal/runner"
	"0945-playbook/internal/server"
	"0945-playbook/internal/watchlist"
)

const projectName = "0945-playbook"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	_ = config.LoadDotEnv(".env")

	mode := "live"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		mode = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet(mode, flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "config file")
	addr := fs.String("addr", "", "HTTP listen address override")
	date := fs.String("date", "", "YYYY-MM-DD for download/replay")
	start := fs.String("start", "", "HH:MM replay start")
	speed := fs.Float64("speed", 0, "demo speed multiplier")
	maxSymbols := fs.Int("max", 0, "max symbols override")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *addr != "" {
		cfg.App.Addr = *addr
	}
	if *maxSymbols > 0 {
		cfg.Scan.MaxSymbols = *maxSymbols
	}
	if *speed > 0 {
		cfg.Replay.Speed = *speed
	}

	loc, err := time.LoadLocation(cfg.App.Timezone)
	if err != nil {
		return err
	}

	items, err := watchlist.Load(cfg.Scan.WatchlistPath, cfg.Scan.MaxSymbols)
	if err != nil {
		return fmt.Errorf("load watchlist: %w", err)
	}
	if len(items) == 0 {
		return fmt.Errorf("watchlist is empty")
	}
	if mode == "demo" && len(items) > 80 {
		items = items[:80]
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch mode {
	case "live":
		prov, err := massiveProvider(cfg, loc)
		if err != nil {
			return err
		}
		r := runner.NewLive(projectName, cfg, loc, items, prov)
		go r.Run(ctx)
		fmt.Printf("%s live dashboard: http://localhost%s\n", projectName, displayAddr(cfg.App.Addr))
		return server.Serve(ctx, cfg.App.Addr, r)

	case "download":
		day := chooseDay(*date, cfg, loc)
		prov, err := massiveProvider(cfg, loc)
		if err != nil {
			return err
		}
		fmt.Printf("downloading %d symbols for %s into %s\n", len(items), day, cfg.Scan.DataDir)
		return runner.Download(ctx, cfg, loc, items, prov, day)

	case "replay":
		day := chooseDay(*date, cfg, loc)
		replayStart := *start
		if replayStart == "" {
			replayStart = cfg.Replay.DefaultStart
		}
		r, err := runner.NewReplay(projectName, "replay", cfg, loc, items, day, replayStart, cfg.Replay.Speed)
		if err != nil {
			return err
		}
		fmt.Printf("%s replay dashboard: http://localhost%s (%s selected %s)\n", projectName, displayAddr(cfg.App.Addr), day, replayStart)
		return server.Serve(ctx, cfg.App.Addr, r)

	case "demo":
		r, err := runner.NewDemo(projectName, cfg, loc, items)
		if err != nil {
			return err
		}
		fmt.Printf("%s demo dashboard: http://localhost%s\n", projectName, displayAddr(cfg.App.Addr))
		return server.Serve(ctx, cfg.App.Addr, r)

	default:
		return fmt.Errorf("unknown mode %q; use live, download, replay, or demo", mode)
	}
}

func massiveProvider(cfg config.Config, loc *time.Location) (data.Provider, error) {
	key := strings.TrimSpace(os.Getenv(cfg.Massive.APIKeyEnv))
	timeout := config.Duration(cfg.Massive.RequestTimeout)
	return data.NewMassiveProvider(key, cfg.Massive.SourceMultiplier, cfg.Massive.SourceTimespan, cfg.Massive.Adjusted, timeout, loc)
}

func chooseDay(flagValue string, cfg config.Config, loc *time.Location) string {
	if flagValue != "" {
		return flagValue
	}
	if cfg.Replay.DefaultDate != "" {
		return cfg.Replay.DefaultDate
	}
	return time.Now().In(loc).Format("2006-01-02")
}

func displayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	if strings.HasPrefix(addr, "127.0.0.1:") {
		return strings.TrimPrefix(addr, "127.0.0.1")
	}
	if strings.HasPrefix(addr, "localhost:") {
		return strings.TrimPrefix(addr, "localhost")
	}
	return "/" + addr
}
