package start

import (
	"fmt"
	"io"
	"time"

	"github.com/swarm-forge/swarm-forge/internal/banner"
	"github.com/swarm-forge/swarm-forge/internal/preflight"
	"github.com/swarm-forge/swarm-forge/internal/prompt"
	"github.com/swarm-forge/swarm-forge/internal/setup"
	"github.com/swarm-forge/swarm-forge/internal/tmux"
)

const window = "swarm"

// Config holds everything needed for the start sequence.
type Config struct {
	Commander        tmux.Commander
	Session          string
	ProjectRoot      string
	FS               setup.FS
	LookPath         preflight.LookPathFunc
	ConstitutionPath string
	Stdout           io.Writer
	// Sleeper, when non-nil, is passed to tmux.SendKeys via tmux.WithSleeper
	// so the metrics-pane init can avoid real wall-clock sleeps in tests.
	// Production leaves this nil; the real time.Sleep is used.
	Sleeper tmux.Sleeper
	// Pause, when non-zero, overrides the default 200ms inter-call pause
	// in tmux.SendKeys. Zero means "use the tmux default".
	Pause time.Duration
}

// Run performs the full startup sequence as an ordered pipeline of steps.
// Any step returning an error short-circuits the pipeline — in particular,
// Attach is never called on an error path.
func Run(cfg Config) error {
	var constitution string
	steps := []func() error{
		func() error { return runPreflight(cfg) },
		func() error { return runSetup(cfg) },
		func() error {
			c, err := readConstitution(cfg)
			constitution = c
			return err
		},
		func() error { banner.Print(cfg.Stdout); return nil },
		func() error { return createSession(cfg) },
		func() error { return writeAndLaunchAgents(cfg, constitution) },
		func() error { return initMetricsPane(cfg) },
		func() error { return cfg.Commander.Attach(cfg.Session) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func runPreflight(cfg Config) error {
	return preflight.Check(cfg.LookPath, "tmux", "claude", "watch")
}

func runSetup(cfg Config) error {
	return setup.EnsureDirs(cfg.FS, cfg.ProjectRoot)
}

func readConstitution(cfg Config) (string, error) {
	path := cfg.ProjectRoot + "/" + cfg.ConstitutionPath
	data, err := cfg.FS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("constitution: %w", err)
	}
	return string(data), nil
}

func createSession(cfg Config) error {
	if cfg.Commander.HasSession(cfg.Session) {
		if err := tmux.KillSession(cfg.Commander, cfg.Session); err != nil {
			return err
		}
	}
	if err := tmux.CreateSession(cfg.Commander, cfg.Session, window); err != nil {
		return err
	}
	if err := tmux.SplitGrid(cfg.Commander, cfg.Session, window); err != nil {
		return err
	}
	return tmux.SetPaneTitles(cfg.Commander, cfg.Session, window)
}

func writeAndLaunchAgents(cfg Config, constitution string) error {
	promptsDir := cfg.ProjectRoot + "/.swarmforge/prompts"
	if err := cfg.FS.MkdirAll(promptsDir, 0o755); err != nil {
		return err
	}
	agents := []struct {
		pane         int
		name         string
		instructions string
	}{
		{0, "Architect", prompt.ArchitectInstructions},
		{1, "E2E-Interpreter", prompt.E2EInterpreterInstructions},
		{2, "Coder", prompt.CoderInstructions},
	}
	for _, a := range agents {
		acfg := prompt.AgentConfig{
			Role:         a.name,
			Instructions: a.instructions,
			Session:      cfg.Session,
			ProjectRoot:  cfg.ProjectRoot,
		}
		content := prompt.Build(acfg, constitution)
		promptFile := promptsDir + "/" + a.name + ".md"
		if err := cfg.FS.WriteFile(promptFile, []byte(content), 0o644); err != nil {
			return err
		}
		if err := tmux.LaunchAgent(cfg.Commander, cfg.Session, a.pane, a.name, promptFile, cfg.ProjectRoot); err != nil {
			return err
		}
	}
	return nil
}

func initMetricsPane(cfg Config) error {
	metricsCmd := "cd '" + cfg.ProjectRoot + "' && touch logs/agent_messages.log && tail -f logs/agent_messages.log"
	return tmux.SendKeys(cfg.Commander, cfg.Session, window, 3, metricsCmd, sendKeysOpts(cfg)...)
}

func sendKeysOpts(cfg Config) []tmux.SendKeysOption {
	var opts []tmux.SendKeysOption
	if cfg.Sleeper != nil {
		opts = append(opts, tmux.WithSleeper(cfg.Sleeper))
	}
	if cfg.Pause != 0 {
		opts = append(opts, tmux.WithPause(cfg.Pause))
	}
	return opts
}
