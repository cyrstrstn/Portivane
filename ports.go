package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func discoverServices() ([]Service, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if runtime.GOOS == "windows" {
		return discoverWindows(ctx)
	}
	return discoverUnix(ctx)
}

func discoverWindows(ctx context.Context) ([]Service, error) {
	// `netstat -p tcp` omits TCPv6 on Windows. Many development servers bind to
	// ::1 only, so scan both TCP and TCPv6 and keep the actual listening host.
	output, err := exec.CommandContext(ctx, "netstat", "-ano").Output()
	if err != nil {
		return nil, fmt.Errorf("inspect listening ports: %w", err)
	}
	processes := windowsProcesses(ctx)
	ports := map[int]Service{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(strings.ToUpper(fields[0]), "TCP") || !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		port, ok := portFromAddress(fields[1])
		if !ok || port == 4747 {
			continue
		}
		pid, _ := strconv.Atoi(fields[4])
		process := processes[pid]
		ports[port] = serviceFromPort(port, process, hostFromAddress(fields[1]))
	}
	return sortedServices(ports), nil
}

func windowsProcesses(ctx context.Context) map[int]string {
	output, err := exec.CommandContext(ctx, "tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return map[int]string{}
	}
	reader := csv.NewReader(strings.NewReader(string(output)))
	result := map[int]string{}
	for {
		record, readErr := reader.Read()
		if readErr != nil {
			break
		}
		if len(record) < 2 {
			continue
		}
		pid, parseErr := strconv.Atoi(strings.ReplaceAll(record[1], ",", ""))
		if parseErr == nil {
			result[pid] = record[0]
		}
	}
	return result
}

func discoverUnix(ctx context.Context) ([]Service, error) {
	if output, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN").Output(); err == nil {
		ports := map[int]Service{}
		for index, line := range strings.Split(string(output), "\n") {
			if index == 0 {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 9 {
				continue
			}
			port, ok := portFromAddress(fields[len(fields)-2])
			if !ok || port == 4747 {
				continue
			}
			ports[port] = serviceFromPort(port, fields[0], hostFromAddress(fields[len(fields)-2]))
		}
		return sortedServices(ports), nil
	}
	output, err := exec.CommandContext(ctx, "ss", "-ltnpH").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("inspect listening ports: install lsof or ss")
	}
	ports := map[int]Service{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		port, ok := portFromAddress(fields[3])
		if !ok || port == 4747 {
			continue
		}
		process := ""
		if len(fields) > 5 {
			process = strings.Trim(fields[5], `users:(("`)
		}
		ports[port] = serviceFromPort(port, process, hostFromAddress(fields[3]))
	}
	return sortedServices(ports), nil
}

func portFromAddress(address string) (int, bool) {
	index := strings.LastIndex(address, ":")
	if index < 0 {
		return 0, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(address[index+1:]))
	return port, err == nil && port > 0 && port <= 65535
}

func hostFromAddress(address string) string {
	address = strings.TrimSpace(address)
	if strings.HasPrefix(address, "[") {
		if end := strings.Index(address, "]"); end > 1 {
			host := address[1:end]
			if host == "::" || host == "*" {
				return "::1"
			}
			return host
		}
	}
	if index := strings.LastIndex(address, ":"); index >= 0 {
		host := address[:index]
		if host == "" || host == "*" || host == "0.0.0.0" {
			return "127.0.0.1"
		}
		return host
	}
	return "localhost"
}

func serviceFromPort(port int, process, originHost string) Service {
	name := strings.TrimSuffix(process, ".exe")
	if name == "" {
		name = fmt.Sprintf("Service on %d", port)
	}
	return Service{ID: fmt.Sprintf("port-%d", port), Name: name, Port: port, Protocol: guessProtocol(port, process), Process: process, OriginHost: originHost, Status: "available"}
}

func guessProtocol(port int, process string) string {
	switch port {
	case 443, 8443, 9443:
		return "https"
	case 80, 3000, 3001, 4000, 4200, 5000, 5173, 8000, 8080, 8888:
		return "http"
	}
	process = strings.ToLower(process)
	for _, name := range []string{"node", "deno", "bun", "python", "php", "ruby", "java", "dotnet", "dart", "nginx", "caddy", "apache", "httpd", "docker"} {
		if strings.Contains(process, name) {
			return "http"
		}
	}
	return "tcp"
}

func sortedServices(ports map[int]Service) []Service {
	result := make([]Service, 0, len(ports))
	for _, service := range ports {
		result = append(result, service)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Port < result[j].Port })
	return result
}
