package acceptance

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swarm-forge/swarm-forge/internal/crap"
)

func writeCrapFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// ── crap: usage and I/O errors exit with code 2 ────────────────────

func TestCrap_ExitCode2OnUsageAndIOErrors(t *testing.T) {
	// Missing --coverprofile is a usage error.
	var stderr bytes.Buffer
	if _, err := crap.ParseArgs(nil, &stderr); err == nil {
		t.Error("expected error when --coverprofile is missing")
	} else if !strings.Contains(strings.ToLower(stderr.String()+err.Error()), "coverprofile") {
		t.Errorf("stderr/err should mention 'coverprofile'; stderr=%q err=%v",
			stderr.String(), err)
	}

	// Unknown flag is a usage error.
	stderr.Reset()
	if _, err := crap.ParseArgs(
		[]string{"--coverprofile=cover.out", "--nope"}, &stderr,
	); err == nil {
		t.Error("expected error for unknown flag --nope")
	}

	// Nonexistent coverprofile path: Run returns exit code 2 and mentions the path.
	var stdout, rstderr bytes.Buffer
	cfg := crap.Config{
		Root:         ".",
		Coverprofile: "does-not-exist.out",
		Thresholds:   crap.Thresholds{CC: 4, CRAP: 30},
		Stdout:       &stdout,
		Stderr:       &rstderr,
	}
	if code := crap.Run(cfg); code != 2 {
		t.Errorf("expected exit 2 for missing file; got %d; stderr=%q", code, rstderr.String())
	}
	if !strings.Contains(rstderr.String(), "does-not-exist.out") {
		t.Errorf("stderr should name the missing path; got: %q", rstderr.String())
	}
}

// ── crap: --root defaults to "." and accepts an override ──────────

func TestCrap_RootFlagDefaultAndOverride(t *testing.T) {
	cfg, err := crap.ParseArgs([]string{"--coverprofile=cover.out"}, io.Discard)
	if err != nil {
		t.Fatalf("ParseArgs (default): %v", err)
	}
	if cfg.Root != "." {
		t.Errorf("default Root = %q, want %q", cfg.Root, ".")
	}

	cfg, err = crap.ParseArgs(
		[]string{"--coverprofile=cover.out", "--root=./internal/crap"}, io.Discard,
	)
	if err != nil {
		t.Fatalf("ParseArgs (override): %v", err)
	}
	if cfg.Root != "./internal/crap" {
		t.Errorf("Root = %q, want %q", cfg.Root, "./internal/crap")
	}
}

// ── crap: threshold flags default to 4/30 and can be overridden ───

func TestCrap_ThresholdFlags(t *testing.T) {
	cfg, err := crap.ParseArgs([]string{"--coverprofile=cover.out"}, io.Discard)
	if err != nil {
		t.Fatalf("ParseArgs (defaults): %v", err)
	}
	if cfg.Thresholds.CC != 4 {
		t.Errorf("default Thresholds.CC = %d, want 4", cfg.Thresholds.CC)
	}
	if cfg.Thresholds.CRAP != 30 {
		t.Errorf("default Thresholds.CRAP = %v, want 30", cfg.Thresholds.CRAP)
	}

	cfg, err = crap.ParseArgs([]string{
		"--coverprofile=cover.out",
		"--threshold-cc=8",
		"--threshold-crap=50",
	}, io.Discard)
	if err != nil {
		t.Fatalf("ParseArgs (overrides): %v", err)
	}
	if cfg.Thresholds.CC != 8 {
		t.Errorf("Thresholds.CC = %d, want 8", cfg.Thresholds.CC)
	}
	if cfg.Thresholds.CRAP != 50 {
		t.Errorf("Thresholds.CRAP = %v, want 50", cfg.Thresholds.CRAP)
	}
}

// ── crap: exit code 0 clean; exit 1 on any violation ──────────────

func TestCrap_ExitCodePolicy(t *testing.T) {
	// Clean fixture: a single CC=1 function. CRAP <= 2, well under defaults.
	cleanDir := t.TempDir()
	writeCrapFile(t, filepath.Join(cleanDir, "clean.go"), "package p\nfunc F() {}\n")
	cleanCover := filepath.Join(cleanDir, "cover.out")
	writeCrapFile(t, cleanCover, "mode: set\n")

	var stdout, stderr bytes.Buffer
	clean := crap.Config{
		Root:         cleanDir,
		Coverprofile: cleanCover,
		Thresholds:   crap.Thresholds{CC: 4, CRAP: 30},
		Stdout:       &stdout,
		Stderr:       &stderr,
	}
	if code := crap.Run(clean); code != 0 {
		t.Errorf("clean fixture: expected exit 0; got %d; stdout=%s stderr=%s",
			code, stdout.String(), stderr.String())
	}

	// CC violation: a single CC=5 function breaches the default CC threshold (4).
	badSrc := `package p
func F(a, b, c, d int) {
	if a > 0 { _ = a }
	if b > 0 { _ = b }
	if c > 0 { _ = c }
	if d > 0 { _ = d }
}
`
	ccDir := t.TempDir()
	writeCrapFile(t, filepath.Join(ccDir, "bad.go"), badSrc)
	ccCover := filepath.Join(ccDir, "cover.out")
	writeCrapFile(t, ccCover, "mode: set\n")

	stdout.Reset()
	stderr.Reset()
	ccViol := crap.Config{
		Root:         ccDir,
		Coverprofile: ccCover,
		Thresholds:   crap.Thresholds{CC: 4, CRAP: 30},
		Stdout:       &stdout,
		Stderr:       &stderr,
	}
	if code := crap.Run(ccViol); code != 1 {
		t.Errorf("CC violation: expected exit 1; got %d; stdout=%s stderr=%s",
			code, stdout.String(), stderr.String())
	}

	// CRAP violation only: CC=5 with cov=0 yields CRAP=30. Raising CC threshold
	// to 10 isolates the CRAP violation; CRAP >= threshold must count as a violation.
	stdout.Reset()
	stderr.Reset()
	crapOnly := crap.Config{
		Root:         ccDir,
		Coverprofile: ccCover,
		Thresholds:   crap.Thresholds{CC: 10, CRAP: 30},
		Stdout:       &stdout,
		Stderr:       &stderr,
	}
	if code := crap.Run(crapOnly); code != 1 {
		t.Errorf("CRAP violation: expected exit 1 when CRAP==threshold; got %d; stdout=%s stderr=%s",
			code, stdout.String(), stderr.String())
	}
}

// ── crap: self-application smoke test ─────────────────────────────
//
// Runs `go test -coverprofile=<tmp> ./...` against the repository at the
// current working directory and then invokes crap.Run at the repo root.
// Expected exit code 0. Skipped in -short mode, and also skipped until
// the Coder finishes implementing the tool so the repo can actually
// satisfy the default thresholds.
func TestCrap_SelfApplicationIsClean(t *testing.T) {
	if testing.Short() {
		t.Skip("self-application smoke test skipped in -short mode")
	}
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	coverFile := filepath.Join(t.TempDir(), "cover.out")
	cmd := exec.Command("go", "test", "-short", "-coverprofile="+coverFile, "./...")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test failed: %v\n%s", err, string(out))
	}
	var stdout, stderr bytes.Buffer
	cfg := crap.Config{
		Root:         repoRoot,
		Coverprofile: coverFile,
		Thresholds:   crap.Thresholds{CC: 4, CRAP: 30},
		Stdout:       &stdout,
		Stderr:       &stderr,
	}
	if code := crap.Run(cfg); code != 0 {
		t.Fatalf("self-application: expected exit 0; got %d\nstdout:\n%s\nstderr:\n%s",
			code, stdout.String(), stderr.String())
	}
}

// findRepoRoot walks up from the current test's working directory until it
// finds go.mod. Exposed for use once the Coder un-skips the self-application
// smoke test.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
