package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
)

const authCookie = "portivane_session"

var sessions = struct {
	sync.RWMutex
	values map[string]struct{}
}{values: map[string]struct{}{}}

func derivePassword(password, salt string) string {
	digest := sha256.Sum256([]byte(salt + password))
	for i := 0; i < 120000; i++ {
		digest = sha256.Sum256(digest[:])
	}
	return base64.RawStdEncoding.EncodeToString(digest[:])
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *server) authStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"configured": s.store.authHash != ""})
}

func (s *server) authSetup(w http.ResponseWriter, r *http.Request) {
	if s.store.authHash != "" {
		writeError(w, http.StatusConflict, "Password is already configured.")
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r.Body, &input); err != nil || len(input.Password) < 8 {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters.")
		return
	}
	s.store.mu.Lock()
	s.store.authSalt = randomToken(16)
	s.store.authHash = derivePassword(input.Password, s.store.authSalt)
	s.store.state.AuthConfigured = true
	_ = s.store.saveLocked()
	s.store.mu.Unlock()
	s.issueSession(w)
	writeJSON(w, http.StatusOK, map[string]bool{"configured": true})
}

func (s *server) authLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.mu.RLock()
	salt, hash := s.store.authSalt, s.store.authHash
	s.store.mu.RUnlock()
	got := derivePassword(input.Password, salt)
	if hash == "" || subtle.ConstantTimeCompare([]byte(got), []byte(hash)) != 1 {
		writeError(w, http.StatusUnauthorized, "Incorrect password.")
		return
	}
	s.issueSession(w)
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *server) issueSession(w http.ResponseWriter) {
	token := randomToken(32)
	sessions.Lock()
	sessions.values[token] = struct{}{}
	sessions.Unlock()
	http.SetCookie(w, &http.Cookie{Name: authCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func authenticated(r *http.Request) bool {
	c, err := r.Cookie(authCookie)
	if err != nil {
		return false
	}
	sessions.RLock()
	_, ok := sessions.values[c.Value]
	sessions.RUnlock()
	return ok
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(host)
	return strings.HasPrefix(host, "127.0.0.1:") || strings.HasPrefix(host, "localhost:") || strings.HasPrefix(host, "[::1]:")
}
