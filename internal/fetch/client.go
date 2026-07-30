package fetch

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Options struct {
	Timeout               time.Duration
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	MaxRedirects          int
	AllowHTTPForTests     bool
}

type Client struct {
	options Options
	http    *http.Client
}

func New(options Options) *Client {
	options = withDefaults(options)
	dialer := &net.Dialer{Timeout: options.DialTimeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   options.TLSHandshakeTimeout,
		ResponseHeaderTimeout: options.ResponseHeaderTimeout,
		ForceAttemptHTTP2:     true,
	}
	return newClient(options, &http.Client{Transport: transport})
}

func NewWithHTTPClient(options Options, provided *http.Client) *Client {
	options = withDefaults(options)
	if provided == nil {
		return New(options)
	}
	copy := *provided
	return newClient(options, &copy)
}

func newClient(options Options, client *http.Client) *Client {
	client.Timeout = options.Timeout
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) > options.MaxRedirects {
			return fmt.Errorf("redirect limit %d exceeded", options.MaxRedirects)
		}
		if len(via) != 0 && via[len(via)-1].URL.Scheme == "https" && request.URL.Scheme != "https" {
			return fmt.Errorf("HTTPS downgrade redirect to %s is forbidden", request.URL.Redacted())
		}
		if request.URL.User != nil {
			return fmt.Errorf("redirect URL credentials are forbidden")
		}
		if request.URL.Scheme != "https" && !options.AllowHTTPForTests {
			return fmt.Errorf("redirect URL must use HTTPS")
		}
		return nil
	}
	return &Client{options: options, http: client}
}

func withDefaults(options Options) Options {
	if options.Timeout <= 0 {
		options.Timeout = 60 * time.Second
	}
	if options.DialTimeout <= 0 {
		options.DialTimeout = 10 * time.Second
	}
	if options.TLSHandshakeTimeout <= 0 {
		options.TLSHandshakeTimeout = 10 * time.Second
	}
	if options.ResponseHeaderTimeout <= 0 {
		options.ResponseHeaderTimeout = 15 * time.Second
	}
	if options.MaxRedirects <= 0 {
		options.MaxRedirects = 5
	}
	return options
}

func (c *Client) Get(ctx context.Context, rawURL string, maxBytes int64) ([]byte, error) {
	if c == nil || c.http == nil {
		return nil, fmt.Errorf("fetch client is not initialized")
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("fetch size limit must be positive")
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid fetch URL %q", rawURL)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("fetch URL credentials are forbidden: %s", parsed.Redacted())
	}
	if parsed.Fragment != "" {
		return nil, fmt.Errorf("fetch URL fragments are forbidden: %s", parsed.Redacted())
	}
	if parsed.Scheme != "https" && !(c.options.AllowHTTPForTests && parsed.Scheme == "http") {
		return nil, fmt.Errorf("fetch URL must use HTTPS: %s", parsed.Redacted())
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create fetch request %s: %w", parsed.Redacted(), err)
	}
	request.Header.Set("User-Agent", "clash-rules-srs/geodata-build")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", parsed.Redacted(), err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s returned HTTP %d", parsed.Redacted(), response.StatusCode)
	}
	if response.ContentLength > maxBytes {
		return nil, fmt.Errorf("fetch %s content length %d exceeds limit %d", parsed.Redacted(), response.ContentLength, maxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read fetch response %s: %w", parsed.Redacted(), err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("fetch %s body exceeds limit %d", parsed.Redacted(), maxBytes)
	}
	return body, nil
}
