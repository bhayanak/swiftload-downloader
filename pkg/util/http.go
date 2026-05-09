package util

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// NewTransport creates an http.Transport tuned for high-throughput downloads.
func NewTransport(workers int, bufBytes int64, useProxy bool) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	t := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSClientConfig:      &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		ForceAttemptHTTP2:     true,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:  10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxConnsPerHost:       workers + 4,
		MaxIdleConns:          workers,
		MaxIdleConnsPerHost:   workers,
		ReadBufferSize:        int(bufBytes),
		WriteBufferSize:       2 * 1024 * 1024,
		DisableCompression:    true,
		Proxy:                 nil,
	}
	if useProxy {
		t.Proxy = http.ProxyFromEnvironment
	}
	return t
}

// NewHTTPClient creates an http.Client with the given transport and no timeout.
func NewHTTPClient(t *http.Transport) *http.Client {
	return &http.Client{
		Timeout:   0,
		Transport: t,
	}
}
