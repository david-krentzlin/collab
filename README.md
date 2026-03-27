# collab — Agent-to-Agent Pair Programming

A filesystem-based communication protocol for AI agents working on the same codebase. Designed for token efficiency: agents poll summaries cheaply and only read full messages when needed.

## Architecture

```
.collab/
  default/                # task/feature namespace (from COLLAB_TASK)
    .seq                  # per-task global sequence counter
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

The seq file is read-then-write without locking.
Two agents could collide. Should we use flock?
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

## Usage

```bash
# Initialize
collab init --agents agent-a,agent-b

# Send (body via stdin, sender from env)
export COLLAB_AGENT=agent-a
export COLLAB_TASK=auth-middleware   # optional; defaults to "default"
echo "Should we split this into two packages?" | \
  collab send --to agent-b --type inquiry --summary "package structure question"

# Poll for new messages (summaries only)
export COLLAB_AGENT=agent-b
collab check
# output: #1 [inquiry] from:agent-a "package structure question"

collab check --since 1
# output: No new messages since #1

# Read full message
collab read 1

# Reply
echo "Yes, I think cmd/ and internal/ is the right split." | \
  collab send --to agent-a --type reply --re 1 --summary "agree on package split"

# Mark as resolved
collab resolve 1
```

## Token Efficiency Design

1. **`check`** returns only summaries — one line per message
2. **`read`** fetches full body on demand
3. **`summary`** field is mandatory — agents write it for each other
4. Agents track `last_seen_seq` and pass `--since` to avoid rescanning
5. Messages live in per-agent directories — no shared mutable state

## OpenCode Integration

See [TOOLS.md](./TOOLS.md) for tool definitions and system prompt additions.

## Future: Conversation Renderer

The structured format makes rendering straightforward:
- Glob `**/*.md`, sort by seq
- Build thread trees from `re` references
- Render as HTML/terminal with thread indentation
- Filter by status, agent, type
