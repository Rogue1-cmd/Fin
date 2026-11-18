package main

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName  = "session"
	sessionTTL  = 12 * time.Hour
	loginWindow = time.Minute
	loginMax    = 5
)

type Auth struct {
	user       string
	hash       []byte
	secret     []byte
	secure     bool
	trustProxy bool

	mu       sync.Mutex
	attempts map[string][]time.Time
}

func NewAuth(user, hash string, secret []byte, secure, trustProxy bool) *Auth {
	return &Auth{user: user, hash: []byte(hash), secret: secret, secure: secure,
		trustProxy: trustProxy, attempts: map[string][]time.Time{}}
}

func (a *Auth) clientIP(r *http.Request) string {
	if a.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// allow records an attempt and reports whether ip is under the rate limit.
func (a *Auth) allow(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	recent := a.attempts[ip][:0]
	for _, t := range a.attempts[ip] {
		if now.Sub(t) < loginWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= loginMax {
		a.attempts[ip] = recent
		return false
	}
	a.attempts[ip] = append(recent, now)
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	if !a.allow(a.clientIP(r)) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts, try again in a minute"})
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	// Always run bcrypt so response time doesn't reveal whether the username exists.
	passErr := bcrypt.CompareHashAndPassword(a.hash, []byte(body.Password))
	userOK := subtle.ConstantTimeCompare([]byte(body.Username), []byte(a.user)) == 1
	if passErr != nil || !userOK {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
		return
	}
	exp := time.Now().Add(sessionTTL)
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   a.user,
		ExpiresAt: jwt.NewNumericDate(exp),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}).SignedString(a.secret)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tok, Path: "/", Expires: exp,
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"user": a.user})
}

func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// User returns the logged-in user from the session cookie, or "".
func (a *Auth) User(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	claims := &jwt.RegisteredClaims{}
	tok, err := jwt.ParseWithClaims(c.Value, claims, func(*jwt.Token) (any, error) { return a.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !tok.Valid {
		return ""
	}
	return claims.Subject
}

func (a *Auth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.User(r) == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
