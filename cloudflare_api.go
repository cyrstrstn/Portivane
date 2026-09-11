package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var cloudflareAPIBase = "https://api.cloudflare.com/client/v4"

type originCredentials struct {
	ZoneID   string `json:"zoneID"`
	APIToken string `json:"apiToken"`
	Endpoint string `json:"endpoint,omitempty"`
}

type apiEnvelope[T any] struct {
	Success bool `json:"success"`
	Result  T    `json:"result"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type zoneResult struct {
	Name string `json:"name"`
}
type dnsRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

var cloudflareHTTPClient = &http.Client{
	Timeout:   8 * time.Second,
	Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
}

var zoneNames = struct {
	sync.RWMutex
	values map[string]string
}{values: map[string]string{}}

func originCertPath() (string, error) {
	home, err := os.UserHomeDir()
	if err == nil {
		path := filepath.Join(home, ".cloudflared", "cert.pem")
		if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
			return path, nil
		}
	}
	for _, path := range []string{"/etc/cloudflared/cert.pem", "/usr/local/etc/cloudflared/cert.pem"} {
		if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", os.ErrNotExist
}

func readOriginCredentials() (originCredentials, error) {
	path, err := originCertPath()
	if err != nil {
		return originCredentials{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return originCredentials{}, fmt.Errorf("read Cloudflare certificate: %w", err)
	}
	return decodeOriginCredentials(data)
}

func decodeOriginCredentials(data []byte) (originCredentials, error) {
	var credentials originCredentials
	for block, rest := pem.Decode(data); block != nil; block, rest = pem.Decode(rest) {
		if block.Type != "ARGO TUNNEL TOKEN" {
			continue
		}
		if err := json.Unmarshal(block.Bytes, &credentials); err != nil {
			return originCredentials{}, fmt.Errorf("decode Cloudflare login: %w", err)
		}
		break
	}
	if credentials.ZoneID == "" || credentials.APIToken == "" {
		return originCredentials{}, fmt.Errorf("Cloudflare login certificate has no tunnel token")
	}
	return credentials, nil
}

func cloudflareRequestMethod[T any](credentials originCredentials, method, path string, target *apiEnvelope[T]) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, cloudflareAPIBase+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+credentials.APIToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Portivane/0.1")
	response, err := cloudflareHTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("contact Cloudflare: %w", err)
	}
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("read Cloudflare response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !target.Success {
		message := "Cloudflare rejected the request"
		if len(target.Errors) > 0 && target.Errors[0].Message != "" {
			message = target.Errors[0].Message
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}

func cloudflareRequest[T any](credentials originCredentials, path string, target *apiEnvelope[T]) error {
	return cloudflareRequestMethod(credentials, http.MethodGet, path, target)
}

func fetchZoneName(credentials originCredentials) (string, error) {
	var envelope apiEnvelope[zoneResult]
	if err := cloudflareRequest(credentials, "/zones/"+url.PathEscape(credentials.ZoneID), &envelope); err != nil {
		return "", err
	}
	domain := strings.ToLower(strings.TrimSuffix(envelope.Result.Name, "."))
	if !hostnamePattern.MatchString("check." + domain) {
		return "", fmt.Errorf("Cloudflare returned an invalid zone name")
	}
	return domain, nil
}

func cachedZoneName(credentials originCredentials) (string, error) {
	zoneNames.RLock()
	domain := zoneNames.values[credentials.ZoneID]
	zoneNames.RUnlock()
	if domain != "" {
		return domain, nil
	}
	domain, err := fetchZoneName(credentials)
	if err != nil {
		return "", err
	}
	zoneNames.Lock()
	zoneNames.values[credentials.ZoneID] = domain
	zoneNames.Unlock()
	return domain, nil
}

func checkHostname(subdomain string) (HostnameCheck, error) {
	credentials, err := readOriginCredentials()
	if err != nil {
		return HostnameCheck{}, fmt.Errorf("connect Cloudflare before checking a hostname")
	}
	domain, err := fetchZoneName(credentials)
	if err != nil {
		return HostnameCheck{}, err
	}
	subdomain = strings.ToLower(strings.TrimSpace(subdomain))
	if !subdomainPattern.MatchString(subdomain) {
		return HostnameCheck{}, fmt.Errorf("enter a subdomain such as app or home.photos")
	}
	hostname := subdomain + "." + domain
	query := url.Values{"name": []string{hostname}, "per_page": []string{"1"}}
	var envelope apiEnvelope[[]dnsRecord]
	if err := cloudflareRequest(credentials, "/zones/"+url.PathEscape(credentials.ZoneID)+"/dns_records?"+query.Encode(), &envelope); err != nil {
		return HostnameCheck{}, err
	}
	result := HostnameCheck{Hostname: hostname, Available: len(envelope.Result) == 0}
	if len(envelope.Result) > 0 {
		result.ExistingType = envelope.Result[0].Type
	}
	return result, nil
}

func deleteTunnelDNSRecord(hostname, tunnelID string) error {
	credentials, err := readOriginCredentials()
	if err != nil {
		return fmt.Errorf("connect Cloudflare before deleting a deployment")
	}
	domain, err := cachedZoneName(credentials)
	if err != nil {
		return err
	}
	hostname = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(hostname), "."))
	if !hostnamePattern.MatchString(hostname) || !strings.HasSuffix(hostname, "."+domain) {
		return fmt.Errorf("refused to delete DNS outside the authorized Cloudflare zone")
	}
	return deleteTunnelDNSRecordWithCredentials(credentials, hostname, tunnelID)
}

func deleteTunnelDNSRecordWithCredentials(credentials originCredentials, hostname, tunnelID string) error {
	query := url.Values{"name": []string{hostname}, "per_page": []string{"100"}}
	path := "/zones/" + url.PathEscape(credentials.ZoneID) + "/dns_records?" + query.Encode()
	var records apiEnvelope[[]dnsRecord]
	if err := cloudflareRequest(credentials, path, &records); err != nil {
		return err
	}
	expectedTarget := strings.ToLower(tunnelID) + ".cfargotunnel.com"
	matched := false
	for _, record := range records.Result {
		if !strings.EqualFold(strings.TrimSuffix(record.Name, "."), hostname) {
			continue
		}
		if !strings.EqualFold(record.Type, "CNAME") || !strings.EqualFold(strings.TrimSuffix(record.Content, "."), expectedTarget) {
			return fmt.Errorf("refused to delete %s because its DNS record is not owned by this Portivane tunnel", hostname)
		}
		if record.ID == "" {
			return fmt.Errorf("Cloudflare returned a DNS record without an ID")
		}
		var deleted apiEnvelope[map[string]any]
		deletePath := "/zones/" + url.PathEscape(credentials.ZoneID) + "/dns_records/" + url.PathEscape(record.ID)
		if err := cloudflareRequestMethod(credentials, http.MethodDelete, deletePath, &deleted); err != nil {
			return fmt.Errorf("delete Cloudflare DNS record: %w", err)
		}
		matched = true
	}
	if matched {
		return nil
	}
	// A missing record is idempotent: it may already have been removed during a retry.
	return nil
}
