# Agent Reliability Product Spec (Draft)

## Goal

Ensure two agents can use `collab` reliably from initialization through feature delivery, even when the harness starts a fresh shell for every tool invocation.

## Problem Summary

Current behavior depends on `COLLAB_AGENT` and `COLLAB_TASK` environment variables. In harnesses that spawn a new shell per tool call, these values may not persist, causing identity/task ambiguity and unreliable routing.

## Options Considered

### Option A: Explicit Flags Everywhere

Use `--agent` and `--task` on all agent-facing commands. Keep env vars as fallback only.

- Upside: deterministic per invocation; robust with fresh-shell harnesses
- Downside: more verbose command lines
- Complexity: low
- Risk: low

### Option B: Bootstrap-Generated Integration Artifacts

Add commands to generate AGENTS.md snippets, tool definitions, and usage skill files.

- Upside: consistent setup; fewer operator mistakes
- Downside: template maintenance
- Complexity: medium
- Risk: low-medium

### Option C: Profile-Based Context

Introduce `--profile <name>` where profile maps to agent/task defaults.

- Upside: concise commands with deterministic context
- Downside: additional config lifecycle
- Complexity: medium
- Risk: medium-low

### Option D: Auto-Derive Identity from Runtime Metadata

Infer agent identity from harness/session/runtime details.

- Upside: minimal manual setup in best case
- Downside: brittle and opaque when inference fails
- Complexity: high
- Risk: high

## Recommendation

Implement **A + B now**, **C later**, and avoid using D as primary behavior.

- A fixes the immediate reliability issue for ephemeral shells.
- B makes correct onboarding/configuration the default.
- C can improve ergonomics once behavior is stable.
- D can exist as optional best-effort fallback only.

## Command UX Spec (v0.1)

## Global Identity/Task Resolution

For agent-facing commands (`check`, `send`, `read`, `resolve`), resolve context in this order:

1. `--agent`, `--task` flags
2. `--profile <name>` (future)
3. `COLLAB_AGENT`, `COLLAB_TASK`
4. fail with actionable error

Example error:

`agent identity not set; pass --agent or set COLLAB_AGENT`

## `collab init` (Store Bootstrap + Skill Install)

```bash
collab init --agents agent-a,agent-b [--force] [--install-skill]
```

Behavior:

- refuses overwrite unless `--force`
- installs skill file by default at `.agents/skills/collab/SKILL.md`
* adds or appends AGENTS.md config unless it exists

## `collab doctor` (New Setup Verifier)

```bash
collab doctor --task feature-x --agent agent-a
```

Checks:

- store exists and is readable
- agent is present in initialized task agents
- seq/index integrity
- skill file is present
- generated tool templates resolve correctly
- optional roundtrip smoke-check (sandbox mode)

Returns non-zero on failure and prints actionable remediation.

## Generated AGENTS.md Snippet (Golden Template)

```md
## Collaboration via collab

When the user asks you to pair on a feature with another agent, you must use the collab skill to do that.
First ask the user which agent-identity you shall assume, and which task context to use.

IMPORTANT: use this agent-identity consistently during the whole session. (even if the task to work on changes)
```

## Installed Skill Content (Draft)

`collab init` should install `.agents/skills/collab/SKILL.md` including:

- when to poll (`check` cadence)
- summary quality guidance
- reply vs info usage
- thread discipline via `--re`
- resolve protocol


## Suggested Delivery Phases

### MVP

- add `--agent`/`--task` to agent-facing commands
* remove env-vars
* update `init` subcommand to add to AGENTS.md (add flog to forbid it)
* add `collab doctor`
