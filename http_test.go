// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetJSONRetries(t *testing.T) {
	defer func(d time.Duration) { retryDelay = d }(retryDelay)
	retryDelay = time.Millisecond

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"a": 1}`))
	}))
	defer srv.Close()

	var out struct{ A int }
	if err := getJSON(context.Background(), srv.URL, nil, nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.A != 1 || calls.Load() != 3 {
		t.Errorf("got %+v after %d calls", out, calls.Load())
	}
}

func TestGetJSONNoRetryOnClientError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	err := getJSON(context.Background(), srv.URL, nil, nil, new(any))
	if err == nil || !strings.Contains(err.Error(), "HTTP 404: nope") {
		t.Errorf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("got %d calls, want 1", calls.Load())
	}
}

func TestGetJSONDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	err := getJSON(context.Background(), srv.URL, nil, nil, new(any))
	if err == nil || !strings.Contains(err.Error(), srv.URL) {
		t.Errorf("error should mention the URL: %v", err)
	}
}

func TestUserAgentTransportDoesNotMutateRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != userAgent {
			t.Errorf("User-Agent = %q", got)
		}
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := (&userAgentTransport{base: http.DefaultTransport}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if req.Header.Get("User-Agent") != "" {
		t.Error("original request was mutated")
	}
}
