package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadCookieFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookie.txt")
	if err := os.WriteFile(path, []byte("Cookie: session=secret; csrftoken=token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readCookieFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "session=secret; csrftoken=token" {
		t.Fatalf("cookie = %q", got)
	}

	if err := os.WriteFile(path, []byte("session=secret\nX-Other: injected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCookieFile(path); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("expected safe validation error, got %v", err)
	}
}
