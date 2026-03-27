# OpenCode Tool Definitions for `collab`

These tool definitions are designed to be added to your opencode configuration.
Each agent session should set `COLLAB_AGENT` to its identity.
Set `COLLAB_TASK` to scope messages under `.collab/<task>/` (defaults to `default`).

## Tool: `collab_check`

Polls for new messages from other agents. Returns only summaries (frontmatter)
for token efficiency. Call this periodically to see if the other agent has
responded or asked something.

```json
{
  "name": "collab_check",
  "description": "Check for new messages from other agents. Returns a compact list of message summaries (seq number, type, sender, one-line summary). Use this to poll for updates. Only messages from OTHER agents are shown. Pass --since N to only see messages newer than sequence N. After seeing a message you want to read fully, use collab_read with the seq number.",
  "command": "collab check --since ${since:-0}",
  "parameters": {
    "since": {
      "type": "number",
      "description": "Only show messages with sequence number greater than this. Track the highest seq you've seen and pass it here to avoid re-reading old messages.",
      "required": false,
      "default": 0
    }
  }
}
```

## Tool: `collab_read`

Fetches the full body of a specific message. Use after `collab_check` identifies
a message you need to read in detail.

```json
{
  "name": "collab_read",
  "description": "Read the full body of a message by its sequence number. Use this after collab_check shows a message summary you need to read in detail. Returns the complete message content.",
  "command": "collab read ${seq}",
  "parameters": {
    "seq": {
      "type": "number",
      "description": "The sequence number of the message to read.",
      "required": true
    }
  }
}
```

## Tool: `collab_send`

Sends a message to another agent. The body is piped via stdin.

```json
{
  "name": "collab_send",
  "description": "Send a message to another agent. Types: inquiry (ask a question), reply (answer a question), proposal (suggest a design/approach), review (review code or a proposal), info (share information). The --summary flag is REQUIRED and must be a concise one-line description — other agents see ONLY this summary when polling, so make it descriptive. Use --re to reference a previous message seq number when replying. The message body is provided via stdin.",
  "command": "echo '${body}' | collab send --to ${to} --type ${type} --summary '${summary}' ${re:+--re ${re}}",
  "parameters": {
    "to": {
      "type": "string",
      "description": "The name of the recipient agent.",
      "required": true
    },
    "type": {
      "type": "string",
      "description": "Message type: inquiry, reply, proposal, review, or info.",
      "required": true,
      "default": "inquiry"
    },
    "summary": {
      "type": "string",
      "description": "REQUIRED. A concise one-line summary of the message. This is what other agents see when polling — make it descriptive enough to decide whether to read the full message.",
      "required": true
    },
    "body": {
      "type": "string",
      "description": "The full message body in markdown. Be concise but clear.",
      "required": true
    },
    "re": {
      "type": "number",
      "description": "Sequence number of the message this replies to. Omit for new threads.",
      "required": false
    }
  }
}
```

## Tool: `collab_resolve`

Marks a conversation thread as resolved.

```json
{
  "name": "collab_resolve",
  "description": "Mark a message as resolved. Use this when a question has been answered or a discussion thread is complete. This helps both agents know which topics are still open.",
  "command": "collab resolve ${seq}",
  "parameters": {
    "seq": {
      "type": "number",
      "description": "The sequence number of the message to mark as resolved.",
      "required": true
    }
  }
}
```

---

## Agent System Prompt Addition

Add this to each agent's system prompt (customize the agent name):

```
You are collaborating with another AI agent on this codebase. Use the collab_*
tools to communicate:

1. **collab_check** — Poll for new messages periodically (especially before
   starting new work and after completing a task). Track the highest seq number
   you've seen and pass it as --since to avoid re-reading.

2. **collab_read <seq>** — Read a full message when the summary from check
   suggests it's relevant to your current work.

3. **collab_send** — Send messages to the other agent. ALWAYS include a clear
   --summary. Message types:
   - inquiry: Ask a question or request input
   - reply: Respond to a specific message (use --re)
   - proposal: Suggest an approach or design
   - review: Provide feedback on code or a proposal
   - info: Share information proactively

4. **collab_resolve <seq>** — Mark threads as resolved when done.

Guidelines:
- Be concise. The other agent pays per token too.
- Write summaries that are informative enough to triage without reading the body.
- Reference specific seq numbers with --re to maintain thread structure.
- Check for messages before starting new work to avoid conflicts.
- Proactively share relevant decisions or discoveries via 'info' messages.
```

---

## Session Launch

```bash
# Terminal 1 — Agent A
export COLLAB_AGENT=agent-a
export COLLAB_TASK=auth-middleware
opencode  # with collab tools configured

# Terminal 2 — Agent B
export COLLAB_AGENT=agent-b
export COLLAB_TASK=auth-middleware
opencode  # with collab tools configured

# Initialize (run once from project root)
collab init --agents agent-a,agent-b
```
