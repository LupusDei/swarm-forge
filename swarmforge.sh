#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────
# SwarmForge — tmux-based agent orchestration platform
# Launches a swarm of specialized AI agents under the SwarmForge Constitution
# ─────────────────────────────────────────────────────────────────────

SESSION="swarmforge"
PROJECT_ROOT="$(cd "$(dirname "$0")" && pwd)"
CONSTITUTION="$PROJECT_ROOT/Constitution.md"

# ── Colors ───────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

# ── Preflight checks ────────────────────────────────────────────────
check_dependency() {
  if ! command -v "$1" &>/dev/null; then
    echo -e "${RED}Error:${RESET} '$1' is required but not installed."
    exit 1
  fi
}

check_dependency tmux
check_dependency claude
check_dependency watch

if [[ ! -f "$CONSTITUTION" ]]; then
  echo -e "${RED}Error:${RESET} Constitution not found at $CONSTITUTION"
  exit 1
fi

# ── Project setup ────────────────────────────────────────────────────
mkdir -p "$PROJECT_ROOT/features" "$PROJECT_ROOT/logs" "$PROJECT_ROOT/agent_context"

cat > "$PROJECT_ROOT/swarm-log.sh" << 'EOF'
#!/bin/bash
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
echo "[$TIMESTAMP] [$1] $2" >> logs/agent_messages.log
echo "[$1] $2"
EOF
chmod +x "$PROJECT_ROOT/swarm-log.sh"

cat > "$PROJECT_ROOT/notify-agent.sh" << 'EOF'
#!/bin/bash
# Usage: ./notify-agent.sh <target-pane-index> "message"
# Panes: 0=Architect, 1=E2E Interpreter, 2=Coder, 3=Metrics
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
echo "[$TIMESTAMP] [pane $1] $2" >> logs/agent_messages.log
tmux send-keys -t swarmforge:swarm.$1 "$2" Enter
EOF
chmod +x "$PROJECT_ROOT/notify-agent.sh"

# ── Kill existing session if running ─────────────────────────────────
if tmux has-session -t "$SESSION" 2>/dev/null; then
  echo -e "${YELLOW}Existing SwarmForge session found. Killing it...${RESET}"
  tmux kill-session -t "$SESSION"
fi

echo -e "${CYAN}${BOLD}"
echo "  ╔═══════════════════════════════════════════════╗"
echo "  ║           SwarmForge v1.0 Starting            ║"
echo "  ║   Disciplined agents build better software    ║"
echo "  ╚═══════════════════════════════════════════════╝"
echo -e "${RESET}"

# ── Constitution preamble for agent system prompts ───────────────────
CONSTITUTION_CONTENT=$(cat "$CONSTITUTION")

build_agent_prompt() {
  local role="$1"
  local instructions="$2"
  cat <<EOF
You are the ${role} agent in the SwarmForge swarm.

## Your Role
${instructions}

## SwarmForge Constitution (MANDATORY — you must obey every rule)
${CONSTITUTION_CONTENT}

## Working Directory
${PROJECT_ROOT}

## Coordination
- You work inside a tmux session named "${SESSION}".
- To send a message to another agent, run: ./notify-agent.sh <pane> "message"
  - Pane 0 = Architect
  - Pane 1 = E2E Interpreter
  - Pane 2 = Coder
  - Pane 3 = Metrics (dashboard, not an agent)
- This types your message directly into that agent's prompt. They will see it and respond.
- Use ./swarm-log.sh "YourRole" "message" to log activity to logs/agent_messages.log.
- Shared files in agent_context/ can be used for passing larger artifacts between agents.
- Follow the Constitution strictly. Reject any work that violates it.
EOF
}

# ── Agent definitions ────────────────────────────────────────────────

ARCHITECT_PROMPT=$(build_agent_prompt "Architect" "$(cat <<'ROLE'
You are the lead Architect. You:
- Receive tasks from the user and decompose them into subtasks for the swarm.
- Design the overall architecture and define interfaces.
- Write Gherkin .feature files describing expected behavior BEFORE implementation.
- Coordinate the TDD cycle: ensure tests are written first, code passes, then refactor.
- Review the work of other agents and enforce the Constitution.
- You are the main point of contact for the human user.
ROLE
)")

CODER_PROMPT=$(build_agent_prompt "Coder" "$(cat <<'ROLE'
You are the Coder. You:
- Write production code ONLY to make failing tests pass (Green phase of TDD).
- Never write more code than necessary to pass the current failing test.
- Follow the architecture and interfaces defined by the Architect.
- Keep methods short, simple, and within complexity limits.
- After tests pass, participate in the Refactor phase.
- Never commit code without accompanying tests that were written first.
ROLE
)")

E2E_INTERPRETER_PROMPT=$(build_agent_prompt "E2E Interpreter" "$(cat <<'ROLE'
You are the E2E Interpreter. You:
- Parse Gherkin .feature files written by the Architect.
- Convert Given-When-Then scenarios into executable end-to-end test code.
- Run E2E tests and report results.
- Ensure all Gherkin scenarios pass before any feature is marked complete.
- Update Gherkin scenarios when behavior changes.
- Gherkin files are the single source of truth for expected system behavior.
ROLE
)")

# ── Create tmux session with 4 panes ────────────────────────────────
echo -e "${GREEN}Launching SwarmForge tmux session...${RESET}"

# Create session with 4 equal panes in a 2x2 grid
# Layout: Architect (TL) | Coder (TR)
#         E2E Interp (BL) | Metrics (BR)
tmux new-session -d -s "$SESSION" -n "swarm"

# Split into left and right columns
tmux split-window -t "$SESSION:swarm.0" -h -p 50

# Split left column into top/bottom
tmux split-window -t "$SESSION:swarm.0" -v -p 50

# Split right column into top/bottom
tmux split-window -t "$SESSION:swarm.2" -v -p 50

# After splits: 0=TL, 1=BL, 2=TR, 3=BR
tmux select-pane -t "$SESSION:swarm.0" -T "Architect"
tmux select-pane -t "$SESSION:swarm.1" -T "E2E Interpreter"
tmux select-pane -t "$SESSION:swarm.2" -T "Coder"
tmux select-pane -t "$SESSION:swarm.3" -T "Metrics"

# Show pane titles in borders
tmux set-option -t "$SESSION" pane-border-status top
tmux set-option -t "$SESSION" pane-border-format " #{pane_title} "
tmux set-window-option -t "$SESSION:swarm" allow-rename off

echo -e "${GREEN}Starting agents...${RESET}"

# ── Launch agents via claude CLI ─────────────────────────────────────
launch_agent() {
  local pane="$1"
  local name="$2"
  local prompt="$3"

  local prompt_file="/tmp/swarmforge-${name}.md"
  printf '%s' "$prompt" > "$prompt_file"

  tmux send-keys -t "$SESSION:swarm.$pane" \
    "cd '$PROJECT_ROOT' && claude --append-system-prompt-file '${prompt_file}' --permission-mode acceptEdits -n 'SwarmForge ${name}'" Enter

  echo -e "  ${CYAN}[${name}]${RESET} started in pane $pane"
}

launch_agent 0 "Architect"       "$ARCHITECT_PROMPT"
launch_agent 2 "Coder"           "$CODER_PROMPT"
launch_agent 1 "E2E-Interpreter" "$E2E_INTERPRETER_PROMPT"

# ── Metrics pane (pane 3 = bottom-right) ─────────────────────────────
tmux send-keys -t "$SESSION:swarm.3" "cd '$PROJECT_ROOT' && touch logs/agent_messages.log && tail -f logs/agent_messages.log" Enter

# ── Attach ───────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}${BOLD}SwarmForge is ready.${RESET}"
echo -e "Agents: Architect (TL), Coder (TR), E2E Interpreter (BL)"
echo -e "Metrics dashboard (BR)"
echo ""
echo -e "Attaching to tmux session '${SESSION}'..."
echo -e "${GREEN}Tip: Use the Architect pane (top-left) to give tasks to the swarm.${RESET}"
echo ""

tmux attach-session -t "$SESSION"
