package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckReadiness(t *testing.T) {
	for _, tc := range []struct {
		name        string
		code        int
		contentType string
		body        string
		wantError   bool
	}{
		{"ready", 200, "application/json; charset=utf-8", `{"status":"ready"}`, false},
		{"unavailable", 503, "application/json", `{"status":"not_ready"}`, true},
		{"SPA fallback", 200, "text/html", `<html>app</html>`, true},
		{"invalid JSON", 200, "application/json", `not JSON`, true},
		{"wrong status", 200, "application/json", `{"status":"ok"}`, true},
		{"missing status", 200, "application/json", `{}`, true},
		{"redirect", 302, "application/json", `{"status":"ready"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				if tc.code == http.StatusFound {
					w.Header().Set("Location", "/ready")
				}
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			err := checkReadiness(context.Background(), server.URL)
			if (err != nil) != tc.wantError {
				t.Fatalf("checkReadiness error %v, want error %v", err, tc.wantError)
			}
		})
	}
}

func TestCheckReadinessHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := checkReadiness(ctx, "http://127.0.0.1:1/api/readyz"); err == nil {
		t.Fatal("expected cancelled probe to fail")
	}
}

func TestCheckReadinessWhenServerStops(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()
	if err := checkReadiness(context.Background(), url); err == nil {
		t.Fatal("expected connection failure")
	}
}

func TestCheckReadinessTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := checkReadiness(ctx, server.URL); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
