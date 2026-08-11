package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cisco-open/synthetic-heart/common/proto"
)

func TestInitialiseAcceptsSupportedIPVersions(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		expected int
	}{
		{
			name: "default",
			config: `
address: http://example.com
expectedCodeRegex: ^200$
`,
			expected: 0,
		},
		{
			name: "ipv4",
			config: `
address: http://example.com
expectedCodeRegex: ^200$
ipVersion: 4
`,
			expected: 4,
		},
		{
			name: "ipv6",
			config: `
address: http://example.com
expectedCodeRegex: ^200$
ipVersion: 6
`,
			expected: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := &HttpPingTest{}
			if err := test.Initialise(synTestConfig(tt.config)); err != nil {
				t.Fatalf("Initialise returned error: %v", err)
			}
			if got := test.configs[0].IPVersion; got != tt.expected {
				t.Fatalf("expected ipVersion %d, got %d", tt.expected, got)
			}
			if got := test.configs[0].WaitBetweenRepeat; got != DefaultWaitBetweenRepeats {
				t.Fatalf("expected default waitBetweenRepeats %q, got %q", DefaultWaitBetweenRepeats, got)
			}
		})
	}
}

func TestInitialiseRejectsUnsupportedIPVersion(t *testing.T) {
	test := &HttpPingTest{}
	err := test.Initialise(synTestConfig(`
address: http://example.com
expectedCodeRegex: ^200$
ipVersion: 5
`))
	if err == nil {
		t.Fatal("expected unsupported ipVersion to fail")
	}
	if !strings.Contains(err.Error(), "invalid ipVersion 5") {
		t.Fatalf("expected invalid ipVersion error, got %v", err)
	}
}

func TestHTTPPingDefaultAddressSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	config := HttpPingTestConfig{
		Address:           server.URL,
		ExpectedCodeRegex: "^200$",
		WaitBetweenRepeat: DefaultWaitBetweenRepeats,
		timeout:           5 * time.Second,
	}

	result, err := httpPingTest(context.Background(), log.New(io.Discard, "", 0), config)
	if err != nil {
		t.Fatalf("httpPingTest returned error: %v", err)
	}
	values := result.(map[string]int)
	if values["marks"] != 1 {
		t.Fatalf("expected one mark for successful default HTTP ping, got %d", values["marks"])
	}
}

func TestHTTPPingForcedIPv6(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback is not available: %v", err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}),
	}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			t.Errorf("IPv6 test server failed: %v", serveErr)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	addr := listener.Addr().(*net.TCPAddr)
	config := HttpPingTestConfig{
		Address:           fmt.Sprintf("http://[%s]:%d", addr.IP.String(), addr.Port),
		ExpectedCodeRegex: "^200$",
		WaitBetweenRepeat: DefaultWaitBetweenRepeats,
		IPVersion:         6,
		timeout:           5 * time.Second,
	}

	result, err := httpPingTest(context.Background(), log.New(io.Discard, "", 0), config)
	if err != nil {
		t.Fatalf("httpPingTest returned error: %v", err)
	}
	values := result.(map[string]int)
	if values["marks"] != 1 {
		t.Fatalf("expected one mark for successful IPv6 HTTP ping, got %d", values["marks"])
	}
}

func synTestConfig(config string) proto.SynTestConfig {
	return proto.SynTestConfig{
		Timeouts: &proto.Timeouts{Run: "1m"},
		Config:   config,
	}
}
