package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type store struct {
	mu        sync.RWMutex
	path      string
	state     AppState
	processes map[string]*managedProcess
	authSalt  string
	authHash  string
}

func newStore() (*store, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(configDir, "Portivane")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	s := &store{
		path:      filepath.Join(dataDir, "state.json"),
		state:     AppState{Services: []Service{}, Logs: []string{}, ServiceLogs: map[string][]string{}},
		processes: map[string]*managedProcess{},
	}
	// Carry existing installs across the Wyrmhole and Tunnelway renames.
	legacyStates := []string{
		filepath.Join(configDir, "Wyrmhole", "state.json"),
		filepath.Join(configDir, "Tunnelway", "state.json"),
	}
	if _, err := os.Stat(s.path); errors.Is(err, os.ErrNotExist) {
		for _, legacyState := range legacyStates {
			if data, readErr := os.ReadFile(legacyState); readErr == nil {
				_ = os.WriteFile(s.path, data, 0o600)
				break
			}
		}
	}
	if err := s.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

func (s *store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var persisted persistedState
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}
	for i := range persisted.Services {
		if persisted.Services[i].Status == "live" || persisted.Services[i].Status == "starting" {
			persisted.Services[i].Status = "stopped"
		}
	}
	s.state.Services = persisted.Services
	s.state.Logs = persisted.Logs
	s.state.ServiceLogs = persisted.ServiceLogs
	s.authSalt, s.authHash = persisted.AuthSalt, persisted.AuthHash
	s.state.AuthConfigured = s.authHash != ""
	if s.state.Services == nil {
		s.state.Services = []Service{}
	}
	if s.state.Logs == nil {
		s.state.Logs = []string{}
	}
	if s.state.ServiceLogs == nil {
		s.state.ServiceLogs = map[string][]string{}
	}
	return nil
}

func (s *store) saveLocked() error {
	persisted := persistedState{Services: s.state.Services, Logs: s.state.Logs, ServiceLogs: s.state.ServiceLogs, AuthSalt: s.authSalt, AuthHash: s.authHash}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}
	temp := s.path + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, s.path)
}

func (s *store) snapshot() AppState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.state
	state.Services = make([]Service, len(s.state.Services))
	copy(state.Services, s.state.Services)
	state.Logs = make([]string, len(s.state.Logs))
	copy(state.Logs, s.state.Logs)
	state.ServiceLogs = make(map[string][]string, len(s.state.ServiceLogs))
	for k, v := range s.state.ServiceLogs {
		copied := make([]string, len(v))
		copy(copied, v)
		state.ServiceLogs[k] = copied
	}
	return state
}

// log appends a global app-level message.
func (s *store) log(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := time.Now().Format("15:04:05") + "  " + message
	s.state.Logs = append(s.state.Logs, entry)
	if len(s.state.Logs) > 200 {
		s.state.Logs = append([]string(nil), s.state.Logs[len(s.state.Logs)-200:]...)
	}
	_ = s.saveLocked()
}

// serviceLog appends a message scoped to a specific service, and also to global.
func (s *store) serviceLog(serviceID, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := time.Now().Format("15:04:05") + "  " + message
	// Per-service log
	logs := s.state.ServiceLogs[serviceID]
	logs = append(logs, entry)
	if len(logs) > 300 {
		logs = append([]string(nil), logs[len(logs)-300:]...)
	}
	s.state.ServiceLogs[serviceID] = logs
	_ = s.saveLocked()
}

// clearServiceLog clears the log for a specific service (called on new deploy).
func (s *store) clearServiceLog(serviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.ServiceLogs[serviceID] = []string{}
	_ = s.saveLocked()
}

func (s *store) setAccount(account Account) { s.mu.Lock(); s.state.Account = account; s.mu.Unlock() }

func (s *store) updateService(id string, update func(*Service)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Services {
		if s.state.Services[i].ID == id {
			update(&s.state.Services[i])
			_ = s.saveLocked()
			return true
		}
	}
	return false
}

func (s *store) mergeServices(discovered []Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	known := make(map[int]Service, len(s.state.Services))
	for _, service := range s.state.Services {
		known[service.Port] = service
	}
	for i := range discovered {
		if previous, ok := known[discovered[i].Port]; ok {
			discovered[i].ID, discovered[i].Name, discovered[i].Hostname, discovered[i].TunnelID = previous.ID, previous.Name, previous.Hostname, previous.TunnelID
			if previous.Status == "live" || previous.Status == "starting" {
				discovered[i].Status = previous.Status
			}
			delete(known, discovered[i].Port)
		}
	}
	for _, service := range known {
		if service.Hostname != "" || service.Process == "manual" {
			service.Status = "stopped"
			discovered = append(discovered, service)
		}
	}
	s.state.Services = discovered
	_ = s.saveLocked()
}

func (s *store) addService(service Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Services {
		if s.state.Services[i].Port == service.Port {
			s.state.Services[i].Name = service.Name
			_ = s.saveLocked()
			return
		}
	}
	s.state.Services = append(s.state.Services, service)
	_ = s.saveLocked()
}

func (s *store) clearDeployment(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Services {
		if s.state.Services[i].ID != id {
			continue
		}
		service := &s.state.Services[i]
		service.Hostname = ""
		service.TunnelID = ""
		if service.Process == "manual" {
			s.state.Services = append(s.state.Services[:i], s.state.Services[i+1:]...)
		} else {
			service.Status = "available"
		}
		_ = s.saveLocked()
		return true
	}
	return false
}
