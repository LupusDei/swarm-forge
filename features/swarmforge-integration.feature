Feature: SwarmForge CLI Integration

  The individual CLI components (preflight, setup, prompt, tmux, etc.)
  are built and unit-tested. This feature verifies they are wired
  together end-to-end: a single call to start.Run must perform the
  complete startup sequence, and cmd/swarmforge/main.go must exist
  as a buildable entry point that dispatches to real implementations.

  Scenario: Start sequence performs complete startup with all components
    Given a recording commander and a fake filesystem
    And the fake filesystem contains a constitution file at "Constitution.md"
    When start.Run is called with the full configuration
    Then preflight checks are performed for "tmux", "claude", and "watch"
    And directory setup creates "features", "logs", and "agent_context"
    And the startup banner is printed to stdout
    And a tmux session "swarmforge" with window "swarm" is created
    And the session is split into a 2x2 grid
    And pane titles are set for all 4 panes
    And agent prompt files are written for "Architect", "Coder", and "E2E-Interpreter"
    And each prompt file contains the constitution content
    And each prompt file contains coordination instructions
    And claude is launched in pane 0 with name "SwarmForge Architect"
    And claude is launched in pane 1 with name "SwarmForge E2E-Interpreter"
    And claude is launched in pane 2 with name "SwarmForge Coder"
    And pane 3 receives "tail -f logs/agent_messages.log"
    And after all panes are initialized the commander attaches to session "swarmforge"

  Scenario: Start sequence kills existing session before creating new one
    Given a recording commander that reports session "swarmforge" exists
    And a fake filesystem with a constitution file
    When start.Run is called with the full configuration
    Then "kill-session" is called for "swarmforge" before "new-session"

  Scenario: Start sequence fails fast on missing dependency
    Given a lookpath function that rejects "claude"
    When start.Run is called with the full configuration
    Then an error is returned containing "claude"
    And no tmux commands are executed

  Scenario: Start sequence fails if constitution file is missing
    Given a fake filesystem without a constitution file
    When start.Run is called with the full configuration
    Then an error is returned containing "constitution"
    And no tmux commands are executed

  Scenario: Real commander executes tmux via os/exec
    Given a real commander implementation from the tmux package
    When Run is called with arguments "list-sessions"
    Then the commander invokes "tmux" with "list-sessions" via os/exec

  Scenario: CLI binary builds and dispatches start command
    Given the cmd/swarmforge package exists with a main.go
    When "go build ./cmd/swarmforge/" is run
    Then the build succeeds with exit code 0
    And the binary accepts "start", "notify", and "log" subcommands

  Scenario: Notify subcommand wires commander and logger end-to-end
    Given a recording commander and a log writer
    When the notify handler is called with pane "0" and message "hello"
    Then the log writer contains "pane 0" and "hello"
    And tmux send-keys is called for pane 0 in session "swarmforge"

  Scenario: Log subcommand writes to both file and stdout
    Given a file writer and a stdout writer
    When the log handler is called with role "Architect" and message "done"
    Then the file writer contains "[Architect] done"
    And the stdout writer contains "[Architect] done"
