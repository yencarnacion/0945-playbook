package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CacheFile struct {
	Symbol     string    `json:"symbol"`
	Source     string    `json:"source"`
	Day        string    `json:"day"`
	CreatedAt  time.Time `json:"created_at"`
	Bars       []Bar     `json:"bars"`
	FetchStart time.Time `json:"fetch_start,omitempty"`
}

func SaveBars(dataDir, day, symbol string, bars []Bar) error {
	return SaveBarsFrom(dataDir, day, symbol, time.Time{}, bars)
}

func SaveBarsFrom(dataDir, day, symbol string, fetchStart time.Time, bars []Bar) error {
	path := CachePath(dataDir, day, symbol)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload := CacheFile{
		Symbol:     strings.ToUpper(symbol),
		Source:     "massive",
		Day:        day,
		CreatedAt:  time.Now().UTC(),
		Bars:       bars,
		FetchStart: fetchStart,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func LoadBarsCoverage(dataDir, day, symbol string) ([]Bar, time.Time, error) {
	b, err := os.ReadFile(CachePath(dataDir, day, symbol))
	if err != nil {
		return nil, time.Time{}, err
	}
	var payload CacheFile
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, time.Time{}, err
	}
	return payload.Bars, payload.FetchStart, nil
}

func LoadBars(dataDir, day, symbol string) ([]Bar, error) {
	b, err := os.ReadFile(CachePath(dataDir, day, symbol))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, err
	}
	var payload CacheFile
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, err
	}
	return payload.Bars, nil
}

func CachePath(dataDir, day, symbol string) string {
	name := strings.ToUpper(strings.TrimSpace(symbol)) + ".json"
	return filepath.Join(dataDir, day, name)
}

func MissingCacheError(day, symbol string) error {
	return fmt.Errorf("no cached bars for %s on %s; run download first", symbol, day)
}
