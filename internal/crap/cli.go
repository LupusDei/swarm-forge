package crap

import (
	"flag"
	"fmt"
	"io"
)

// ParseArgs parses the crap subcommand flags.
func ParseArgs(args []string, stderr io.Writer) (Config, error) {
	fs := flag.NewFlagSet("crap", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := newDefaultConfig()
	registerFlags(fs, &cfg)
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if cfg.Coverprofile == "" {
		fmt.Fprintln(stderr, "swarmforge crap: --coverprofile is required")
		return cfg, fmt.Errorf("--coverprofile is required")
	}
	return cfg, nil
}

func newDefaultConfig() Config {
	return Config{
		Root:       ".",
		Thresholds: Thresholds{CC: 4, CRAP: 30},
	}
}

func registerFlags(fs *flag.FlagSet, cfg *Config) {
	fs.StringVar(&cfg.Coverprofile, "coverprofile", "", "path to coverprofile (required)")
	fs.StringVar(&cfg.Root, "root", ".", "analysis root directory")
	fs.BoolVar(&cfg.JSON, "json", false, "emit JSON output")
	fs.IntVar(&cfg.Thresholds.CC, "threshold-cc", 4, "max cyclomatic complexity")
	fs.Float64Var(&cfg.Thresholds.CRAP, "threshold-crap", 30,
		"CRAP threshold (exclusive upper bound)")
}

// Run executes the crap subcommand and returns the process exit code.
func Run(cfg Config) int {
	rows, err := Analyze(cfg)
	if err != nil {
		fmt.Fprintf(cfg.Stderr, "%s: %v\n", cfg.Coverprofile, err)
		return 2
	}
	if err := writeReport(cfg, rows); err != nil {
		fmt.Fprintf(cfg.Stderr, "render: %v\n", err)
		return 2
	}
	return exitCodeFor(rows)
}

func writeReport(cfg Config, rows []Row) error {
	if cfg.JSON {
		return RenderJSON(rows, cfg.Stdout)
	}
	return RenderText(rows, cfg.Thresholds, cfg.Stdout)
}

func exitCodeFor(rows []Row) int {
	for _, r := range rows {
		if len(r.Violations) > 0 {
			return 1
		}
	}
	return 0
}
