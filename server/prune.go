package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Alerts younger than this are never pruned: their clip may still be encoding.
	pruneMinAge = 2 * time.Minute
	// Files in the media dir that no alert references are removed once this old.
	orphanMinAge = time.Hour
)

// mediaFiles returns the media file names an alert points at.
func mediaFiles(a Alert) []string {
	var out []string
	for _, u := range []string{a.Snapshot, ptrString(a.Clip)} {
		if u != "" {
			out = append(out, filepath.Base(strings.TrimPrefix(u, "/media/")))
		}
	}
	return out
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func fileSize(path string) int64 {
	if st, err := os.Stat(path); err == nil {
		return st.Size()
	}
	return 0
}

func (s *Server) alertSize(a Alert) int64 {
	var n int64
	for _, f := range mediaFiles(a) {
		n += fileSize(filepath.Join(s.mediaDir, f))
	}
	return n
}

// prune deletes alerts (and their files) older than RetentionDays, then the oldest alerts
// until the media directory is under MaxMediaBytes. It also removes stray files.
// It returns the number of alerts removed.
func (s *Server) prune(now time.Time) int {
	alerts := s.store.Oldest()
	drop := map[int64]Alert{}

	if d := s.cfg.RetentionDays; d > 0 {
		cutoff := now.AddDate(0, 0, -d)
		for _, a := range alerts {
			if a.TS.Before(cutoff) {
				drop[a.ID] = a
			}
		}
	}

	if max := s.cfg.MaxMediaBytes; max > 0 {
		var total int64
		entries, _ := os.ReadDir(s.mediaDir)
		for _, e := range entries {
			if info, err := e.Info(); err == nil && !e.IsDir() {
				total += info.Size()
			}
		}
		for _, a := range drop {
			total -= s.alertSize(a)
		}
		for _, a := range alerts { // oldest first
			if total <= max {
				break
			}
			if _, gone := drop[a.ID]; gone || now.Sub(a.TS) < pruneMinAge {
				continue
			}
			total -= s.alertSize(a)
			drop[a.ID] = a
		}
	}

	ids := map[int64]bool{}
	for id := range drop {
		ids[id] = true
	}
	if err := s.store.Remove(ids); err != nil {
		log.Printf("prune: %v", err)
		return 0
	}
	for _, a := range drop {
		for _, f := range mediaFiles(a) {
			_ = os.Remove(filepath.Join(s.mediaDir, f))
		}
	}
	if len(ids) > 0 {
		list := make([]int64, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		s.broker.Publish(map[string]any{"type": "alert_removed", "ids": list})
		log.Printf("prune: removed %d old alert(s)", len(ids))
	}
	s.removeOrphans(now)
	return len(ids)
}

// removeOrphans deletes media files no alert refers to (e.g. a clip whose encode failed).
func (s *Server) removeOrphans(now time.Time) {
	referenced := map[string]bool{}
	for _, a := range s.store.Oldest() {
		for _, f := range mediaFiles(a) {
			referenced[f] = true
		}
	}
	entries, _ := os.ReadDir(s.mediaDir)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() || referenced[e.Name()] || now.Sub(info.ModTime()) < orphanMinAge {
			continue
		}
		_ = os.Remove(filepath.Join(s.mediaDir, e.Name()))
	}
}
