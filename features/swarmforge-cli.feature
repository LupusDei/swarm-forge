Feature: SwarmForge CLI

  The swarmforge CLI is a single Go binary that launches and coordinates
  the swarm. Running "swarmforge start" performs preflight checks, creates
  project directories, builds agent prompts, and launches a tmux session
  with three AI agents and a metrics dashboard. Agents coordinate via the
  "swarmforge notify" and "swarmforge log" subcommands.

  Scenario: Preflight rejects missing dependency
    Given the system does not have "tmux" installed
    When the user runs preflight checks
    Then an error is returned containing "tmux"

  Scenario: Preflight passes with all dependencies
    Given the system has "tmux", "claude", and "watch" installed
    When the user runs preflight checks
    Then no error is returned

  Scenario: Directory setup creates required directories
    Given a project root directory exists
    When directory setup runs for the project root
    Then the directory "features" exists under the project root
    And the directory "logs" exists under the project root
    And the directory "agent_context" exists under the project root

  Scenario: Agent prompt includes role and constitution
    Given a constitution with content "Rule 1: TDD is mandatory"
    And the agent role is "Architect" with standard instructions
    When the prompt builder generates the prompt
    Then the prompt contains "You are the Architect agent"
    And the prompt contains "Rule 1: TDD is mandatory"
    And the prompt contains "Pane 0 = Architect"

  Scenario: Agent prompt includes coordination instructions
    Given a constitution with content "Constitution content"
    And the agent role is "Coder" with standard instructions
    When the prompt builder generates the prompt
    Then the prompt contains "./swarmforge notify"
    And the prompt contains "./swarmforge log"
    And the prompt contains "agent_context/"

  Scenario: E2E Interpreter prompt scopes responsibility to coverage only
    Given a constitution with content "Constitution content"
    And the agent role is "E2E-Interpreter" with standard instructions
    When the prompt builder generates the prompt
    Then the prompt contains "cover every Gherkin scenario with a failing end-to-end test"
    And the prompt contains "hand off the failing E2E tests to the Coder"
    And the prompt does not contain "Ensure all Gherkin scenarios pass before any feature is marked complete"

  Scenario: Coder prompt states responsibility for making E2E tests pass
    Given a constitution with content "Constitution content"
    And the agent role is "Coder" with standard instructions
    When the prompt builder generates the prompt
    Then the prompt contains "receive failing end-to-end tests from the E2E Interpreter"
    And the prompt contains "implement the feature until every E2E test passes"

  Scenario: Start kills existing tmux session before creating new one
    Given a tmux session named "swarmforge" already exists
    When the start sequence runs
    Then the existing "swarmforge" session is killed
    And a new "swarmforge" session is created

  Scenario: Start creates tmux session with 2x2 grid layout
    Given no tmux session named "swarmforge" exists
    When the start sequence creates the tmux session
    Then a new tmux session "swarmforge" with window "swarm" is created
    And the window is split into 4 panes
    And pane borders display agent titles

  Scenario: Agents are launched with correct claude commands
    Given a tmux session "swarmforge" with 4 panes exists
    And agent prompt files have been written
    When agents are launched in their panes
    Then pane 0 receives a claude command containing "SwarmForge Architect"
    And pane 1 receives a claude command containing "SwarmForge E2E-Interpreter"
    And pane 2 receives a claude command containing "SwarmForge Coder"
    And each claude command includes "--permission-mode acceptEdits"

  Scenario: Metrics pane tails the agent log file
    Given a tmux session "swarmforge" with 4 panes exists
    When the metrics pane is initialized
    Then pane 3 receives a command containing "tail -f logs/agent_messages.log"

  Scenario: Start sequence attaches the user to the tmux session after launch
    Given no tmux session named "swarmforge" exists
    When the start sequence completes all setup steps
    Then the commander attaches to session "swarmforge"
    And the attach step occurs after the metrics pane is initialized
    And the attach step is the final action of the start sequence

  Scenario: Notify subcommand logs and sends message to pane
    Given a log writer is configured
    And a tmux commander is available
    When the user runs notify for pane 0 with message "hello architect"
    Then a timestamped log entry containing "[pane 0] hello architect" is written
    And tmux send-keys is invoked for session "swarmforge" pane 0

  Scenario: Notify always submits the handoff with a standalone Enter keystroke
    Given a recording tmux commander
    When the user runs notify for pane 2 with message "handoff ready"
    Then two "send-keys" invocations are recorded in order
    And both invocations target "swarmforge:swarm.2"
    And the first invocation uses the literal-input flag "-l" and carries the message "handoff ready"
    And the first invocation does NOT include the argument "Enter"
    And the second invocation's final argument is exactly "Enter"
    And the second invocation carries no payload other than "Enter"

  Scenario: SendKeys delivers text and Enter as two separate tmux calls
    Given a recording tmux commander
    And a recording sleeper that records every pause duration it is asked to wait
    When SendKeys is called for session "swarmforge" window "swarm" pane 1 with keys "any payload"
    Then exactly two tmux invocations are recorded
    And the first invocation is "send-keys" with the "-l" flag and payload "any payload"
    And the sleeper is invoked exactly once between the first and second tmux invocations
    And the second invocation is "send-keys" with a single key argument "Enter"
    And omitting either the "-l" text call or the trailing standalone Enter call is a violation of the handoff contract

  Scenario: SendKeys waits a configurable pause between text and Enter
    Given a recording tmux commander
    And a recording sleeper
    When SendKeys is called with a pause of 200 milliseconds
    Then the sleeper is asked to wait exactly 200 milliseconds
    When SendKeys is called with a pause of 750 milliseconds
    Then the sleeper is asked to wait exactly 750 milliseconds

  Scenario: SendKeys defaults to a 200 millisecond pause when no pause is configured
    Given a recording tmux commander
    And a recording sleeper
    And no pause duration has been explicitly configured
    When SendKeys is called with default configuration
    Then the sleeper is asked to wait exactly 200 milliseconds

  Scenario: SendKeys sleeper is injectable so tests run fast
    Given a no-op sleeper that returns immediately without sleeping
    And a recording tmux commander
    When SendKeys is called many times in a tight loop
    Then no wall-clock time is spent sleeping
    And both tmux invocations still occur per call in the documented order

  Scenario: Literal text send never has tmux key-name side effects
    Given a recording tmux commander
    And a recording sleeper
    When SendKeys is called with a message containing the token "Enter" embedded in the text
    Then the first tmux invocation uses the "-l" flag so "Enter" is sent as literal characters, not the Enter key
    And only the final standalone send-keys invocation with argument "Enter" actually submits the input

  Scenario: Log subcommand writes timestamped entry to file and stdout
    Given a log writer and stdout writer are configured
    When the user logs a message with role "Architect" and text "task started"
    Then the log writer contains "[Architect] task started"
    And the stdout writer contains "[Architect] task started"

  Scenario: Log entries are separated for readability in the Metrics pane
    Given a log writer is configured
    When the user logs a message with role "Architect" and text "task started"
    Then the log writer output contains a separator line of "========"
    And the log writer output ends with a newline character
    When the user logs a second message with role "Coder" and text "tests green"
    Then the log writer output contains both "task started" and "tests green"
    And the separator "========" appears between the two entries

  Scenario: CLI dispatches subcommands correctly
    Given the CLI receives arguments "start"
    Then the start handler is invoked
    Given the CLI receives arguments "notify" "1" "hello"
    Then the notify handler is invoked
    Given the CLI receives arguments "log" "Coder" "done"
    Then the log handler is invoked
    Given the CLI receives no arguments
    Then a usage error is returned

  Scenario: Full startup banner is displayed
    Given a writer captures output
    When the startup banner is printed
    Then the output contains "SwarmForge"
    And the output contains "Disciplined agents build better software"
