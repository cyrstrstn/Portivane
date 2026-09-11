package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasswordSetupAndLogin(t *testing.T) {
	s := &store{state: AppState{}, processes: map[string]*managedProcess{}}
	app := &server{store: s}
	setup := httptest.NewRecorder()
	app.authSetup(setup, httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(`{"password":"correct horse battery"}`)))
	if setup.Code != 200 || s.authHash == "" || s.authSalt == "" {
		t.Fatalf("password setup failed: status=%d", setup.Code)
	}
	wrong := httptest.NewRecorder()
	app.authLogin(wrong, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"wrong password"}`)))
	if wrong.Code != 401 {
		t.Fatalf("wrong password status=%d", wrong.Code)
	}
	right := httptest.NewRecorder()
	app.authLogin(right, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"correct horse battery"}`)))
	if right.Code != 200 || right.Header().Get("Set-Cookie") == "" {
		t.Fatalf("valid login failed: status=%d", right.Code)
	}
}
