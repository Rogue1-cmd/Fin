package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Alert struct {
	ID         int64     `json:"id"`
	TS         time.Time `json:"ts"`
	Confidence float64   `json:"confidence"`
	Snapshot   string    `json:"snapshot"`       // URL path
	Clip       *string   `json:"clip,omitempty"` // URL path, set once encoded
}

// Store keeps alerts in memory and persists them to a JSON file.
// Alerts arrive a few times a day at most, so a database would be overkill.
type Store struct {
	mu     sync.Mutex
	path   string
	alerts []Alert // oldest first
	nextID int64
}

func OpenStore(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "alerts.json"), nextID: 1}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.alerts); err != nil {
		return nil, err
	}
	for _, a := range s.alerts {
		if a.ID >= s.nextID {
			s.nextID = a.ID + 1
		}
	}
	return s, nil
}

func (s *Store) Close() error { return nil }

// save writes atomically (temp file + rename). Caller holds s.mu.
func (s *Store) save() error {
	b, err := json.MarshalIndent(s.alerts, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Insert(ts time.Time, conf float64, snapshot string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := Alert{ID: s.nextID, TS: ts.UTC(), Confidence: conf, Snapshot: snapshot}
	s.alerts = append(s.alerts, a)
	if err := s.save(); err != nil {
		s.alerts = s.alerts[:len(s.alerts)-1]
		return 0, err
	}
	s.nextID++
	return a.ID, nil
}

func (s *Store) SetClip(id int64, clip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.alerts {
		if s.alerts[i].ID == id {
			s.alerts[i].Clip = &clip
			return s.save()
		}
	}
	return errors.New("alert not found")
}

// List returns up to limit alerts, newest first.
func (s *Store) List(limit int) ([]Alert, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Alert{}
	for i := len(s.alerts) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.alerts[i])
	}
	return out, nil
}

func (s *Store) Get(id int64) (Alert, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.alerts {
		if a.ID == id {
			return a, nil
		}
	}
	return Alert{}, errors.New("alert not found")
}

// LastAlertTime returns the newest alert time, or nil.
func (s *Store) LastAlertTime() *time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.alerts) == 0 {
		return nil
	}
	t := s.alerts[len(s.alerts)-1].TS
	return &t
}

// Oldest returns a copy of all alerts, oldest first.
func (s *Store) Oldest() []Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Alert(nil), s.alerts...)
}

// Remove deletes the given alert IDs and persists the change once.
func (s *Store) Remove(ids map[int64]bool) error {
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.alerts[:0:0]
	for _, a := range s.alerts {
		if !ids[a.ID] {
			kept = append(kept, a)
		}
	}
	s.alerts = kept
	return s.save()
}
