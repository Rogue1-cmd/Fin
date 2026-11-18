package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/bcrypt"
)

const (
	testUser = "admin"
	testPass = "correct horse battery"
	testKey  = "device-key-0123456789"
)

func testJPEG(t testing.TB, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for x := 0; x < 64; x++ {
		for y := 0; y < 48; y++ {
			img.Set(x, y, color.RGBA{shade, uint8(x * 4), uint8(y * 5), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testPass), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		DataDir: t.TempDir(), AdminUser: testUser, AdminHash: string(hash),
		JWTSecret: strings.Repeat("s", 40), DeviceKey: testKey,
		Secure: false, FPS: 8, PreFrames: 4, PostFrames: 4,
	}
	s, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Routes())
	t.Cleanup(ts.Close)
	return s, ts
}

func loggedInClient(t *testing.T, base string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	body, _ := json.Marshal(map[string]string{"username": testUser, "password": testPass})
	res, err := c.Post(base+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("login failed: %v %v", err, res)
	}
	res.Body.Close()
	return c
}

// ---- hub ----

func TestHubSubscriberPrimedWithLatest(t *testing.T) {
	h := NewHub(4)
	h.Publish([]byte("one"), true)
	ch, cancel := h.Subscribe()
	defer cancel()
	if got := <-ch; string(got) != "one" {
		t.Fatalf("got %q", got)
	}
}

func TestHubSlowSubscriberNeverBlocksAndGetsNewest(t *testing.T) {
	h := NewHub(4)
	ch, cancel := h.Subscribe()
	defer cancel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			h.Publish([]byte{byte(i)}, true) // nobody is reading ch
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}
	if got := <-ch; got[0] != byte(999%256) {
		t.Fatalf("expected newest frame, got %d", got[0])
	}
	if h.Viewers() != 1 {
		t.Fatalf("viewers = %d", h.Viewers())
	}
}

func TestHubPrerollOnlyGoesToRing(t *testing.T) {
	h := NewHub(3)
	ch, cancel := h.Subscribe()
	defer cancel()
	for i := 0; i < 5; i++ {
		h.Publish([]byte{byte(i)}, false)
	}
	select {
	case f := <-ch:
		t.Fatalf("pre-roll frame leaked to viewer: %v", f)
	default:
	}
	ring := h.Ring()
	if len(ring) != 3 || ring[0][0] != 2 || ring[2][0] != 4 {
		t.Fatalf("ring = %v", ring)
	}
}

// ---- auth ----

func TestProtectedRoutesRequireLogin(t *testing.T) {
	_, ts := newTestServer(t)
	for _, p := range []string{"/api/me", "/api/status", "/api/alerts", "/api/events", "/stream.mjpg", "/media/x.jpg"} {
		res, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", p, res.StatusCode)
		}
	}
}

func TestLogin(t *testing.T) {
	_, ts := newTestServer(t)

	bad, _ := json.Marshal(map[string]string{"username": testUser, "password": "nope"})
	res, _ := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(bad))
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("wrong password: %d", res.StatusCode)
	}

	c := loggedInClient(t, ts.URL)
	res, err := c.Get(ts.URL + "/api/me")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("me after login: %v %v", err, res)
	}
	res.Body.Close()

	// cookie flags
	body, _ := json.Marshal(map[string]string{"username": testUser, "password": testPass})
	res, _ = http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(body))
	res.Body.Close()
	ck := res.Cookies()
	if len(ck) != 1 || !ck[0].HttpOnly || ck[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie not hardened: %+v", ck)
	}
}

func TestForgedTokenRejected(t *testing.T) {
	_, ts := newTestServer(t)
	// unsigned "alg":"none" token
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	pl := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"admin","exp":9999999999}`))
	req, _ := http.NewRequest("GET", ts.URL+"/api/me", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: hdr + "." + pl + "."})
	res, _ := http.DefaultClient.Do(req)
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("alg=none token accepted: %d", res.StatusCode)
	}
}

func TestLoginRateLimit(t *testing.T) {
	_, ts := newTestServer(t)
	bad, _ := json.Marshal(map[string]string{"username": testUser, "password": "nope"})
	var last int
	for i := 0; i < loginMax+2; i++ {
		res, _ := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(bad))
		res.Body.Close()
		last = res.StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("status after many attempts = %d, want 429", last)
	}
}

func TestMediaNoDirectoryListing(t *testing.T) {
	_, ts := newTestServer(t)
	c := loggedInClient(t, ts.URL)
	res, _ := c.Get(ts.URL + "/media/")
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("media listing status %d, want 404", res.StatusCode)
	}
}

// ---- ingest ----

func dialIngest(t *testing.T, ts *httptest.Server, key string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ingest"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"X-Device-Key": {key}}})
}

func TestIngestRejectsWrongKey(t *testing.T) {
	_, ts := newTestServer(t)
	_, res, err := dialIngest(t, ts, "wrong-key-wrong-key")
	if err == nil {
		t.Fatal("dial with wrong key succeeded")
	}
	if res == nil || res.StatusCode != 401 {
		t.Fatalf("response = %v", res)
	}
}

func TestIngestFrameReachesViewerAndViewerCountReachesDevice(t *testing.T) {
	s, ts := newTestServer(t)
	conn, _, err := dialIngest(t, ts, testKey)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	ctx := context.Background()

	// Server tells the device how many viewers there are.
	ch, cancel := s.hub.Subscribe()
	defer cancel()
	deadline := time.After(5 * time.Second)
	for got := false; !got; {
		rctx, rc := context.WithTimeout(ctx, 3*time.Second)
		_, msg, err := conn.Read(rctx)
		rc()
		if err != nil {
			t.Fatalf("waiting for viewers message: %v", err)
		}
		var v struct{ Viewers int }
		_ = json.Unmarshal(msg, &v)
		got = v.Viewers == 1
		select {
		case <-deadline:
			t.Fatal("timeout")
		default:
		}
	}

	frame := testJPEG(t, 10)
	if err := conn.Write(ctx, websocket.MessageBinary, append([]byte{flagLive}, frame...)); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-ch:
		if !bytes.Equal(got, frame) {
			t.Fatal("frame corrupted in transit")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("viewer never received the frame")
	}
	if !s.hub.View(nil).Online {
		t.Fatal("hub should report device online")
	}
}

func TestIngestIgnoresNonJPEG(t *testing.T) {
	s, ts := newTestServer(t)
	conn, _, err := dialIngest(t, ts, testKey)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	_ = conn.Write(context.Background(), websocket.MessageBinary, []byte{flagLive, 'h', 'e', 'l', 'l', 'o'})
	_ = conn.Write(context.Background(), websocket.MessageBinary, append([]byte{flagLive}, testJPEG(t, 1)...))
	time.Sleep(300 * time.Millisecond)
	if n := len(s.hub.Ring()); n != 1 {
		t.Fatalf("ring has %d frames, want 1 (garbage must be dropped)", n)
	}
}

func TestAlertIsStoredPublishedAndClipped(t *testing.T) {
	s, ts := newTestServer(t)
	conn, _, err := dialIngest(t, ts, testKey)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	ctx := context.Background()

	events, cancelEv := s.broker.subscribe()
	defer cancelEv()

	// pre-roll, alert, then post-roll frames
	for i := 0; i < 3; i++ {
		_ = conn.Write(ctx, websocket.MessageBinary, append([]byte{flagPreroll}, testJPEG(t, uint8(i*20))...))
	}
	snap := testJPEG(t, 200)
	msg, _ := json.Marshal(map[string]any{"type": "alert", "confidence": 0.42, "snapshot": base64.StdEncoding.EncodeToString(snap)})
	_ = conn.Write(ctx, websocket.MessageText, msg)

	// alert event is published
	var alertID int64
	deadline := time.After(5 * time.Second)
wait:
	for {
		select {
		case b := <-events:
			var ev struct {
				Type  string
				Alert Alert
			}
			_ = json.Unmarshal(b, &ev)
			if ev.Type == "alert" {
				alertID = ev.Alert.ID
				if ev.Alert.Confidence != 0.42 || !strings.HasPrefix(ev.Alert.Snapshot, "/media/alert-") {
					t.Fatalf("bad alert event: %+v", ev.Alert)
				}
				break wait
			}
		case <-deadline:
			t.Fatal("no alert event")
		}
	}
	// snapshot file saved
	files, _ := filepath.Glob(filepath.Join(s.mediaDir, "alert-*.jpg"))
	if len(files) != 1 {
		t.Fatalf("snapshot files: %v", files)
	}
	if b, _ := os.ReadFile(files[0]); !bytes.Equal(b, snap) {
		t.Fatal("snapshot content differs")
	}
	// protected media download
	c := loggedInClient(t, ts.URL)
	res, _ := c.Get(ts.URL + "/media/" + filepath.Base(files[0]))
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("snapshot download %d", res.StatusCode)
	}

	if s.ffmpeg == "" {
		t.Log("ffmpeg not installed: skipping clip check")
		return
	}
	for i := 0; i < s.cfg.PostFrames; i++ {
		_ = conn.Write(ctx, websocket.MessageBinary, append([]byte{flagLive}, testJPEG(t, uint8(100+i*10))...))
		time.Sleep(20 * time.Millisecond)
	}
	// clip arrives as an alert_update
	deadline = time.After(20 * time.Second)
	for {
		select {
		case b := <-events:
			var ev struct {
				Type  string
				Alert Alert
			}
			_ = json.Unmarshal(b, &ev)
			if ev.Type == "alert_update" && ev.Alert.ID == alertID && ev.Alert.Clip != nil {
				clips, _ := filepath.Glob(filepath.Join(s.mediaDir, "clip-*.mp4"))
				if len(clips) != 1 {
					t.Fatalf("clip files: %v", clips)
				}
				if st, _ := os.Stat(clips[0]); st == nil || st.Size() < 500 {
					t.Fatal("clip is empty")
				}
				a, _ := s.store.Get(alertID)
				if a.Clip == nil {
					t.Fatal("store not updated with clip")
				}
				return
			}
		case <-deadline:
			t.Fatal("clip was never produced")
		}
	}
}

func TestStorePersists(t *testing.T) {
	dir := t.TempDir()
	st, _ := OpenStore(dir)
	id1, _ := st.Insert(time.Now(), 0.5, "/media/a.jpg")
	id2, _ := st.Insert(time.Now(), 0.6, "/media/b.jpg")
	_ = st.SetClip(id1, "/media/a.mp4")

	st2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := st2.List(10)
	if len(list) != 2 || list[0].ID != id2 || list[1].Clip == nil {
		t.Fatalf("list = %+v", list)
	}
	id3, _ := st2.Insert(time.Now(), 0.7, "/media/c.jpg")
	if id3 <= id2 {
		t.Fatalf("ids must keep increasing: %d after %d", id3, id2)
	}
}

// ---- retention ----

func addAlertWithFiles(t *testing.T, s *Server, ts time.Time, snapSize, clipSize int) Alert {
	t.Helper()
	id, err := s.store.Insert(ts, 0.5, "")
	if err != nil {
		t.Fatal(err)
	}
	snap := "alert-" + randName() + ".jpg"
	clip := "clip-" + randName() + ".mp4"
	for name, size := range map[string]int{snap: snapSize, clip: clipSize} {
		if err := os.WriteFile(filepath.Join(s.mediaDir, name), make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// point the stored alert at the files
	s.store.mu.Lock()
	for i := range s.store.alerts {
		if s.store.alerts[i].ID == id {
			s.store.alerts[i].Snapshot = "/media/" + snap
			c := "/media/" + clip
			s.store.alerts[i].Clip = &c
		}
	}
	s.store.mu.Unlock()
	a, _ := s.store.Get(id)
	return a
}

func exists(t *testing.T, s *Server, a Alert) bool {
	t.Helper()
	for _, f := range mediaFiles(a) {
		if _, err := os.Stat(filepath.Join(s.mediaDir, f)); err == nil {
			return true
		}
	}
	return false
}

func TestPruneByAge(t *testing.T) {
	s, _ := newTestServer(t)
	s.cfg.RetentionDays = 7
	s.cfg.MaxMediaBytes = 0
	now := time.Now()
	old := addAlertWithFiles(t, s, now.AddDate(0, 0, -8), 100, 100)
	recent := addAlertWithFiles(t, s, now.AddDate(0, 0, -2), 100, 100)

	events, cancel := s.broker.subscribe()
	defer cancel()
	if n := s.prune(now); n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	if exists(t, s, old) {
		t.Fatal("old alert's files were not deleted")
	}
	if !exists(t, s, recent) {
		t.Fatal("recent alert's files must be kept")
	}
	list, _ := s.store.List(10)
	if len(list) != 1 || list[0].ID != recent.ID {
		t.Fatalf("store = %+v", list)
	}
	select {
	case b := <-events:
		if !strings.Contains(string(b), "alert_removed") {
			t.Fatalf("event = %s", b)
		}
	default:
		t.Fatal("no alert_removed event published")
	}
}

func TestPruneBySizeCapDropsOldestFirst(t *testing.T) {
	s, _ := newTestServer(t)
	s.cfg.RetentionDays = 0
	s.cfg.MaxMediaBytes = 2500
	now := time.Now()
	a1 := addAlertWithFiles(t, s, now.Add(-5*time.Hour), 500, 500) // 1000 B each
	a2 := addAlertWithFiles(t, s, now.Add(-4*time.Hour), 500, 500)
	a3 := addAlertWithFiles(t, s, now.Add(-3*time.Hour), 500, 500)
	// 3000 B total, cap 2500 -> exactly the oldest must go
	if n := s.prune(now); n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	if exists(t, s, a1) || !exists(t, s, a2) || !exists(t, s, a3) {
		t.Fatal("expected only the oldest alert to be removed")
	}
}

func TestPruneNeverTouchesFreshAlertsOrDisabledLimits(t *testing.T) {
	s, _ := newTestServer(t)
	s.cfg.RetentionDays = 0
	s.cfg.MaxMediaBytes = 1 // absurdly small cap
	now := time.Now()
	fresh := addAlertWithFiles(t, s, now.Add(-10*time.Second), 500, 500) // clip may still be encoding
	if n := s.prune(now); n != 0 || !exists(t, s, fresh) {
		t.Fatalf("fresh alert was pruned (n=%d)", n)
	}
	s.cfg.MaxMediaBytes = 0 // both limits off: keep everything
	old := addAlertWithFiles(t, s, now.AddDate(-1, 0, 0), 500, 500)
	if n := s.prune(now); n != 0 || !exists(t, s, old) {
		t.Fatal("alert removed although retention is disabled")
	}
}

func TestPruneRemovesOnlyStaleOrphans(t *testing.T) {
	s, _ := newTestServer(t)
	s.cfg.RetentionDays, s.cfg.MaxMediaBytes = 0, 0
	now := time.Now()
	oldOrphan := filepath.Join(s.mediaDir, "clip-9-dead.mp4.part.mp4")
	newOrphan := filepath.Join(s.mediaDir, "clip-10-inprogress.mp4.part.mp4")
	for _, p := range []string{oldOrphan, newOrphan} {
		_ = os.WriteFile(p, []byte("x"), 0o644)
	}
	_ = os.Chtimes(oldOrphan, now.Add(-3*time.Hour), now.Add(-3*time.Hour))
	kept := addAlertWithFiles(t, s, now.Add(-3*time.Hour), 10, 10)
	_ = os.Chtimes(filepath.Join(s.mediaDir, mediaFiles(kept)[0]), now.Add(-3*time.Hour), now.Add(-3*time.Hour))

	s.prune(now)
	if _, err := os.Stat(oldOrphan); err == nil {
		t.Fatal("stale orphan not removed")
	}
	if _, err := os.Stat(newOrphan); err != nil {
		t.Fatal("in-progress file must not be removed")
	}
	if !exists(t, s, kept) {
		t.Fatal("referenced files must never be treated as orphans")
	}
}

func TestStoreRemove(t *testing.T) {
	dir := t.TempDir()
	st, _ := OpenStore(dir)
	a, _ := st.Insert(time.Now(), 0.1, "/media/a.jpg")
	b, _ := st.Insert(time.Now(), 0.2, "/media/b.jpg")
	if err := st.Remove(map[int64]bool{a: true}); err != nil {
		t.Fatal(err)
	}
	st2, _ := OpenStore(dir)
	list, _ := st2.List(10)
	if len(list) != 1 || list[0].ID != b {
		t.Fatalf("after reopen: %+v", list)
	}
}
