package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestShortName: map.txt keys a package by its last element, or the one
// before a /vN suffix (decision 0655).
func TestShortName(t *testing.T) {
	for pkg, want := range map[string]string{
		"strings":                        "strings",
		"unicode/utf8":                   "utf8",
		"math/rand/v2":                   "rand",
		"github.com/golang-jwt/jwt/v5":   "jwt",
		"google.golang.org/grpc/health":  "health",
		"github.com/fxamacker/cbor/v2":   "cbor",
		"go.opentelemetry.io/otel/v":     "v",
		"google.golang.org/grpc/codes":   "codes",
		"gopkg.in/yaml.v3":               "yaml",
		"github.com/cenkalti/backoff/v4": "backoff",
	} {
		if got := shortName(pkg); got != want {
			t.Errorf("shortName(%q) = %q, want %q", pkg, got, want)
		}
	}
}

// TestPinOf: a package belongs to the outside.txt module that is its
// path or a prefix of it at a slash.
func TestPinOf(t *testing.T) {
	f := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(f, []byte("# c\ngRPC | grpc | google.golang.org/grpc v1.79.2 | google.golang.org/grpc google.golang.org/grpc/codes\nGo time layouts | timego | go go1.26.1 | time\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := outsideFile
	outsideFile = f
	defer func() { outsideFile = old }()
	for pkg, want := range map[string]bool{
		"google.golang.org/grpc":       true,
		"google.golang.org/grpc/codes": true,
		"google.golang.org/grpcx":      false,
		"net/http":                     false,
		"time":                         false, // a go pin's packages are GOROOT's
	} {
		if _, ok := pinOf(pkg); ok != want {
			t.Errorf("pinOf(%q) = %v, want %v", pkg, ok, want)
		}
	}
	if p, _ := pinOf("google.golang.org/grpc/codes"); p.xoMod != "grpc" || p.version != "v1.79.2" || len(p.pkgs) != 2 {
		t.Errorf("pin = %+v", p)
	}
}
