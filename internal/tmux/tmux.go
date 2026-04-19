package tmux

import (
	"fmt"
	"time"
)

// Commander abstracts tmux shell commands for testability.
type Commander interface {
	Run(args ...string) error
	HasSession(name string) bool
	Attach(session string) error
}

// Sleeper abstracts time.Sleep so tests can substitute a no-op or recorder.
// Production uses a real sleeper that wraps time.Sleep; tests MUST NOT sleep
// in wall-clock time.
type Sleeper interface {
	Sleep(d time.Duration)
}

// SendKeysOption configures an individual SendKeys call.
type SendKeysOption func(*sendKeysConfig)

// sendKeysConfig is the unexported configuration applied by SendKeysOptions.
// pause is a pointer so the call site can distinguish "unset" (nil, default
// 200ms applies) from "explicitly zero" (WithPause(0), zero sleep requested).
type sendKeysConfig struct {
	sleeper Sleeper
	pause   *time.Duration
}

// WithSleeper injects a Sleeper. When unset, SendKeys falls back to a
// realSleeper that delegates to time.Sleep. Tests pass a no-op or
// recording sleeper to avoid real sleeping.
func WithSleeper(s Sleeper) SendKeysOption {
	return func(c *sendKeysConfig) { c.sleeper = s }
}

// WithPause sets the pause between the text send and the Enter send.
// When unset, SendKeys uses the defaultSendKeysPause (200ms).
// Passing WithPause(0) explicitly requests zero sleep.
func WithPause(d time.Duration) SendKeysOption {
	return func(c *sendKeysConfig) { c.pause = &d }
}

// realSleeper is the production Sleeper. It wraps time.Sleep so SendKeys has
// a non-nil default when no WithSleeper option is passed.
type realSleeper struct{}

func (realSleeper) Sleep(d time.Duration) { time.Sleep(d) }

const defaultSendKeysPause = 200 * time.Millisecond

// CreateSession creates a new tmux session with the given name and window.
func CreateSession(cmd Commander, session, window string) error {
	return cmd.Run("new-session", "-d", "-s", session, "-n", window)
}

// SplitGrid splits a window into a 2x2 grid of panes.
func SplitGrid(cmd Commander, session, window string) error {
	target := session + ":" + window
	err := cmd.Run("split-window", "-t", target+".0", "-h", "-p", "50")
	if err != nil {
		return err
	}
	err = cmd.Run("split-window", "-t", target+".0", "-v", "-p", "50")
	if err != nil {
		return err
	}
	return cmd.Run("split-window", "-t", target+".2", "-v", "-p", "50")
}

// SetPaneTitles sets the title for each pane and enables border display.
func SetPaneTitles(cmd Commander, session, window string) error {
	target := session + ":" + window
	if err := assignPaneTitles(cmd, target); err != nil {
		return err
	}
	return setBorderOptions(cmd, session, target)
}

func assignPaneTitles(cmd Commander, target string) error {
	titles := []string{"Architect", "E2E Interpreter", "Coder", "Metrics"}
	for i, title := range titles {
		pane := fmt.Sprintf("%s.%d", target, i)
		if err := cmd.Run("select-pane", "-t", pane, "-T", title); err != nil {
			return err
		}
	}
	return nil
}

func setBorderOptions(cmd Commander, session, target string) error {
	if err := cmd.Run("set-option", "-t", session, "pane-border-status", "top"); err != nil {
		return err
	}
	if err := cmd.Run("set-option", "-t", session,
		"pane-border-format", " #{pane_title} "); err != nil {
		return err
	}
	return cmd.Run("set-window-option", "-t", target, "allow-rename", "off")
}

// KillSession kills an existing tmux session.
func KillSession(cmd Commander, session string) error {
	return cmd.Run("kill-session", "-t", session)
}

// LaunchAgent sends a claude command to the given pane.
func LaunchAgent(cmd Commander, session string, pane int, name, promptFile, projectRoot string) error {
	target := fmt.Sprintf("%s:swarm.%d", session, pane)
	command := fmt.Sprintf(
		"cd '%s' && claude --append-system-prompt-file '%s' --permission-mode acceptEdits -n 'SwarmForge %s'",
		projectRoot, promptFile, name,
	)
	return cmd.Run("send-keys", "-t", target, command, "Enter")
}

// SendKeys delivers keystrokes to a tmux pane in two calls: first the payload
// with tmux's '-l' literal flag so key-name tokens like "Enter" are typed as
// characters, then a configurable pause, then a standalone Enter to submit.
// The split is required because Claude Code's input box drops the Enter when
// it arrives in the same send-keys invocation as the pasted text.
func SendKeys(cmd Commander, session, window string, pane int, keys string, opts ...SendKeysOption) error {
	cfg := applySendKeysOpts(opts)
	target := fmt.Sprintf("%s:%s.%d", session, window, pane)
	if err := cmd.Run("send-keys", "-t", target, "-l", keys); err != nil {
		return err
	}
	cfg.sleeper.Sleep(*cfg.pause)
	return cmd.Run("send-keys", "-t", target, "Enter")
}

func applySendKeysOpts(opts []SendKeysOption) sendKeysConfig {
	cfg := sendKeysConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.sleeper == nil {
		cfg.sleeper = realSleeper{}
	}
	if cfg.pause == nil {
		d := defaultSendKeysPause
		cfg.pause = &d
	}
	return cfg
}
