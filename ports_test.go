package main

import "testing"

func TestPortFromAddress(t *testing.T) {
	tests := []struct {
		input string
		want  int
		ok    bool
	}{
		{"127.0.0.1:8080", 8080, true},
		{"[::]:443", 443, true},
		{"0.0.0.0:*", 0, false},
		{"invalid", 0, false},
		{"127.0.0.1:70000", 70000, false},
	}
	for _, test := range tests {
		got, ok := portFromAddress(test.input)
		if got != test.want || ok != test.ok {
			t.Fatalf("portFromAddress(%q) = (%d, %v), want (%d, %v)", test.input, got, ok, test.want, test.ok)
		}
	}
}

func TestGuessProtocol(t *testing.T) {
	if got := guessProtocol(8443, "unknown"); got != "https" {
		t.Fatalf("guessProtocol(8443) = %q", got)
	}
	if got := guessProtocol(8080, "unknown"); got != "http" {
		t.Fatalf("guessProtocol(8080) = %q", got)
	}
	if got := guessProtocol(5432, "postgres.exe"); got != "tcp" {
		t.Fatalf("guessProtocol(5432) = %q", got)
	}
}

func TestHostFromAddress(t *testing.T) {
	tests := map[string]string{
		"127.0.0.1:3000": "127.0.0.1",
		"0.0.0.0:8080":   "127.0.0.1",
		"[::1]:3000":      "::1",
		"[::]:5173":       "::1",
	}
	for input, want := range tests {
		if got := hostFromAddress(input); got != want {
			t.Fatalf("hostFromAddress(%q) = %q, want %q", input, got, want)
		}
	}
}
