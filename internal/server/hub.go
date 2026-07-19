package server

import (
	"sync"
	"sync/atomic"

	"0945-playbook/internal/dashboard"
)

type HubStats struct {
	Connected       int    `json:"connected"`
	MaxQueueDepth   int    `json:"max_queue_depth"`
	SlowDisconnects uint64 `json:"slow_disconnects"`
	Resyncs         uint64 `json:"resyncs"`
	Broadcasts      uint64 `json:"broadcasts"`
}
type Client struct {
	id     uint64
	C      chan dashboard.Delta
	hub    *Hub
	once   sync.Once
	resync atomic.Bool
}

func (c *Client) Close() { c.once.Do(func() { c.hub.remove(c.id) }) }

type Hub struct {
	mu                      sync.RWMutex
	clients                 map[uint64]*Client
	next                    atomic.Uint64
	queueSize               int
	slow, resync, broadcast atomic.Uint64
}

func NewHub(queueSize int) *Hub {
	if queueSize < 1 {
		queueSize = 64
	}
	return &Hub{clients: make(map[uint64]*Client), queueSize: queueSize}
}
func (h *Hub) Subscribe() *Client {
	id := h.next.Add(1)
	c := &Client{id: id, C: make(chan dashboard.Delta, h.queueSize), hub: h}
	h.mu.Lock()
	h.clients[id] = c
	h.mu.Unlock()
	return c
}
func (h *Hub) remove(id uint64) {
	h.mu.Lock()
	if c := h.clients[id]; c != nil {
		delete(h.clients, id)
		close(c.C)
	}
	h.mu.Unlock()
}
func (h *Hub) Publish(d dashboard.Delta) {
	h.broadcast.Add(1)
	h.mu.RLock()
	slow := make([]uint64, 0)
	for id, c := range h.clients {
		select {
		case c.C <- d:
		default:
			if c.resync.CompareAndSwap(false, true) {
				h.resync.Add(1)
				select {
				case <-c.C:
				default:
				}
				msg := dashboard.Delta{ProtocolVersion: 1, Type: "resync_required", Reason: "client_queue_overflow"}
				select {
				case c.C <- msg:
				default:
				}
				slow = append(slow, id)
			}
		}
	}
	h.mu.RUnlock()
	for _, id := range slow {
		h.remove(id)
	}
	if len(slow) > 0 {
		h.slow.Add(uint64(len(slow)))
	}
}
func (h *Hub) Stats() HubStats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	max := 0
	for _, c := range h.clients {
		if len(c.C) > max {
			max = len(c.C)
		}
	}
	return HubStats{Connected: len(h.clients), MaxQueueDepth: max, SlowDisconnects: h.slow.Load(), Resyncs: h.resync.Load(), Broadcasts: h.broadcast.Load()}
}
