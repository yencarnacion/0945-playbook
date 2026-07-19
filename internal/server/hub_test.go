package server

import (
	"0945-playbook/internal/dashboard"
	"testing"
	"time"
)

func TestHubBroadcastsToTenClients(t *testing.T) {
	h := NewHub(4)
	cs := make([]*Client, 10)
	for i := range cs {
		cs[i] = h.Subscribe()
		defer cs[i].Close()
	}
	d := dashboard.Delta{Type: "delta", PlaybookGeneration: 1}
	h.Publish(d)
	for i, c := range cs {
		select {
		case got := <-c.C:
			if got.PlaybookGeneration != 1 {
				t.Fatalf("client %d got %+v", i, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("client %d missed delta", i)
		}
	}
}
func TestHubSlowClientRequiresResyncWithoutBlocking(t *testing.T) {
	h := NewHub(1)
	slow := h.Subscribe()
	defer slow.Close()
	fast := h.Subscribe()
	defer fast.Close()
	h.Publish(dashboard.Delta{Type: "delta", PlaybookGeneration: 1})
	<-fast.C
	done := make(chan struct{})
	go func() { h.Publish(dashboard.Delta{Type: "delta", PlaybookGeneration: 2}); close(done) }()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("slow client blocked publisher")
	}
	got := <-slow.C
	if got.Type != "resync_required" {
		t.Fatalf("slow client got %+v", got)
	}
	if h.Stats().Resyncs != 1 {
		t.Fatalf("stats %+v", h.Stats())
	}
}
func TestHubDisconnectRemovesClient(t *testing.T) {
	h := NewHub(1)
	c := h.Subscribe()
	c.Close()
	if h.Stats().Connected != 0 {
		t.Fatalf("stats %+v", h.Stats())
	}
}
