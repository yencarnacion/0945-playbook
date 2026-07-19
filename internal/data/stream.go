package data

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	polygonws "github.com/polygon-io/client-go/websocket"
	"github.com/polygon-io/client-go/websocket/models"
)

type StreamHealth struct {
	Connected     bool      `json:"connected"`
	Subscriptions int       `json:"subscriptions"`
	Reconnects    uint64    `json:"reconnects"`
	LastMessage   time.Time `json:"last_message"`
	Error         string    `json:"error,omitempty"`
}
type StreamEvent struct {
	Symbol                                                  string
	Start, End                                              time.Time
	Open, High, Low, Close, Volume, VWAP, AccumulatedVolume float64
	Received                                                time.Time
}
type StreamProvider interface {
	Connect(context.Context) error
	Subscribe([]string) error
	Unsubscribe([]string) error
	Events() <-chan StreamEvent
	Health() StreamHealth
	Close() error
}

type wsClient interface {
	Connect() error
	Subscribe(polygonws.Topic, ...string) error
	Unsubscribe(polygonws.Topic, ...string) error
	Output() <-chan any
	Error() <-chan error
	Close()
}

type MassiveStream struct {
	client        wsClient
	events        chan StreamEvent
	batch         int
	connected     atomic.Bool
	subscriptions atomic.Int64
	reconnects    atomic.Uint64
	lastMessage   atomic.Int64
	lastErr       atomic.Value
}

func NewMassiveStream(apiKey, websocketURL string, batch int) (*MassiveStream, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("missing Massive API key")
	}
	if batch < 1 {
		batch = 200
	}
	feed := polygonws.RealTime
	if websocketURL != "" {
		feed = polygonws.Feed(websocketURL)
	}
	var stream *MassiveStream
	c, err := polygonws.New(polygonws.Config{APIKey: apiKey, Feed: feed, Market: polygonws.Stocks, ReconnectCallback: func(error) {
		if stream != nil {
			stream.reconnects.Add(1)
		}
	}})
	if err != nil {
		return nil, err
	}
	stream = newMassiveStream(c, batch)
	return stream, nil
}
func newMassiveStream(c wsClient, batch int) *MassiveStream {
	return &MassiveStream{client: c, events: make(chan StreamEvent, 8192), batch: batch}
}
func (s *MassiveStream) Subscribe(symbols []string) error {
	for i := 0; i < len(symbols); i += s.batch {
		end := i + s.batch
		if end > len(symbols) {
			end = len(symbols)
		}
		if err := s.client.Subscribe(polygonws.StocksSecAggs, symbols[i:end]...); err != nil {
			return err
		}
		s.subscriptions.Add(int64(end - i))
	}
	return nil
}
func (s *MassiveStream) Unsubscribe(symbols []string) error {
	if err := s.client.Unsubscribe(polygonws.StocksSecAggs, symbols...); err != nil {
		return err
	}
	s.subscriptions.Add(-int64(len(symbols)))
	return nil
}
func (s *MassiveStream) Connect(ctx context.Context) error {
	if err := s.client.Connect(); err != nil {
		return err
	}
	s.connected.Store(true)
	go s.read(ctx)
	return nil
}
func (s *MassiveStream) read(ctx context.Context) {
	defer s.connected.Store(false)
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-s.client.Error():
			if err != nil {
				s.lastErr.Store(err.Error())
			}
			return
		case raw, ok := <-s.client.Output():
			if !ok {
				return
			}
			now := time.Now()
			s.lastMessage.Store(now.UnixNano())
			a, ok := raw.(models.EquityAgg)
			if !ok {
				continue
			}
			ev := StreamEvent{Symbol: a.Symbol, Start: time.UnixMilli(a.StartTimestamp), End: time.UnixMilli(a.EndTimestamp), Open: a.Open, High: a.High, Low: a.Low, Close: a.Close, Volume: a.Volume, VWAP: a.VWAP, AccumulatedVolume: a.AccumulatedVolume, Received: now}
			select {
			case s.events <- ev:
			case <-ctx.Done():
				return
			}
		}
	}
}
func (s *MassiveStream) Events() <-chan StreamEvent { return s.events }
func (s *MassiveStream) Health() StreamHealth {
	h := StreamHealth{Connected: s.connected.Load(), Subscriptions: int(s.subscriptions.Load()), Reconnects: s.reconnects.Load()}
	if n := s.lastMessage.Load(); n > 0 {
		h.LastMessage = time.Unix(0, n)
	}
	if v := s.lastErr.Load(); v != nil {
		h.Error = v.(string)
	}
	return h
}
func (s *MassiveStream) Close() error { s.client.Close(); return nil }
