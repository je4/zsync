package client

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/je4/zsync/v2/info"
	"github.com/rs/zerolog"
)

func TestCheckRetryAndBackoff(t *testing.T) {
	logger := zerolog.Nop()
	c := &Client{
		Logger: &logger,
	}

	// No headers
	hEmpty := http.Header{}
	if c.CheckRetry(hEmpty) {
		t.Errorf("CheckRetry should return false for empty headers")
	}
	if c.CheckBackoff(hEmpty) {
		t.Errorf("CheckBackoff should return false for empty headers")
	}

	// Zero headers
	hZero := http.Header{
		"Retry-After": []string{"0"},
		"Backoff":     []string{"0"},
	}
	if c.CheckRetry(hZero) {
		t.Errorf("CheckRetry should return false for 0 Retry-After")
	}
	if c.CheckBackoff(hZero) {
		t.Errorf("CheckBackoff should return false for 0 Backoff")
	}

	// Invalid header strings
	hInvalid := http.Header{
		"Retry-After": []string{"invalid"},
		"Backoff":     []string{"invalid"},
	}
	if c.CheckRetry(hInvalid) {
		t.Errorf("CheckRetry should return false for invalid Retry-After")
	}
	if c.CheckBackoff(hInvalid) {
		t.Errorf("CheckBackoff should return false for invalid Backoff")
	}

	// Past HTTP-date Retry-After
	hPastDate := http.Header{
		"Retry-After": []string{"Fri, 31 Dec 1999 23:59:59 GMT"},
	}
	if c.CheckRetry(hPastDate) {
		t.Errorf("CheckRetry should return false for past HTTP-Date Retry-After")
	}

	// Future HTTP-date Retry-After (1 second ahead)
	futureDate := time.Now().Add(1 * time.Second).UTC().Format(http.TimeFormat)
	hFutureDate := http.Header{
		"Retry-After": []string{futureDate},
	}
	start := time.Now()
	if !c.CheckRetry(hFutureDate) {
		t.Errorf("CheckRetry should return true for future HTTP-Date Retry-After")
	}
	elapsed := time.Since(start)
	if elapsed < 500*time.Millisecond {
		t.Errorf("expected sleep on future HTTP-Date Retry-After, got elapsed %v", elapsed)
	}
}

func TestTagPagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/groups/100/tags" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Zotero-API-Version") != "3" {
			t.Errorf("expected Zotero-API-Version: 3, got '%s'", r.Header.Get("Zotero-API-Version"))
		}

		callCount++
		start := r.URL.Query().Get("start")
		w.Header().Set("Last-Modified-Version", "42")
		w.Header().Set("Total-Results", "3")
		w.Header().Set("Content-Type", "application/json")

		if start == "0" || start == "" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"tag":"tag1","meta":{"type":0,"numItems":1}},{"tag":"tag2","meta":{"type":0,"numItems":2}}]`))
		} else if start == "100" || start == "2" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"tag":"tag3","meta":{"type":0,"numItems":3}}]`))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()

	logger := zerolog.Nop()
	c, err := NewClient(testCtx, server.URL, "test-api-key", &logger)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	tags, lastMod, err := c.GetTags(testCtx, 100, 0)
	if err != nil {
		t.Fatalf("unexpected error fetching tags: %v", err)
	}
	if lastMod != 42 {
		t.Errorf("expected Last-Modified-Version=42, got %d", lastMod)
	}
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags aggregated across pages, got %d", len(tags))
	}
	if tags[0].Tag != "tag1" || tags[1].Tag != "tag2" || tags[2].Tag != "tag3" {
		t.Errorf("unexpected tags: %v", tags)
	}
}

func TestApiVersionHeaderAndInit(t *testing.T) {
	receivedVersionHeader := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedVersionHeader = r.Header.Get("Zotero-API-Version")
		if r.URL.Path == "/keys/test-api-key" || r.URL.Path == "/keys/current" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"key":"test-api-key","userId":123,"user":123,"access":{"user":{"library":true,"files":true}}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	logger := zerolog.Nop()
	c, err := NewClient(testCtx, server.URL, "test-api-key", &logger)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	if receivedVersionHeader != "3" {
		t.Errorf("expected Zotero-API-Version '3', got '%s'", receivedVersionHeader)
	}
	if c.client.Header.Get("Zotero-API-Version") != "3" {
		t.Errorf("expected client header Zotero-API-Version to be '3', got '%s'", c.client.Header.Get("Zotero-API-Version"))
	}
}

func TestUserAgentHeaderAndInit(t *testing.T) {
	receivedUserAgentHeader := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgentHeader = r.Header.Get("User-Agent")
		if r.URL.Path == "/keys/test-api-key" || r.URL.Path == "/keys/current" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"key":"test-api-key","userId":123,"user":123,"access":{"user":{"library":true,"files":true}}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	logger := zerolog.Nop()
	c, err := NewClient(testCtx, server.URL, "test-api-key", &logger)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	expectedUA := info.GetUserAgent()
	if receivedUserAgentHeader != expectedUA {
		t.Errorf("expected User-Agent '%s', got '%s'", expectedUA, receivedUserAgentHeader)
	}
	if c.client.Header.Get("User-Agent") != expectedUA {
		t.Errorf("expected client header User-Agent to be '%s', got '%s'", expectedUA, c.client.Header.Get("User-Agent"))
	}
}

func TestServerIdHeaderMock(t *testing.T) {
	mockServerID := "mock-server-id-xyz987"
	mockGroupVersions := map[string]int64{
		"6642571": 42,
		"1234567": 10,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "zsync") {
			t.Errorf("expected User-Agent starting with 'zsync', got '%s'", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Zotero-Server-ID", mockServerID)
		w.Header().Set("Zotero-API-Version", "3")
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/" || r.URL.Path == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message": "local api root"}`))
			return
		}

		if strings.HasPrefix(r.URL.Path, "/users/") && strings.HasSuffix(r.URL.Path, "/groups") {
			w.WriteHeader(http.StatusOK)
			_ = json.MarshalWrite(w, mockGroupVersions)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	logger := zerolog.Nop()
	c, err := NewClient(testCtx, server.URL, "", &logger)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if c.GetServerId() != mockServerID {
		t.Errorf("expected ServerId '%s', got '%s'", mockServerID, c.GetServerId())
	}

	versions, err := c.GetUserGroupVersions(testCtx, nil)
	if err != nil {
		t.Fatalf("GetUserGroupVersions failed on mock server: %v", err)
	}
	if versions[6642571] != 42 || versions[1234567] != 10 {
		t.Errorf("unexpected group versions result: %v", versions)
	}
}

// testCtx is the context used by all client tests.
var testCtx = context.Background()
