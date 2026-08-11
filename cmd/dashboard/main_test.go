package main

import "testing"

// The startup line is the only place a human reads the bind address, and
// 0.0.0.0 there is unopenable — the case this exists for.
func TestBrowsableURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		host string
		port int
		want string
	}{
		{"wildcard v4 becomes loopback", "0.0.0.0", 8881, "http://localhost:8881"},
		{"wildcard v6 too", "::", 8881, "http://localhost:8881"},
		{"empty host is also a wildcard", "", 8881, "http://localhost:8881"},
		{"a real host is left alone", "127.0.0.1", 8881, "http://127.0.0.1:8881"},
		{"a hostname is left alone", "dashboard.local", 80, "http://dashboard.local:80"},
		{"a literal v6 host keeps its brackets", "fd7a::1", 8881, "http://[fd7a::1]:8881"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := browsableURL(tc.host, tc.port); got != tc.want {
				t.Errorf("browsableURL(%q, %d) = %q, want %q", tc.host, tc.port, got, tc.want)
			}
		})
	}
}
