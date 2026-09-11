package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var uuidPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)

type managedProcess struct {
	cancel context.CancelFunc
	cmd    *exec.Cmd
}

func cloudflaredPath() (string, error) {
	name := "cloudflared"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if executable, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(executable), name)
		if info, statErr := os.Stat(beside); statErr == nil && !info.IsDir() {
			return beside, nil
		}
	}
	return exec.LookPath(name)
}

func cloudflaredInstallPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := "cloudflared"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(executable), name), nil
}

func downloadCloudflared() error {
	installPath, err := cloudflaredInstallPath()
	if err != nil {
		return fmt.Errorf("cannot determine install path: %w", err)
	}

	// Build download URL for official Cloudflare release
	var downloadURL string
	switch runtime.GOOS {
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			downloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe"
		case "arm64":
			downloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-arm64.exe"
		default:
			return fmt.Errorf("unsupported Windows architecture: %s", runtime.GOARCH)
		}
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			downloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-amd64.tgz"
		case "arm64":
			downloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-arm64.tgz"
		default:
			return fmt.Errorf("unsupported macOS architecture: %s", runtime.GOARCH)
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			downloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64"
		case "arm64":
			downloadURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-arm64"
		default:
			return fmt.Errorf("unsupported Linux architecture: %s", runtime.GOARCH)
		}
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("build download request: %w", err)
	}
	req.Header.Set("User-Agent", "Portivane/0.1")

	downloadClient := &http.Client{Timeout: 300 * time.Second}
	resp, err := downloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("download cloudflared: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	tmp := installPath + ".download"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write cloudflared: %w", err)
	}
	f.Close()

	if err := os.Rename(tmp, installPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("install cloudflared: %w", err)
	}
	return nil
}

func inspectAccount() Account {
	account := Account{}
	path, err := cloudflaredPath()
	if err != nil {
		return account
	}
	account.CloudflaredInstalled = true
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, _ := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	account.CloudflaredVersion = strings.TrimSpace(strings.TrimPrefix(string(output), "cloudflared version "))
	if credentials, credentialErr := readOriginCredentials(); credentialErr == nil {
		account.Connected = true
		account.Name = "Cloudflare connected"
		if domain, domainErr := cachedZoneName(credentials); domainErr == nil {
			account.Domain = domain
		}
	}
	return account
}

func beginLogin(s *store) error {
	path, err := cloudflaredPath()
	if err != nil {
		return fmt.Errorf("cloudflared was not found; install it or place it beside Portivane")
	}
	cmd := exec.Command(path, "tunnel", "login")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Cloudflare login: %w", err)
	}
	s.log("Cloudflare login opened in your browser")
	go func() { _ = cmd.Wait(); s.setAccount(inspectAccount()); s.log("Cloudflare login flow finished") }()
	return nil
}

func tunnelConfigPath(tunnelID, protocol, originHost string, port int) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(configDir, "Portivane", "tunnels")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	credentials := filepath.ToSlash(filepath.Join(home, ".cloudflared", tunnelID+".json"))
	if originHost == "" {
		originHost = "localhost"
	}
	origin := net.JoinHostPort(originHost, strconv.Itoa(port))
	content := fmt.Sprintf("tunnel: %s\ncredentials-file: %s\nurl: %s://%s\n", tunnelID, credentials, protocol, origin)
	path := filepath.Join(dir, tunnelID+".yml")
	return path, os.WriteFile(path, []byte(content), 0o600)
}

func createTunnel(binary string, service Service) (string, error) {
	name := fmt.Sprintf("portivane-%d-%s", service.Port, service.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "tunnel", "create", name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("create tunnel: %s", cleanOutput(output, err))
	}
	id := uuidPattern.FindString(string(output))
	if id == "" {
		return "", fmt.Errorf("cloudflared created the tunnel but did not return its ID")
	}
	return strings.ToLower(id), nil
}

func routeDNS(binary, tunnelID, hostname string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "tunnel", "route", "dns", tunnelID, hostname).CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(output)), "already exists") {
		return fmt.Errorf("create DNS route: %s", cleanOutput(output, err))
	}
	return nil
}

func startTunnel(s *store, service Service, hostname string) error {
	binary, err := cloudflaredPath()
	if err != nil {
		return fmt.Errorf("cloudflared was not found; install it or place it beside Portivane")
	}
	tunnelID := service.TunnelID
	isResume := tunnelID != ""
	if tunnelID == "" {
		tunnelID, err = createTunnel(binary, service)
		if err != nil {
			return err
		}
		s.updateService(service.ID, func(item *Service) { item.TunnelID = tunnelID })
		s.serviceLog(service.ID, "Created tunnel "+tunnelID)
	}
	if err := routeDNS(binary, tunnelID, hostname); err != nil {
		return err
	}
	config, err := tunnelConfigPath(tunnelID, service.Protocol, service.OriginHost, service.Port)
	if err != nil {
		return fmt.Errorf("write tunnel configuration: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, "tunnel", "--config", config, "run", tunnelID)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start tunnel: %w", err)
	}
	s.mu.Lock()
	s.processes[service.ID] = &managedProcess{cancel: cancel, cmd: cmd}
	s.mu.Unlock()
	s.updateService(service.ID, func(item *Service) { item.Hostname = hostname; item.TunnelID = tunnelID; item.Status = "starting" })
	// Only wipe logs on a fresh deploy; resumes keep prior log history
	if !isResume {
		s.clearServiceLog(service.ID)
	}
	s.serviceLog(service.ID, "Connecting "+originDisplay(service)+" to https://"+hostname)
	go scanProcessOutput(s, service.ID, stdout)
	go scanProcessOutput(s, service.ID, stderr)
	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		delete(s.processes, service.ID)
		s.mu.Unlock()
		s.updateService(service.ID, func(item *Service) {
			if item.Status == "live" || item.Status == "starting" {
				if err != nil {
					item.Status = "error"
				} else {
					item.Status = "stopped"
				}
			}
		})
		if err != nil {
			s.serviceLog(service.ID, "Tunnel stopped: "+err.Error())
		}
	}()
	return nil
}

func stopTunnel(s *store, id string) error {
	s.mu.RLock()
	process := s.processes[id]
	s.mu.RUnlock()
	if process == nil {
		s.updateService(id, func(item *Service) { item.Status = "stopped" })
		return nil
	}
	s.serviceLog(id, "Stopping tunnel…")
	process.cancel()
	s.mu.Lock()
	delete(s.processes, id)
	s.mu.Unlock()
	s.updateService(id, func(item *Service) { item.Status = "stopped" })
	// Keep only the final stopped message so next deploy starts with a clean log
	s.mu.Lock()
	s.state.ServiceLogs[id] = []string{time.Now().Format("15:04:05") + "  Tunnel stopped."}
	_ = s.saveLocked()
	s.mu.Unlock()
	s.log("Tunnel stopped by user")
	return nil
}

func deleteNamedTunnel(tunnelID string) error {
	if uuidPattern.FindString(tunnelID) != tunnelID {
		return fmt.Errorf("refused to delete an invalid tunnel ID")
	}
	binary, err := cloudflaredPath()
	if err != nil {
		return fmt.Errorf("cloudflared was not found; install it or place it beside Portivane")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "tunnel", "delete", "--force", tunnelID).CombinedOutput()
	if err != nil {
		message := strings.ToLower(string(output))
		if !strings.Contains(message, "not found") && !strings.Contains(message, "does not exist") {
			return fmt.Errorf("delete Cloudflare tunnel: %s", cleanOutput(output, err))
		}
	}
	configDir, _ := os.UserConfigDir()
	home, _ := os.UserHomeDir()
	for _, path := range []string{
		filepath.Join(configDir, "Portivane", "tunnels", tunnelID+".yml"),
		filepath.Join(configDir, "Wyrmhole", "tunnels", tunnelID+".yml"),
		filepath.Join(home, ".cloudflared", tunnelID+".json"),
	} {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("remove local tunnel credentials: %w", removeErr)
		}
	}
	return nil
}

func scanProcessOutput(s *store, id string, source io.Reader) {
	scanner := bufio.NewScanner(source)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			s.serviceLog(id, line)
			if strings.Contains(strings.ToLower(line), "registered tunnel connection") {
				s.updateService(id, func(item *Service) { item.Status = "live" })
			}
		}
	}
}

func originDisplay(service Service) string {
	host := service.OriginHost
	if host == "" {
		host = "localhost"
	}
	return net.JoinHostPort(host, strconv.Itoa(service.Port))
}

func cleanOutput(output []byte, err error) string {
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = err.Error()
	}
	if len(message) > 600 {
		message = message[:600]
	}
	return message
}
