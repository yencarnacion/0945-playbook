package data

import (
	"context"
	polygonws "github.com/polygon-io/client-go/websocket"
	"github.com/polygon-io/client-go/websocket/models"
	"testing"
	"time"
)

type mockWS struct {
	subs [][]string
	out  chan any
	errs chan error
}

func (m *mockWS) Connect() error { return nil }
func (m *mockWS) Subscribe(_ polygonws.Topic, s ...string) error {
	m.subs = append(m.subs, append([]string(nil), s...))
	return nil
}
func (m *mockWS) Unsubscribe(polygonws.Topic, ...string) error { return nil }
func (m *mockWS) Output() <-chan any                           { return m.out }
func (m *mockWS) Error() <-chan error                          { return m.errs }
func (m *mockWS) Close()                                       {}
func TestMassiveStreamChunksSubscriptionsAndRecordsReceipt(t *testing.T) {
	m := &mockWS{out: make(chan any, 1), errs: make(chan error, 1)}
	s := newMassiveStream(m, 2)
	if err := s.Subscribe([]string{"A", "B", "C", "D", "E"}); err != nil {
		t.Fatal(err)
	}
	if len(m.subs) != 3 || len(m.subs[2]) != 1 {
		t.Fatalf("batches=%v", m.subs)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	m.out <- models.EquityAgg{Symbol: "A", Open: 1, High: 2, Low: 1, Close: 2, StartTimestamp: time.Now().UnixMilli(), EndTimestamp: time.Now().UnixMilli()}
	select {
	case ev := <-s.Events():
		if ev.Received.IsZero() || ev.Symbol != "A" {
			t.Fatalf("event %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("event timeout")
	}
}
