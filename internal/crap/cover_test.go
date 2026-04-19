package crap_test

import (
	"math"
	"strings"
	"testing"

	"github.com/swarm-forge/swarm-forge/internal/crap"
)

func parseBlocks(t *testing.T, profile string) []crap.Block {
	t.Helper()
	blocks, err := crap.ParseCoverprofile(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("ParseCoverprofile: %v", err)
	}
	return blocks
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestCoverageForRange_FractionFromOverlappingBlocks(t *testing.T) {
	profile := "mode: set\n" +
		"pkg/x.go:10.2,15.3 4 1\n" +
		"pkg/x.go:16.2,20.3 4 0\n"
	blocks := parseBlocks(t, profile)

	got := crap.CoverageForRange(blocks, "pkg/x.go", 10, 20)
	if !approxEqual(got, 0.5) {
		t.Fatalf("coverage = %v, want 0.5", got)
	}
}

func TestCoverageForRange_NoMatchesIsZero(t *testing.T) {
	if got := crap.CoverageForRange(nil, "pkg/y.go", 1, 10); got != 0.0 {
		t.Fatalf("nil blocks: coverage = %v, want 0.0", got)
	}

	profile := "mode: set\npkg/other.go:1.1,5.1 2 1\n"
	blocks := parseBlocks(t, profile)
	if got := crap.CoverageForRange(blocks, "pkg/y.go", 1, 10); got != 0.0 {
		t.Fatalf("other-file blocks: coverage = %v, want 0.0", got)
	}
}

func TestParseCoverprofile_ToleratesModeLine(t *testing.T) {
	profile := "mode: set\npkg/x.go:1.1,2.1 1 1\n"
	blocks, err := crap.ParseCoverprofile(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("ParseCoverprofile: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected exactly 1 block (mode line must not become a block); got %d: %+v",
			len(blocks), blocks)
	}
	b := blocks[0]
	if b.File != "pkg/x.go" {
		t.Fatalf("block.File = %q, want pkg/x.go", b.File)
	}
	if b.StartLine != 1 || b.EndLine != 2 {
		t.Fatalf("block range = %d..%d, want 1..2", b.StartLine, b.EndLine)
	}
	if b.NumStmts != 1 || b.Count != 1 {
		t.Fatalf("block stmts=%d count=%d, want 1 1", b.NumStmts, b.Count)
	}
}

func TestCoverageForRange_PerFunctionSelectivity(t *testing.T) {
	profile := "mode: set\npkg/z.go:10.1,15.1 3 1\n"
	blocks := parseBlocks(t, profile)

	if got := crap.CoverageForRange(blocks, "pkg/z.go", 1, 5); got != 0.0 {
		t.Fatalf("function A (1..5) coverage = %v, want 0.0", got)
	}
	if got := crap.CoverageForRange(blocks, "pkg/z.go", 10, 15); !approxEqual(got, 1.0) {
		t.Fatalf("function B (10..15) coverage = %v, want 1.0", got)
	}
}
