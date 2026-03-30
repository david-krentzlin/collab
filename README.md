
# ATTENTION: highly experimental prototype. Use at your own risk

# collab — Agent-to-Agent Pair Programming

A filesystem-based communication protocol for AI agents working on the same codebase. Designed for token efficiency: agents poll summaries cheaply and only read full messages when needed.

## Architecture

```
.collab/
  default/                # task/feature namespace (selected via --task)
    .seq                  # per-task global sequence counter
    .seq.lock             # file lock for sequence allocation
    .index.jsonl          # append-only metadata index (seq->path, summary, status)
    .index.lock           # file lock for index updates
    agent-a/
      001-inquiry.md      # agent-a's outgoing messages
      003-reply.md
    agent-b/
      002-reply.md        # agent-b's outgoing messages
      004-proposal.md
```

Messages are markdown files with YAML frontmatter. Global sequence numbers provide total ordering across agents.

## Message Format

```yaml
---
seq: 4
from: agent-b
to: agent-a
type: inquiry
re: 2
ts: "2026-03-27T14:32:01Z"
summary: "Should we use a mutex for the seq counter?"
status: open
---

Sequence allocation uses a lock file and optimistic retry around `.seq` updates.
```

### Fields

| Field     | Description                                    |
|-----------|------------------------------------------------|
| `seq`     | Global sequence number (total ordering)        |
| `from`    | Sender agent name                              |
| `to`      | Recipient agent name                           |
| `type`    | `inquiry` `reply` `proposal` `review` `info`   |
| `re`      | Seq number this references (thread linking)    |
| `ts`      | ISO 8601 timestamp                             |
| `summary` | One-line description (for token-efficient poll)|
| `status`  | `open` or `resolved`                           |

## Install

```bash
go build -o collab .
# move to PATH
mv collab /usr/local/bin/
```

## Quality Gates

Use the repository `Makefile` to run consistent local checks:

```bash
# standard QA gate (used for local validation)
make quality-gate

# or run individual checks
make test
make test-race
make build

# strict gate including formatting + vet
make quality-gate-strict
```

Additional helper targets:

- `make fmt`
- `make vet`
- `make bench`
- `make qa` / `make ci` (aliases for `make quality-gate`)

## Usage

```bash
# Initialize
collab init --agents agent-a,agent-b

# Send (body via stdin, sender/task passed explicitly)
echo "Should we split this into two packages?" | \
  collab send --agent agent-a --task auth-middleware --to agent-b --type inquiry --summary "package structure question"

# Poll for new messages (summaries only)
collab check --agent agent-b --task auth-middleware
# output: #1 [inquiry] from:agent-a "package structure question"

collab check --agent agent-b --task auth-middleware --since 1
# output: No new messages since #1

# Wait for replies by polling (tail-like inbox wait)
collab check --agent agent-b --task auth-middleware --since 1 --poll 10 --interval 2s

# Read full message
collab read 1 --agent agent-b --task auth-middleware

# Reply
echo "Yes, I think cmd/ and internal/ is the right split." | \
  collab send --agent agent-b --task auth-middleware --to agent-a --type reply --re 1 --summary "agree on package split"

# Mark as resolved
collab resolve 1 --agent agent-a --task auth-middleware

# Validate setup for one agent/task context
collab doctor --agent agent-a --task auth-middleware

# Human log view for a specific task
collab log --task auth-middleware

# Follow conversation updates (tail-like refresh)
collab log --task auth-middleware --follow --interval 2s
```

## Token Efficiency Design

1. **`check`** returns only summaries — one line per message
2. **`read`** fetches full body on demand
3. **`summary`** field is mandatory — agents write it for each other
4. `check` and `read` use the metadata index to avoid directory rescans
5. Messages live in per-agent directories — no shared mutable state
