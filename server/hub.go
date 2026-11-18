package main

import (
	"sync"
	"time"
)

// Status is what the Pi reports about itself.
type Status struct {
	FPS      float64    `json:"fps"`
	CameraOK bool       `json:"camera_ok"`
	LastSMS  *time.Time `json:"last_sms"`
}

// Hub receives frames from the Pi and fans them out to viewers.
// Publishing never blocks: slow viewers simply skip frames.
type Hub struct {
	mu        sync.RWMutex
	latest    []byte
	ring      [][]byte
	ringMax   int
	subs      map[chan []byte]struct{}
	cols      map[*Collector]struct{}
	connected bool
	status    Status
}

func NewHub(ringMax int) *Hub {
	return &Hub{
		ringMax: ringMax,
		subs:    map[chan []byte]struct{}{},
		cols:    map[*Collector]struct{}{},
	}
}

// Publish stores a frame. Live frames are sent to viewers and clip collectors;
// non-live (pre-roll) frames only go into the ring buffer used for clips.
func (h *Hub) Publish(f []byte, live bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.ring = append(h.ring, f)
	if len(h.ring) > h.ringMax {
		h.ring = h.ring[len(h.ring)-h.ringMax:]
	}
	if !live {
		return
	}
	h.latest = f
	for ch := range h.subs {
		sendLatest(ch, f)
	}
	for c := range h.cols {
		select {
		case c.ch <- f:
		default: // collector is full
		}
	}
}

// sendLatest delivers f on a size-1 channel, replacing any stale frame.
func sendLatest(ch chan []byte, f []byte) {
	select {
	case ch <- f:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- f:
	default:
	}
}

// Subscribe returns a channel of live frames and a cancel func.
func (h *Hub) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	if h.latest != nil {
		ch <- h.latest
	}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

func (h *Hub) Viewers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

// Ring returns a copy of the recent frames (oldest first).
func (h *Hub) Ring() [][]byte {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([][]byte(nil), h.ring...)
}

func (h *Hub) SetConnected(v bool) {
	h.mu.Lock()
	h.connected = v
	if !v {
		h.status = Status{}
	}
	h.mu.Unlock()
}

func (h *Hub) SetStatus(s Status) {
	h.mu.Lock()
	h.status = s
	h.mu.Unlock()
}

// Collector gathers the next live frames (used for clip post-roll).
type Collector struct {
	ch  chan []byte
	hub *Hub
}

func (h *Hub) NewCollector(max int) *Collector {
	c := &Collector{ch: make(chan []byte, max), hub: h}
	h.mu.Lock()
	h.cols[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (c *Collector) Frames() <-chan []byte { return c.ch }

func (c *Collector) Close() {
	c.hub.mu.Lock()
	delete(c.hub.cols, c)
	c.hub.mu.Unlock()
}

// StatusView is the JSON shape served to the web app.
type StatusView struct {
	Online    bool       `json:"online"`
	Viewers   int        `json:"viewers"`
	FPS       float64    `json:"fps"`
	CameraOK  bool       `json:"camera_ok"`
	LastSMS   *time.Time `json:"last_sms"`
	LastAlert *time.Time `json:"last_alert"`
}

func (h *Hub) View(lastAlert *time.Time) StatusView {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return StatusView{
		Online:    h.connected,
		Viewers:   len(h.subs),
		FPS:       h.status.FPS,
		CameraOK:  h.status.CameraOK,
		LastSMS:   h.status.LastSMS,
		LastAlert: lastAlert,
	}
}
