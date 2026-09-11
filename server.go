package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var hostnamePattern = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
var subdomainPattern = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$`)

type server struct {
	store  *store
	assets fs.FS
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.getState)
	mux.HandleFunc("GET /api/auth/status", s.authStatus)
	mux.HandleFunc("POST /api/auth/setup", s.authSetup)
	mux.HandleFunc("POST /api/auth/login", s.authLogin)
	mux.HandleFunc("POST /api/ports/refresh", s.refreshPorts)
	mux.HandleFunc("POST /api/cloudflare/login", s.login)
	mux.HandleFunc("POST /api/cloudflared/install", s.installCloudflared)
	mux.HandleFunc("GET /api/hostnames/check", s.checkHostname)
	mux.HandleFunc("POST /api/services", s.saveService)
	mux.HandleFunc("POST /api/services/{id}/start", s.startService)
	mux.HandleFunc("POST /api/services/{id}/stop", s.stopService)
	mux.HandleFunc("DELETE /api/services/{id}/deployment", s.deleteDeployment)
	mux.Handle("/", s.staticHandler())
	return s.secure(mux)
}

func (s *server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		host := strings.ToLower(request.Host)
		loopback := isLoopbackHost(host)
		if !loopback && os.Getenv("PORTIVANE_ALLOW_NETWORK") != "1" {
			http.Error(response, "local access only", http.StatusForbidden)
			return
		}
		publicAuthRoute := request.URL.Path == "/api/auth/status" || request.URL.Path == "/api/auth/setup" || request.URL.Path == "/api/auth/login"
		if strings.HasPrefix(request.URL.Path, "/api/") && !publicAuthRoute && !authenticated(request) && s.store.authHash != "" {
			writeError(response, http.StatusUnauthorized, "Authentication required.")
			return
		}
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("X-Frame-Options", "DENY")
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; font-src 'self' data:; img-src 'self' data:; connect-src 'self'; frame-src http://localhost:* http://127.0.0.1:*; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if request.Method != http.MethodGet && request.Header.Get("X-Portivane-Request") != "1" && request.Header.Get("X-Wyrmhole-Request") != "1" {
			writeError(response, http.StatusForbidden, "Request rejected. Use the local Portivane interface.")
			return
		}
		next.ServeHTTP(response, request)
	})
}

func (s *server) staticHandler() http.Handler {
	files := http.FileServer(http.FS(s.assets))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/") {
			writeError(response, http.StatusNotFound, "API route not found")
			return
		}
		path := strings.TrimPrefix(request.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(s.assets, path); err != nil {
			request.URL.Path = "/"
		}
		files.ServeHTTP(response, request)
	})
}

func (s *server) getState(response http.ResponseWriter, _ *http.Request) {
	s.store.setAccount(inspectAccount())
	writeJSON(response, http.StatusOK, s.store.snapshot())
}

func (s *server) refreshPorts(response http.ResponseWriter, _ *http.Request) {
	services, err := discoverServices()
	if err != nil {
		writeError(response, http.StatusInternalServerError, err.Error())
		return
	}
	s.store.mergeServices(services)
	s.store.setAccount(inspectAccount())
	writeJSON(response, http.StatusOK, s.store.snapshot())
}

func (s *server) login(response http.ResponseWriter, _ *http.Request) {
	if err := beginLogin(s.store); err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]string{"message": "Cloudflare login opened in your browser."})
}

func (s *server) installCloudflared(response http.ResponseWriter, _ *http.Request) {
	if err := downloadCloudflared(); err != nil {
		writeError(response, http.StatusInternalServerError, err.Error())
		return
	}
	s.store.setAccount(inspectAccount())
	writeJSON(response, http.StatusOK, s.store.snapshot())
}

func (s *server) checkHostname(response http.ResponseWriter, request *http.Request) {
	check, err := checkHostname(request.URL.Query().Get("subdomain"))
	if err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(response, http.StatusOK, check)
}

func (s *server) saveService(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Name     string `json:"name"`
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
	}
	if err := decodeJSON(request.Body, &input); err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	if input.Port < 1 || input.Port > 65535 || input.Port == 4747 {
		writeError(response, http.StatusBadRequest, "Port must be between 1 and 65535 and cannot be 4747.")
		return
	}
	if input.Protocol == "" {
		input.Protocol = "http"
	}
	if input.Protocol != "http" && input.Protocol != "https" && input.Protocol != "tcp" {
		writeError(response, http.StatusBadRequest, "Protocol must be http, https, or tcp.")
		return
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Service on " + strconv.Itoa(input.Port)
	}
	if len(name) > 80 {
		writeError(response, http.StatusBadRequest, "Service name is too long.")
		return
	}
	service := Service{ID: fmt.Sprintf("manual-%d", input.Port), Name: name, Port: input.Port, Protocol: input.Protocol, Process: "manual", OriginHost: "localhost", Status: "stopped"}
	s.store.addService(service)
	s.store.log("Added localhost:" + strconv.Itoa(input.Port) + " manually")
	writeJSON(response, http.StatusOK, s.store.snapshot())
}

func (s *server) startService(response http.ResponseWriter, request *http.Request) {
	id := request.PathValue("id")
	var input struct {
		Subdomain string `json:"subdomain"`
	}
	if err := decodeJSON(request.Body, &input); err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	check, err := checkHostname(input.Subdomain)
	if err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	hostname := check.Hostname
	state := s.store.snapshot()
	var selected *Service
	for i := range state.Services {
		if state.Services[i].ID == id {
			selected = &state.Services[i]
			break
		}
	}
	if selected == nil {
		writeError(response, http.StatusNotFound, "Local service not found.")
		return
	}
	if !check.Available && selected.Hostname != hostname {
		writeError(response, http.StatusConflict, hostname+" already has a DNS record. Choose another subdomain.")
		return
	}
	if !inspectAccount().Connected {
		writeError(response, http.StatusConflict, "Connect your Cloudflare account before publishing.")
		return
	}
	s.store.updateService(id, func(item *Service) { item.Status = "starting" })
	if err := startTunnel(s.store, *selected, hostname); err != nil {
		s.store.updateService(id, func(item *Service) { item.Status = "error" })
		s.store.log(err.Error())
		writeError(response, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(response, http.StatusOK, s.store.snapshot())
}

func (s *server) stopService(response http.ResponseWriter, request *http.Request) {
	if err := stopTunnel(s.store, request.PathValue("id")); err != nil {
		writeError(response, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(response, http.StatusOK, s.store.snapshot())
}

func (s *server) deleteDeployment(response http.ResponseWriter, request *http.Request) {
	id := request.PathValue("id")
	var input struct {
		Hostname string `json:"hostname"`
	}
	if err := decodeJSON(request.Body, &input); err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	state := s.store.snapshot()
	var selected *Service
	for i := range state.Services {
		if state.Services[i].ID == id {
			selected = &state.Services[i]
			break
		}
	}
	if selected == nil {
		writeError(response, http.StatusNotFound, "Deployment not found.")
		return
	}
	if selected.Hostname == "" || selected.TunnelID == "" {
		writeError(response, http.StatusConflict, "This service has no Cloudflare deployment to delete.")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(input.Hostname), selected.Hostname) {
		writeError(response, http.StatusBadRequest, "Type the full hostname to confirm deletion.")
		return
	}
	if err := stopTunnel(s.store, id); err != nil {
		writeError(response, http.StatusBadGateway, err.Error())
		return
	}
	time.Sleep(400 * time.Millisecond)
	if err := deleteTunnelDNSRecord(selected.Hostname, selected.TunnelID); err != nil {
		s.store.log("Deployment deletion stopped: " + err.Error())
		writeError(response, http.StatusBadGateway, err.Error())
		return
	}
	if err := deleteNamedTunnel(selected.TunnelID); err != nil {
		s.store.log("Deployment deletion stopped: " + err.Error())
		writeError(response, http.StatusBadGateway, err.Error())
		return
	}
	s.store.clearDeployment(id)
	s.store.log("Deleted deployment " + selected.Hostname + " from Cloudflare")
	writeJSON(response, http.StatusOK, s.store.snapshot())
}

func decodeJSON(body io.ReadCloser, target any) error {
	defer body.Close()
	decoder := json.NewDecoder(io.LimitReader(body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	return nil
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(response http.ResponseWriter, status int, message string) {
	writeJSON(response, status, map[string]string{"error": message})
}
