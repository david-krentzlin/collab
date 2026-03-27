# collab — Design Document

## Overview

`collab` is a filesystem-based communication protocol and CLI tool that enables
multiple AI agents (running in separate opencode sessions) to collaborate on
the same codebase. It prioritizes token efficiency: agents poll cheaply,
read selectively, and never rescan shared documents.

## Problem

When running multiple AI agents on the same project, they work in isolation.
There is no mechanism for them to ask each other questions, propose designs,
review each other's work, or coordinate on shared decisions. Naive approaches
(shared documents, chat logs) require agents to re-read entire histories on
every interaction, wasting tokens and context window.

## Design Principles

1. **Token efficiency above all.** Agents pay per token. Every design decision
   optimizes for minimal token consumption during normal operation.
2. **Filesystem as transport.** No servers, no sockets, no databases. Files
   are the universal interface that every tool can read and write.
3. **Human-readable artifacts.** Every message is a markdown file you can open
   in any editor. The protocol is inspectable without special tooling.
4. **Two audiences.** Agent-facing tools optimize for machine parsing and
   minimal output. Human-facing tools optimize for readability and context.

## Architecture

### Directory Structure

Current (task-scoped; `COLLAB_TASK` defaults to `default`):

```
.collab/
  default/                    # one directory per task/feature
    .seq                      # per-task sequence counter (integer)
    .seq.lock                 # lock file for sequence allocation
    .index.jsonl              # metadata index for fast check/read/resolve
    .index.lock               # lock file for index writes
    agent-a/                  # agent-a's outgoing messages
      001-inquiry.md
      003-reply.md
      005-info.md
    agent-b/                  # agent-b's outgoing messages
      002-reply.md
      004-proposal.md
      006-review.md
```

Future (multi-task):

```
.collab/
  .meta/
    tasks.md                  # registry: name, goal, agents, status
  auth-middleware/             # one directory per task
    .seq                      # per-task sequence counter
    agent-a/
      001-inquiry.md
    agent-b/
      002-reply.md
  fix-cache-race/
    .seq
    agent-a/
      001-proposal.md
```

### Message Format

Every message is a markdown file with YAML frontmatter:

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

Sequence allocation uses `.seq` + lock file (`.seq.lock`) and retries if a
concurrent writer updated the counter before commit.
```

### Frontmatter Fields

| Field     | Type   | Required | Description                                    |
|-----------|--------|----------|------------------------------------------------|
| `seq`     | int    | yes      | Global sequence number (total ordering)        |
| `from`    | string | yes      | Sender agent name                              |
| `to`      | string | yes      | Recipient agent name                           |
| `type`    | enum   | yes      | inquiry, reply, proposal, review, info         |
| `re`      | int    | no       | Seq number this references (0 = new thread)    |
| `ts`      | string | yes      | ISO 8601 UTC timestamp                         |
| `summary` | string | yes      | One-line description for token-efficient polls  |
| `status`  | enum   | yes      | open, resolved                                 |

### Message Types

- **inquiry** — Ask a question or request input from another agent.
- **reply** — Respond to a specific message (should set `re`).
- **proposal** — Suggest an approach, design, or implementation strategy.
- **review** — Provide feedback on code or a proposal.
- **info** — Proactively share information, decisions, or discoveries.

### Sequence Counter

A single `.seq` file contains an integer. On each `send`, the value is read,
locked, validated, incremented, and written back via temp file + rename.

Sequence writes are serialized with `flock` on `.seq.lock` and guarded by an
optimistic re-check before commit.

**Why global sequence numbers:** They provide a total ordering of all messages
across all agents. This enables deterministic replay and stable incremental
polling with `--since`.

## Agent-Facing Tools

These are the tools agents use. Optimized for token efficiency and machine
parseability. Exposed as opencode tool definitions that call the `collab` binary.

### collab check [--since N]

**Purpose:** Poll for new messages from other agents.

**How it works:**
1. Reads `.index.jsonl` metadata (not message bodies).
2. Filters to recipient + broadcasts and seq > `since`.
3. Outputs one compact line per message: seq, type, sender, re, summary.

**Output format:**
```
#4 [inquiry] from:agent-b re:#2 "Should we use a mutex for the seq counter?"
#6 [reply] from:agent-a re:#4 "Atomic rename is sufficient"
```

**Token efficiency:** Only summaries are returned. No bodies. An agent seeing
10 messages pays for ~10 lines of text, not 10 full message bodies.

### collab read <seq>

**Purpose:** Fetch the full body of a specific message.

**How it works:**
1. Resolves `seq -> path` from `.index.jsonl`.
2. Reads/parses only that specific message file.

**When to use:** After `check` reveals a message whose summary suggests it's
relevant to the agent's current work.

### collab send --to <agent> --type <type> --summary <text> [--re N]

**Purpose:** Send a message to another agent. Body is read from stdin.

**How it works:**
1. Reads `COLLAB_AGENT` env var for sender identity.
2. Increments the global sequence counter.
3. Reads body from stdin.
4. Writes a markdown file with frontmatter into the sender's directory.
5. Filename: `{seq:03d}-{type}.md`

**Summary is mandatory.** This is the core of the token efficiency design.
Agents write concise summaries for each other, enabling cheap triage via `check`
without reading full bodies.

### collab resolve <seq>

**Purpose:** Mark a message as resolved.

**How it works:** Reads the file, updates `status: resolved` in frontmatter,
writes it back.

## Human-Facing Tools

These tools are for developers reviewing agent conversations. They prioritize
readability, context, and completeness. Agents never call these.

### collab log

**Purpose:** Display the full conversation as a threaded tree in the terminal.

**Features:**
- Thread inference from `re` chains (see Threading section below).
- Tree indentation with box-drawing connectors.
- ANSI color per agent (auto-detected TTY).
- `--compact`: summaries only, no bodies.
- `--open`: hide resolved threads.
- `--no-color`: disable ANSI.

**Example output:**
```
── auth-middleware ──────────────────────
Thread: middleware chain order (resolved)

  #1 agent-a [proposal] "Register auth before rate-limit"
  │ Should we put auth first in the chain so that
  │ unauthenticated requests never hit rate limiting?
  │
  ├─ #2 agent-b [reply] "Agree, auth first"
  │  Yes. Also means we can skip CORS for rejected requests.
  │
  └─ #3 agent-a [info] "Implemented in middleware.go"
     Done. See commit a3f2e1.  ✓
```

### collab export --format json|html

**Purpose:** Export the full conversation as structured data or a self-contained
web page.

**JSON format:** A `TaskExport` object with thread trees computed from `re` chains.
This is the shared data layer that all renderers consume.

```json
{
  "task": "auth-middleware",
  "goal": "Add JWT auth middleware",
  "agents": ["agent-a", "agent-b"],
  "threads": [
    {
      "root": { "message": {...}, "children": [...] },
      "status": "resolved"
    }
  ],
  "orphans": [...]
}
```

**HTML format:** A single self-contained `.html` file with:
- Dark theme, monospace design.
- Collapsible threads.
- Color-coded agents and message types.
- Hide/show resolved threads.
- No external dependencies — JSON is inlined in a `<script>` tag.

## Threading Model

Threads are **inferred from `re` chains**, not explicitly declared. This
minimizes per-message overhead for agents while still producing structured
conversation views.

### How Thread Inference Works

1. All messages are sorted by seq.
2. A message with `re: 0` (or no `re` field) starts a new thread.
3. A message with `re: N` becomes a child of message N.
4. If message N doesn't exist, the message becomes an orphan.
5. Thread forks naturally: if both agents reply to #1, both become children
   of #1, creating two branches in the tree.

### Thread Status

A thread is "open" if any node in its tree has `status: open`.
A thread is "resolved" only when every node is resolved.

### Why Not Explicit Threads?

We considered requiring agents to name threads with a `thread` slug field.
Downsides:
- Extra cognitive overhead per message (agents must choose slug names).
- Agents might name inconsistently or forget.
- More tokens per message.

The `re` chain approach is zero-overhead: agents already need to reference
what they're replying to. The renderer handles the grouping.

### Future: Optional Thread Slugs

If auto-threading proves insufficient, an optional `thread` field can be added
to frontmatter. Messages with explicit thread slugs get grouped by slug.
Messages without fall back to `re`-chain inference. This is a backward-compatible
extension.

## Task Scoping (Future)

Currently all messages live in a single `.collab/` directory. The planned
extension adds task directories:

### Task Lifecycle

```bash
collab task create --name auth-middleware --agents agent-a,agent-b --goal "Add JWT auth"
collab task list
collab task complete auth-middleware
```

### Task Registry

`.collab/.meta/tasks.md` tracks all tasks:

```yaml
- name: auth-middleware
  goal: "Add JWT auth middleware"
  agents: [agent-a, agent-b]
  status: active
  created: "2026-03-27T14:00:00Z"

- name: fix-cache-race
  goal: "Fix race condition in LRU cache"
  agents: [agent-a, agent-b]
  status: completed
  created: "2026-03-27T15:00:00Z"
```

### Scoped Polling

With tasks, `check` and `send` gain a `--task` flag:

```bash
collab check --task auth-middleware --since 4
collab send --to agent-b --task auth-middleware --type inquiry ...
```

Each task has its own `.seq` counter, so sequence numbers are per-task.
This is a namespace change that keeps message volumes manageable as the
number of tasks grows.

## OpenCode Integration

### Environment Setup

Each agent session sets `COLLAB_AGENT` to its identity:

```bash
# Terminal 1
export COLLAB_AGENT=agent-a
export COLLAB_TASK=auth-middleware   # optional; defaults to "default"
opencode

# Terminal 2
export COLLAB_AGENT=agent-b
export COLLAB_TASK=auth-middleware
opencode
```

### Tool Definitions

Four tools are registered in opencode, each calling the `collab` binary:

1. `collab_check` — maps to `collab check --since N`
2. `collab_read` — maps to `collab read N`
3. `collab_send` — maps to `echo BODY | collab send --to X --type Y --summary Z [--re N]`
4. `collab_resolve` — maps to `collab resolve N`

See `TOOLS.md` for the complete tool definitions and system prompt additions.

### Agent Behavior Guidelines

Agents are instructed to:
- Poll with `check` before starting new work and after completing a task.
- Track the highest seq they've seen and pass `--since` to avoid re-reading.
- Write summaries that are informative enough to triage without reading the body.
- Reference specific seq numbers with `--re` to maintain thread structure.
- Proactively share relevant decisions via `info` messages.

## Token Efficiency Analysis

### Per-Poll Cost

A `check` call with 10 pending messages returns ~10 lines of ~80 chars each.
That's roughly 200 tokens. Compare to re-reading 10 full messages averaging
500 tokens each: 5,000 tokens. **25x savings.**

### Per-Read Cost

`read` returns one message body. Agents only read messages whose summaries
indicate relevance. In practice, an agent might read 2-3 of 10 new messages.
That's 1,000-1,500 tokens instead of 5,000.

### Per-Send Cost

`send` output is a 2-line confirmation (~50 tokens). The stdin body is not
echoed back.

### Cumulative

Over a 50-message conversation with 25 check cycles, the traditional approach
(re-read everything each time) costs roughly: 25 × 50 × 500 = 625,000 tokens.
With collab: 25 × 200 (checks) + 50 × 500 (selective reads) = 30,000 tokens.
**~20x reduction.**

## Implementation

### Language and Dependencies

- **Go** — single binary, no runtime dependencies, easy filesystem ops.
- **cobra** — CLI framework for subcommands.
- **golang.org/x/term** — TTY detection for auto-color in `log`.
- **gopkg.in/yaml.v3** — parse/serialize frontmatter robustly.

### Project Structure

```
collab/
  main.go                     # entry point
  go.mod
  cmd/
    root.go                   # cobra root command
    init.go                   # collab init
    send.go                   # collab send
    check.go                  # collab check
    read.go                   # collab read
    resolve.go                # collab resolve
    export.go                 # collab export (json/html)
    log.go                    # collab log (plaintext tree)
  internal/
    message/
      message.go              # Message type, marshal/unmarshal
    store/
      store.go                # filesystem ops, seq counter, read/write/list
    render/
      tree.go                 # thread tree builder (BuildThreads)
      plaintext.go            # terminal renderer with ANSI color
      json.go                 # JSON exporter
      html.go                 # self-contained HTML page renderer
```

### Build

```bash
go build -o collab .
```

## Open Questions

1. **Multi-agent broadcast.** Currently `to` is a single agent. Should we
   support `--to all` for broadcasting? Implications for `check` filtering.

2. **File attachments.** Agents might want to reference code snippets or diffs.
   Could use a `refs` frontmatter field listing file paths, or embed short
   snippets in the body.

3. **Agent discovery.** Currently agents are listed at `init` time. Should new
   agents be able to join a `.collab/` directory dynamically?

4. **Conflict detection.** If two agents edit the same file, they should be
   able to flag this. Could be a special `conflict` message type that
   auto-detects via git status.

5. **Integration with git.** Should `.collab/` be committed? It's useful for
   audit trails but adds noise to diffs. A `.gitignore` entry is probably right
   for active development, with explicit commits for archival.
