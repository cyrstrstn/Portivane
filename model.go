package main

type Service struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	Process    string `json:"process,omitempty"`
	OriginHost string `json:"originHost,omitempty"`
	Hostname   string `json:"hostname,omitempty"`
	Status     string `json:"status"`
	TunnelID   string `json:"tunnelId,omitempty"`
}

type Account struct {
	Connected            bool   `json:"connected"`
	Name                 string `json:"name,omitempty"`
	Email                string `json:"email,omitempty"`
	Domain               string `json:"domain,omitempty"`
	CloudflaredInstalled bool   `json:"cloudflaredInstalled"`
	CloudflaredVersion   string `json:"cloudflaredVersion,omitempty"`
}

type HostnameCheck struct {
	Hostname     string `json:"hostname"`
	Available    bool   `json:"available"`
	ExistingType string `json:"existingType,omitempty"`
}

type AppState struct {
	Account        Account             `json:"account"`
	Services       []Service           `json:"services"`
	Logs           []string            `json:"logs"`
	ServiceLogs    map[string][]string `json:"serviceLogs,omitempty"`
	AuthConfigured bool                `json:"authConfigured"`
}

type persistedState struct {
	Services    []Service           `json:"services"`
	Logs        []string            `json:"logs"`
	ServiceLogs map[string][]string `json:"serviceLogs,omitempty"`
	AuthSalt    string              `json:"authSalt,omitempty"`
	AuthHash    string              `json:"authHash,omitempty"`
}
