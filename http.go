// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	requestTimeout = 180 * time.Second
	userAgent      = "susepkg"
	maxAttempts    = 3
)

// retryDelay is the base delay between attempts; it doubles each retry.
// It is a variable so tests can shorten it.
var retryDelay = time.Second

var httpClient = &http.Client{
	Timeout:   requestTimeout,
	Transport: newTransport(),
}

// newTransport builds the RoundTripper used for all requests: it always
// sets the User-Agent header, and when DEBUG is set in the environment it
// also dumps the full request/response to stderr.
func newTransport() http.RoundTripper {
	var rt http.RoundTripper = &userAgentTransport{base: http.DefaultTransport}
	if os.Getenv("DEBUG") != "" {
		rt = &debugTransport{base: rt}
	}
	return rt
}

type userAgentTransport struct {
	base http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", userAgent)
	return t.base.RoundTrip(req)
}

type debugTransport struct {
	base http.RoundTripper
	mu   sync.Mutex // keeps concurrent dumps from interleaving
}

func (t *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqDump, _ := httputil.DumpRequestOut(req, true)
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		t.print(reqDump)
		return resp, err
	}
	respDump, _ := httputil.DumpResponse(resp, true)
	t.print(reqDump, respDump)
	return resp, nil
}

func (t *debugTransport) print(dumps ...[]byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, d := range dumps {
		fmt.Fprintln(os.Stderr, string(d))
	}
}

// getJSON performs a GET request against rawURL with the given headers and
// query parameters, and decodes the JSON response body into out.
// Callers unwrap any envelope (such as a "data" key) by decoding into a
// wrapper struct.
func getJSON(ctx context.Context, rawURL string, headers map[string]string, params url.Values, out any) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := doWithRetry(req)
	if err != nil {
		return fmt.Errorf("%s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		msg := strings.TrimSpace(string(snippet))
		if msg != "" {
			return fmt.Errorf("%s: HTTP %d: %s", rawURL, resp.StatusCode, msg)
		}
		return fmt.Errorf("%s: HTTP %d", rawURL, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s: decoding response: %w", rawURL, err)
	}
	return nil
}

// doWithRetry sends req, retrying with exponential backoff on transport
// errors, HTTP 429 and HTTP 5xx. The caller closes the returned body.
func doWithRetry(req *http.Request) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)
	delay := retryDelay
	for attempt := 1; ; attempt++ {
		resp, err = httpClient.Do(req)
		retryable := err != nil || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if !retryable || attempt == maxAttempts {
			return resp, err
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}
