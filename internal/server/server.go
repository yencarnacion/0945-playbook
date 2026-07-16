package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"0945-playbook/internal/dashboard"
)

//go:embed web/*
var webFS embed.FS

type StateProvider interface {
	Snapshot(context.Context) dashboard.State
}

type ReplayController interface {
	SeekReplay(context.Context, string) (dashboard.State, error)
	StepReplay(context.Context, int) (dashboard.State, error)
}

type ExtendedProvider interface {
	ExtendedSnapshot(context.Context, int64) dashboard.ExtendedState
	AlertSoundPath() string
}

func Serve(ctx context.Context, addr string, provider StateProvider) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		state := provider.Snapshot(r.Context())
		writeJSON(w, state)
	})
	mux.HandleFunc("/api/extended", func(w http.ResponseWriter, r *http.Request) {
		extended, ok := provider.(ExtendedProvider)
		if !ok {
			http.Error(w, "extended live scan unavailable", http.StatusNotFound)
			return
		}
		var selectedID int64
		if raw := r.URL.Query().Get("minute"); raw != "" {
			var err error
			selectedID, err = strconv.ParseInt(raw, 10, 64)
			if err != nil {
				http.Error(w, "invalid minute", http.StatusBadRequest)
				return
			}
		}
		writeJSON(w, extended.ExtendedSnapshot(r.Context(), selectedID))
	})
	mux.HandleFunc("/api/extended/sound", func(w http.ResponseWriter, r *http.Request) {
		extended, ok := provider.(ExtendedProvider)
		if !ok {
			http.Error(w, "extended live scan unavailable", http.StatusNotFound)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.ServeFile(w, r, extended.AlertSoundPath())
	})
	mux.HandleFunc("/api/replay/seek", func(w http.ResponseWriter, r *http.Request) {
		controller, ok := provider.(ReplayController)
		if !ok {
			http.Error(w, "replay controls unavailable", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Clock string `json:"clock"`
			Time  string `json:"time"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		clock := req.Clock
		if clock == "" {
			clock = req.Time
		}
		state, err := controller.SeekReplay(r.Context(), clock)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, state)
	})
	mux.HandleFunc("/api/replay/step", func(w http.ResponseWriter, r *http.Request) {
		controller, ok := provider.(ReplayController)
		if !ok {
			http.Error(w, "replay controls unavailable", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Minutes int `json:"minutes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		state, err := controller.StepReplay(r.Context(), req.Minutes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, state)
	})

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
