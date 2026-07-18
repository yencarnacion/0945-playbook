package watchlist

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type Item struct {
	Symbol   string  `json:"symbol"`
	Name     string  `json:"name"`
	Industry string  `json:"industry"`
	Order    int     `json:"order"`
	ATR14    float64 `json:"atr14"`
}

func number(value string) float64 {
	value = strings.TrimSpace(strings.Trim(value, "$%"))
	v, _ := strconv.ParseFloat(value, 64)
	return v
}

func Load(path string, max int) ([]Item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return nil, err
	}

	index := headerIndex(header)
	if _, ok := index["symbol"]; !ok {
		item := itemFromRecord(header, nil, 0)
		if item.Symbol == "" {
			return nil, fmt.Errorf("watchlist %s does not contain a Symbol column", path)
		}
		items := []Item{item}
		return appendRecords(r, nil, items, max)
	}

	return appendRecords(r, index, nil, max)
}

func appendRecords(r *csv.Reader, index map[string]int, items []Item, max int) ([]Item, error) {
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Symbol] = true
	}

	underLimit := func() bool {
		return max <= 0 || len(items) < max
	}

	for underLimit() {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		item := itemFromRecord(rec, index, len(items))
		if item.Symbol == "" || seen[item.Symbol] {
			continue
		}
		seen[item.Symbol] = true
		items = append(items, item)
	}
	return items, nil
}

func itemFromRecord(rec []string, index map[string]int, order int) Item {
	get := func(name string, fallback int) string {
		i, ok := index[name]
		if !ok {
			i = fallback
		}
		if i < 0 || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}

	symbol := get("symbol", 0)
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	symbol = strings.TrimPrefix(symbol, "$")
	if symbol == "" || strings.EqualFold(symbol, "symbol") {
		return Item{}
	}

	return Item{
		Symbol:   symbol,
		Name:     get("name", -1),
		Industry: get("industry", -1),
		Order:    order,
		ATR14:    number(get("atr - 14 days", -1)),
	}
}

func headerIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		idx[key] = i
	}
	return idx
}
