package fetch_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-rules-srs/internal/fetch"
)

func TestGetReturnsSuccessfulBoundedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "rules")
	}))
	defer server.Close()
	client := fetch.New(fetch.Options{Timeout: time.Second, AllowHTTPForTests: true})
	got, err := client.Get(context.Background(), server.URL, 5)
	if err != nil || string(got) != "rules" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestGetRejectsChunkedBodyOverLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, strings.Repeat("x", 9))
	}))
	defer server.Close()
	client := fetch.New(fetch.Options{Timeout: time.Second, AllowHTTPForTests: true})
	_, err := client.Get(context.Background(), server.URL, 8)
	if err == nil || !strings.Contains(err.Error(), "limit 8") {
		t.Fatalf("err=%v", err)
	}
}

func TestGetRejectsContentLengthAndNonSuccess(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"content length": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "9")
			_, _ = io.WriteString(w, "123456789")
		},
		"status": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "missing", http.StatusNotFound)
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			client := fetch.New(fetch.Options{Timeout: time.Second, AllowHTTPForTests: true})
			_, err := client.Get(context.Background(), server.URL, 8)
			if err == nil || !strings.Contains(err.Error(), server.URL) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestGetEnforcesTotalTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, "late")
	}))
	defer server.Close()
	client := fetch.New(fetch.Options{Timeout: 20 * time.Millisecond, AllowHTTPForTests: true})
	_, err := client.Get(context.Background(), server.URL, 32)
	if err == nil || !strings.Contains(err.Error(), "fetch") {
		t.Fatalf("err=%v", err)
	}
}

func TestGetLimitsRedirectsAndRejectsHTTPInProduction(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+r.URL.Path+"x", http.StatusFound)
	}))
	defer server.Close()
	client := fetch.New(fetch.Options{Timeout: time.Second, MaxRedirects: 2, AllowHTTPForTests: true})
	_, err := client.Get(context.Background(), server.URL, 32)
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("err=%v", err)
	}

	production := fetch.New(fetch.Options{Timeout: time.Second})
	_, err = production.Get(context.Background(), server.URL, 32)
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("err=%v", err)
	}
}

func TestGetRejectsInvalidLimitsAndCredentials(t *testing.T) {
	client := fetch.New(fetch.Options{Timeout: time.Second, AllowHTTPForTests: true})
	for _, tc := range []struct {
		url   string
		limit int64
	}{
		{"http://example.test", 0},
		{"http://user:pass@example.test", 10},
	} {
		if _, err := client.Get(context.Background(), tc.url, tc.limit); err == nil {
			t.Fatalf("url=%s limit=%d", tc.url, tc.limit)
		}
	}
}

func TestGetRejectsHTTPSDowngradeRedirect(t *testing.T) {
	httpTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "unsafe")
	}))
	defer httpTarget.Close()
	tlsSource := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, httpTarget.URL, http.StatusFound)
	}))
	defer tlsSource.Close()
	client := fetch.NewWithHTTPClient(fetch.Options{Timeout: time.Second}, tlsSource.Client())
	_, err := client.Get(context.Background(), tlsSource.URL, 32)
	if err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("err=%v", err)
	}
}
