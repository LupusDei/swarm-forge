package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/swarm-forge/swarm-forge/internal/cli"
	"github.com/swarm-forge/swarm-forge/internal/crap"
	"github.com/swarm-forge/swarm-forge/internal/notify"
	"github.com/swarm-forge/swarm-forge/internal/setup"
	"github.com/swarm-forge/swarm-forge/internal/start"
	"github.com/swarm-forge/swarm-forge/internal/swarmlog"
	"github.com/swarm-forge/swarm-forge/internal/tmux"
)

func main() {
	commander := tmux.NewExecCommander()
	projectRoot, err := os.Getwd()
	if err != nil {
		fatal(err)
	}

	logFile := openLogFile(projectRoot)
	defer logFile.Close()
	logger := swarmlog.New(logFile, os.Stdout)

	cfg := cli.Config{
		Start:  startHandler(commander, projectRoot),
		Notify: notifyHandler(commander, logger),
		Log:    logHandler(logger),
		Crap:   crapHandler(),
	}

	if err := cli.Dispatch(os.Args[1:], cfg); err != nil {
		fatal(err)
	}
}

func startHandler(cmd tmux.Commander, root string) cli.Handler {
	return func(_ []string) error {
		cfg := start.Config{
			Commander:        cmd,
			Session:          "swarmforge",
			ProjectRoot:      root,
			FS:               setup.OSFS{},
			LookPath:         exec.LookPath,
			ConstitutionPath: "Constitution.md",
			Stdout:           os.Stdout,
		}
		return start.Run(cfg)
	}
}

func notifyHandler(cmd tmux.Commander, logger *swarmlog.Logger) cli.Handler {
	return func(args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("usage: swarmforge notify <pane> <message>")
		}
		pane, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid pane number: %s", args[0])
		}
		message := args[1]
		return notify.Notify(cmd, logger, "swarmforge", pane, message)
	}
}

func logHandler(logger *swarmlog.Logger) cli.Handler {
	return func(args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("usage: swarmforge log <role> <message>")
		}
		return logger.Write(args[0], args[1])
	}
}

func crapHandler() cli.Handler {
	return func(args []string) error {
		cfg, err := crap.ParseArgs(args, os.Stderr)
		if err != nil {
			os.Exit(2)
		}
		cfg.Stdout = os.Stdout
		cfg.Stderr = os.Stderr
		os.Exit(crap.Run(cfg))
		return nil
	}
}

func openLogFile(root string) *os.File {
	path := root + "/logs/agent_messages.log"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return os.Stdout
	}
	return f
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
