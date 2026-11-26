//go:build windows

package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"
)

// What is said of a failed update never names the address the releases come from.
func TestUpdateWhyHidesTheAddress(t *testing.T) {
	addr := "https://example.org/owner/repo/main/latest.json"
	offline := fmt.Errorf("read manifest: %w", &url.Error{Op: "Get", URL: addr,
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "example.org", IsNotFound: true}}})
	for _, err := range []error{offline, errors.New("installer is larger than announced"), fmt.Errorf("address not allowed: %s", addr)} {
		said := updateWhy(err)
		if strings.Contains(said, "example.org") || strings.Contains(said, "http") || strings.Contains(said, err.Error()) {
			t.Errorf("the address shows: %q", said)
		}
	}
	if said := updateWhy(offline); !strings.Contains(said, "internet") {
		t.Errorf("no connection is not named: %q", said)
	}
}
