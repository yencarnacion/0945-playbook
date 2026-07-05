package runner

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"0945-playbook/internal/config"
	"0945-playbook/internal/data"
	"0945-playbook/internal/watchlist"
)

func Download(ctx context.Context, cfg config.Config, loc *time.Location, items []watchlist.Item, prov data.Provider, day string) error {
	start, err := parseDayClock(day, cfg.Session.Open, loc)
	if err != nil {
		return err
	}
	end, err := parseDayClock(day, cfg.Session.Close, loc)
	if err != nil {
		return err
	}

	var done atomic.Int64
	var failed atomic.Int64
	var skipped atomic.Int64
	sem := make(chan struct{}, cfg.Massive.ConcurrentRequests)
	var wg sync.WaitGroup
	errCh := make(chan error, len(items))

	for _, item := range items {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}

			if cached, err := data.LoadBars(cfg.Scan.DataDir, day, item.Symbol); err == nil && len(cached) > 0 {
				n := done.Add(1)
				skipped.Add(1)
				if n == 1 || n%25 == 0 || int(n) == len(items) {
					fmt.Printf("ready %d/%d symbols (%d cached)\n", n, len(items), skipped.Load())
				}
				return
			}

			bars, err := prov.FetchBars(ctx, item.Symbol, start, end)
			if err != nil {
				failed.Add(1)
				errCh <- fmt.Errorf("%s: %w", item.Symbol, err)
				return
			}
			if err := data.SaveBars(cfg.Scan.DataDir, day, item.Symbol, bars); err != nil {
				failed.Add(1)
				errCh <- fmt.Errorf("%s save: %w", item.Symbol, err)
				return
			}
			n := done.Add(1)
			if n == 1 || n%25 == 0 || int(n) == len(items) {
				fmt.Printf("ready %d/%d symbols (%d cached)\n", n, len(items), skipped.Load())
			}
		}()
	}
	wg.Wait()
	close(errCh)

	var firstErr error
	for err := range errCh {
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("download completed with %d failures; first error: %w", failed.Load(), firstErr)
	}
	return nil
}
