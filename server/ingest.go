package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/coder/websocket"
)

// Binary message layout from the Pi: 1 flag byte + JPEG.
const (
	flagLive    = 1 // broadcast to viewers
	flagPreroll = 2 // only kept for clip pre-roll
)

func randName() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func isJPEG(b []byte) bool { return len(b) > 4 && b[0] == 0xFF && b[1] == 0xD8 }

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("X-Device-Key")
	if subtle.ConstantTimeCompare([]byte(key), []byte(s.cfg.DeviceKey)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(8 << 20)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Only one Pi at a time: a new connection replaces the old one.
	s.ingestMu.Lock()
	if s.ingestCancel != nil {
		s.ingestCancel()
	}
	s.ingestGen++
	gen := s.ingestGen
	s.ingestCancel = cancel
	s.ingestMu.Unlock()

	s.hub.SetConnected(true)
	s.publishStatus()
	log.Printf("ingest: device connected from %s", r.RemoteAddr)
	defer func() {
		s.ingestMu.Lock()
		current := s.ingestGen == gen
		s.ingestMu.Unlock()
		if current {
			s.hub.SetConnected(false)
			s.publishStatus()
		}
		log.Printf("ingest: device disconnected")
	}()

	// Tell the Pi how many people are watching so it can skip uploads when nobody is.
	go func() {
		last := -1
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			if n := s.hub.Viewers(); n != last {
				msg, _ := json.Marshal(map[string]int{"viewers": n})
				if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
					return
				}
				last = n
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()

	for {
		rctx, rcancel := context.WithTimeout(ctx, 30*time.Second)
		typ, data, err := conn.Read(rctx)
		rcancel()
		if err != nil {
			return
		}
		switch typ {
		case websocket.MessageBinary:
			if len(data) < 2 || !isJPEG(data[1:]) {
				continue
			}
			s.hub.Publish(data[1:], data[0] == flagLive)
		case websocket.MessageText:
			s.handleDeviceMessage(data)
		}
	}
}

func (s *Server) handleDeviceMessage(data []byte) {
	var m struct {
		Type       string     `json:"type"`
		Confidence float64    `json:"confidence"`
		Snapshot   string     `json:"snapshot"`
		FPS        float64    `json:"fps"`
		CameraOK   bool       `json:"camera_ok"`
		LastSMS    *time.Time `json:"last_sms"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	switch m.Type {
	case "status":
		s.hub.SetStatus(Status{FPS: m.FPS, CameraOK: m.CameraOK, LastSMS: m.LastSMS})
		s.publishStatus()
	case "alert":
		s.recordAlert(m.Confidence, m.Snapshot)
	}
}

func (s *Server) recordAlert(conf float64, snapshotB64 string) {
	jpg, err := base64.StdEncoding.DecodeString(snapshotB64)
	if err != nil || !isJPEG(jpg) {
		log.Printf("alert: bad snapshot")
		return
	}
	name := "alert-" + time.Now().UTC().Format("20060102-150405") + "-" + randName() + ".jpg"
	if err := os.WriteFile(filepath.Join(s.mediaDir, name), jpg, 0o644); err != nil {
		log.Printf("alert: save snapshot: %v", err)
		return
	}
	id, err := s.store.Insert(time.Now(), conf, "/media/"+name)
	if err != nil {
		log.Printf("alert: store: %v", err)
		return
	}
	if a, err := s.store.Get(id); err == nil {
		s.broker.Publish(map[string]any{"type": "alert", "alert": a})
	}
	s.publishStatus()
	pre := s.hub.Ring()
	go s.buildClip(id, pre)
}
