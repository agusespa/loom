package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"
)

// checkReadiness uses HTTP only: the probe never opens the DB or sends webhooks.
func checkReadiness(ctx context.Context, url string) error {
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness returned HTTP %d", res.StatusCode)
	}
	contentType, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return errors.New("readiness did not return JSON")
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&result); err != nil {
		return fmt.Errorf("invalid readiness response: %w", err)
	}
	if result.Status != "ready" {
		return fmt.Errorf("readiness returned status %q", result.Status)
	}
	return nil
}
