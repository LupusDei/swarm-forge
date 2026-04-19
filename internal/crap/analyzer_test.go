package crap_test

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swarm-forge/swarm-forge/internal/crap"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// The walker must include only top-level .go files that are not test files,
// not under vendor/, and not under any directory whose name starts with '.'.
func TestAnalyze_FileFilters(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "foo.go"), "package p\nfunc FooFn() {}\n")
	writeFile(t, filepath.Join(root, "foo_test.go"), "package p\nfunc FooTestFn() {}\n")
	writeFile(t, filepath.Join(root, "vendor/x/y.go"), "package p\nfunc VendorFn() {}\n")
	writeFile(t, filepath.Join(root, ".git/a.go"), "package p\nfunc GitFn() {}\n")
	writeFile(t, filepath.Join(root, ".claude/b.go"), "package p\nfunc ClaudeFn() {}\n")
	writeFile(t, filepath.Join(root, "README.md"), "# not a go file\n")
	writeFile(t, filepath.Join(root, "pkg/c.go"), "package p\nfunc PkgFn() {}\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\nfunc MainFn() {}\n")

	coverPath := filepath.Join(root, "cover.out")
	writeFile(t, coverPath, "mode: set\n")

	cfg := crap.Config{
		Root:         root,
		Coverprofile: coverPath,
		Thresholds:   crap.Thresholds{CC: 4, CRAP: 30},
	}
	rows, err := crap.Analyze(cfg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	wantSuffixes := map[string]bool{"foo.go": false, "pkg/c.go": false, "main.go": false}
	for _, row := range rows {
		file := filepath.ToSlash(row.File)
		matched := false
		for suf := range wantSuffixes {
			if strings.HasSuffix(file, suf) {
				if strings.HasSuffix(file, "foo_test.go") {
					continue
				}
				wantSuffixes[suf] = true
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("unexpected row for file %q", row.File)
		}
		if strings.Contains(file, "/vendor/") ||
			strings.Contains(file, "/.git/") ||
			strings.Contains(file, "/.claude/") ||
			strings.HasSuffix(file, "_test.go") ||
			strings.HasSuffix(file, ".md") {
			t.Errorf("row for excluded file: %q", row.File)
		}
	}
	for suf, seen := range wantSuffixes {
		if !seen {
			t.Errorf("expected a row for %s, none found; rows=%+v", suf, rows)
		}
	}
}

func TestCrapScore(t *testing.T) {
	cases := []struct {
		cc   int
		cov  float64
		crap float64
		band string
	}{
		{1, 1.00, 1.00, "low"},
		{2, 0.50, 2.50, "low"},
		{4, 1.00, 4.00, "low"},
		{4, 0.00, 20.00, "moderate"},
		{5, 1.00, 5.00, "moderate"},
		{5, 0.00, 30.00, "high"},
		{10, 0.50, 22.50, "moderate"},
		{10, 0.00, 110.00, "high"},
	}
	for _, tc := range cases {
		got := crap.CrapScore(tc.cc, tc.cov)
		if math.Abs(got-tc.crap) > 1e-6 {
			t.Errorf("CrapScore(%d, %.2f) = %v, want %v", tc.cc, tc.cov, got, tc.crap)
		}
		if band := crap.Band(got); band != tc.band {
			t.Errorf("Band(%v) = %q, want %q (cc=%d cov=%.2f)", got, band, tc.band, tc.cc, tc.cov)
		}
	}
}
