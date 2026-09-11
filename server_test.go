package main

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testServer(t *testing.T) *server {
	t.Helper()
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok"), Mode: fs.FileMode(0o644)}}
	return &server{store: &store{processes: map[string]*managedProcess{}}, assets: assets}
}

func TestHostnameValidation(t *testing.T) {
	valid := []string{"app.example.com", "home-1.example.co.uk"}
	invalid := []string{"localhost", "https://app.example.com", "-bad.example.com", "bad_.example.com"}
	for _, hostname := range valid {
		if !hostnamePattern.MatchString(hostname) {
			t.Errorf("expected %q to be valid", hostname)
		}
	}
	for _, hostname := range invalid {
		if hostnamePattern.MatchString(hostname) {
			t.Errorf("expected %q to be invalid", hostname)
		}
	}
}

func TestSubdomainValidation(t *testing.T) {
	valid := []string{"app", "home-1", "photos.home"}
	invalid := []string{"", ".app", "app.", "https://app", "bad_label"}
	for _, value := range valid {
		if !subdomainPattern.MatchString(value) {
			t.Errorf("expected %q to be valid", value)
		}
	}
	for _, value := range invalid {
		if subdomainPattern.MatchString(value) {
			t.Errorf("expected %q to be invalid", value)
		}
	}
}

func TestSecurityRejectsNonLocalHost(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://evil.example/api/state", nil)
	response := httptest.NewRecorder()
	testServer(t).handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestSecurityRejectsMutationWithoutHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4747/api/ports/refresh", nil)
	response := httptest.NewRecorder()
	testServer(t).handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestDeleteDeploymentRequiresExactHostname(t *testing.T) {
	s := testServer(t)
	s.store.state.Services = []Service{{ID: "service-1", Port: 3000, Hostname: "app.example.com", TunnelID: "faa937aa-b0ac-4244-b6bd-950033de4dad", Status: "stopped"}}
	request := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1:4747/api/services/service-1/deployment", bytes.NewBufferString(`{"hostname":"wrong.example.com"}`))
	request.Header.Set("X-Portivane-Request", "1")
	response := httptest.NewRecorder()
	s.handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
