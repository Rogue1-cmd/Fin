package main

import (
	"fmt"
	"net/http"
	"time"
)

// handleStream serves the live feed as MJPEG (multipart/x-mixed-replace).
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	w.Header().Set("Cache-Control", "no-store")
	rc := http.NewResponseController(w)

	ch, cancel := s.hub.Subscribe()
	defer cancel()

	for {
		select {
		case <-r.Context().Done():
			return
		case f := <-ch:
			// A stuck client must not hold a goroutine forever.
			_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := fmt.Fprintf(w, "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(f)); err != nil {
				return
			}
			if _, err := w.Write(f); err != nil {
				return
			}
			if _, err := w.Write([]byte("\r\n")); err != nil {
				return
			}
			fl.Flush()
		}
	}
}
