package main

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// buildClip encodes pre-roll + post-roll frames into an mp4 and attaches it to the alert.
func (s *Server) buildClip(alertID int64, pre [][]byte) {
	if s.ffmpeg == "" {
		return
	}
	col := s.hub.NewCollector(s.cfg.PostFrames)
	post := make([][]byte, 0, s.cfg.PostFrames)
	timeout := time.After(time.Duration(s.cfg.PostFrames/s.cfg.FPS+6) * time.Second)
collect:
	for len(post) < s.cfg.PostFrames {
		select {
		case f := <-col.Frames():
			post = append(post, f)
		case <-timeout:
			break collect
		}
	}
	col.Close()

	frames := append(append([][]byte{}, pre...), post...)
	if len(frames) < 2 {
		return
	}
	name := "clip-" + strconv.FormatInt(alertID, 10) + "-" + randName() + ".mp4"
	tmp := filepath.Join(s.mediaDir, name+".part.mp4")
	cmd := exec.Command(s.ffmpeg, "-y", "-loglevel", "error",
		"-f", "image2pipe", "-framerate", strconv.Itoa(s.cfg.FPS), "-c:v", "mjpeg", "-i", "-",
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-movflags", "+faststart", tmp)
	cmd.Stdin = bytes.NewReader(bytes.Join(frames, nil))
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("clip %d: ffmpeg failed: %v: %s", alertID, err, out)
		_ = os.Remove(tmp)
		return
	}
	final := filepath.Join(s.mediaDir, name)
	if err := os.Rename(tmp, final); err != nil {
		log.Printf("clip %d: %v", alertID, err)
		return
	}
	url := "/media/" + name
	if err := s.store.SetClip(alertID, url); err != nil {
		log.Printf("clip %d: %v", alertID, err)
		return
	}
	if a, err := s.store.Get(alertID); err == nil {
		s.broker.Publish(map[string]any{"type": "alert_update", "alert": a})
	}
}
