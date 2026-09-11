package main

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecodeOriginCredentials(t *testing.T) {
	payload, err := json.Marshal(originCredentials{ZoneID: "zone-123", APIToken: "secret-token"})
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "ARGO TUNNEL TOKEN", Bytes: payload})
	credentials, err := decodeOriginCredentials(data)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.ZoneID != "zone-123" || credentials.APIToken != "secret-token" {
		t.Fatalf("unexpected credentials: %#v", credentials)
	}
}

func TestDecodeOriginCredentialsRejectsMissingToken(t *testing.T) {
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not a tunnel token")})
	if _, err := decodeOriginCredentials(data); err == nil {
		t.Fatal("expected missing token error")
	}
}

func TestFetchZoneName(t *testing.T) {
	testAPI := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/zones/zone-123" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer secret-token" {
			t.Error("missing bearer token")
		}
		_ = json.NewEncoder(response).Encode(map[string]any{"success": true, "result": map[string]string{"name": "Example.COM"}})
	}))
	defer testAPI.Close()
	previous := cloudflareAPIBase
	cloudflareAPIBase = testAPI.URL
	defer func() { cloudflareAPIBase = previous }()
	domain, err := fetchZoneName(originCredentials{ZoneID: "zone-123", APIToken: "secret-token"})
	if err != nil {
		t.Fatal(err)
	}
	if domain != "example.com" {
		t.Fatalf("domain = %q", domain)
	}
}

func TestDeleteTunnelDNSRecordOnlyDeletesOwnedCNAME(t *testing.T) {
	const tunnelID = "faa937aa-b0ac-4244-b6bd-950033de4dad"
	deleted := false
	testAPI := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/zones/zone-123/dns_records":
			_ = json.NewEncoder(response).Encode(map[string]any{"success": true, "result": []map[string]string{{"id": "record-1", "type": "CNAME", "name": "app.example.com", "content": tunnelID + ".cfargotunnel.com"}}})
		case request.Method == http.MethodDelete && request.URL.Path == "/zones/zone-123/dns_records/record-1":
			deleted = true
			_ = json.NewEncoder(response).Encode(map[string]any{"success": true, "result": map[string]string{"id": "record-1"}})
		default:
			http.Error(response, "unexpected request", http.StatusNotFound)
		}
	}))
	defer testAPI.Close()
	previous := cloudflareAPIBase
	cloudflareAPIBase = testAPI.URL
	defer func() { cloudflareAPIBase = previous }()
	credentials := originCredentials{ZoneID: "zone-123", APIToken: "secret-token"}
	if err := deleteTunnelDNSRecordWithCredentials(credentials, "app.example.com", tunnelID); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected owned DNS record to be deleted")
	}
}

func TestDeleteTunnelDNSRecordRefusesUnrelatedTarget(t *testing.T) {
	testAPI := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(response).Encode(map[string]any{"success": true, "result": []map[string]string{{"id": "record-1", "type": "CNAME", "name": "app.example.com", "content": "another-target.example.com"}}})
	}))
	defer testAPI.Close()
	previous := cloudflareAPIBase
	cloudflareAPIBase = testAPI.URL
	defer func() { cloudflareAPIBase = previous }()
	credentials := originCredentials{ZoneID: "zone-123", APIToken: "secret-token"}
	if err := deleteTunnelDNSRecordWithCredentials(credentials, "app.example.com", "faa937aa-b0ac-4244-b6bd-950033de4dad"); err == nil {
		t.Fatal("expected unrelated DNS target to be refused")
	}
}
