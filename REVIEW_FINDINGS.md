# collab review findings

Verdict: **NO-GO** until the critical items below are addressed.

Review scope:
- read `DESIGN.md`, `README.md`, `TOOLS.md`, and the current Go implementation
- ran `go test ./...` and `go build ./...`
- ran manual CLI smoke tests in temp directories

## Recommended work order

1. Fix sequence allocation races
2. Make `init` non-destructive
3. Stop full-directory rescans by introducing an index/journal
4. Fix recipient filtering in `check`
5. Fix broken/unsafe renderers
6. Add tests around all of the above

---

## 1) P0 — sequence allocation is unsafe under concurrency

**Where:** `internal/store/store.go:60-81`

**What is wrong**
- `NextSeq()` does a read-modify-write with no lock.
- It also uses a single shared temp path: `.seq.tmp`.
- Concurrent `send` calls can fail or reuse the same sequence.

**Observed evidence**
- A concurrent smoke test produced multiple failures like:
  - `rename .../.seq.tmp .../.seq: no such file or directory`
- The same run printed `#57` twice during concurrent sends.

**Why it matters**
- Sequence numbers are the protocol's primary ordering key.
- Once they collide or a send fails mid-flight, `read`, `resolve`, and thread reconstruction become ambiguous.

**Suggested improvement**
- Use an actual lock (`flock`, `fcntl`, or lockfile) around sequence allocation.
- If the file rewrite stays, use a unique temp file per writer, not a shared `.seq.tmp`.
- Add a concurrency regression test that launches many parallel `send`s and asserts no failures, no duplicate seqs, and no gaps caused by crashes.

---

## 2) P0 — `init` resets live state and can corrupt the timeline

**Where:** `internal/store/store.go:41-58`

**What is wrong**
- Re-running `collab init` always rewrites `.seq` to `0`.
- Existing message files are left in place, so new sends can reuse old seq values.

**Observed evidence**
- After sending `#1` and `#2`, re-running `init` allowed another message to be written as `#1`.

**Why it matters**
- This breaks the uniqueness guarantee for `seq`.
- Duplicate seqs make `read <seq>` and thread linking unreliable.

**Suggested improvement**
- Make `init` idempotent.
- If `.collab` already exists, refuse by default and require an explicit `--force` for destructive reset.
- Add tests covering repeated `init` calls on empty and non-empty stores.

---

## 3) P1 — the implementation rescans too much, and some paths are O(n²)

**Where:**
- `internal/store/store.go:112-192`
- `cmd/export.go:60-93`

**What is wrong**
- `check`, `read`, and `resolve` scan agent directories repeatedly.
- `List()` reads and parses every message file every time.
- `buildExport()` calls `List()` and then calls `ReadMessage()` once per entry, causing repeated full rescans.

**Why it matters**
- This is the main scalability issue you already suspected.
- Polling cost grows with total message count, not with new activity.
- Export/log get slower faster than they need to.

**Suggested improvement**
- Introduce an append-only index/journal, e.g. `.collab/index.jsonl`, with one compact record per message.
- `send` appends metadata once; `check --since` reads only the index.
- Maintain a `seq -> path` lookup so `read` and `resolve` do not rescan directories.
- Add a benchmark for `check` and `export` before/after the change.

**Nice side effect**
- The same index can support recipient filtering, corruption reporting, and faster rendering.

---

## 4) P1 — `check` ignores the `to` field

**Where:**
- `cmd/check.go:24-58`
- `internal/store/store.go:112-161`

**What is wrong**
- `check` excludes the caller's own messages, but it does not filter to messages actually addressed to that caller.

**Observed evidence**
- With messages between `agent-a` and `agent-b`, `agent-c` still saw both messages in `collab check`.

**Why it matters**
- Direct messages are effectively broadcast to every other agent.
- This wastes tokens and makes the `to` field mostly informational.

**Suggested improvement**
- Filter `check` to `to == $COLLAB_AGENT` by default.
- If broadcast is needed, add an explicit `to: all` / `--to all` design.
- Add CLI tests for direct, self, and broadcast traffic.

---

## 5) P1 — HTML export is broken and unsafe

**Where:** `internal/render/html.go:10-206`

**What is wrong**
- The code embeds raw JSON into a JavaScript template literal:
  - `const DATA = \`...json...\`;`
- `DATA` is then used as an object (`DATA.task`, `DATA.threads`) without `JSON.parse`.
- Raw message content can also break the script if it contains backticks, `${`, or `</script>`.

**Why it matters**
- The exported HTML is not reliable.
- Message content can corrupt the page or create an injection problem.

**Suggested improvement**
- Embed JSON in `<script type="application/json" id="collab-data">...</script>` and parse it safely.
- Or base64-encode the payload before embedding.
- Add golden tests with bodies containing code fences, `${...}`, and `</script>`.

---

## 6) P1 — plaintext thread rendering does not draw the tree correctly

**Where:** `internal/render/plaintext.go:93-170`

**What is wrong**
- Root nodes render with an empty prefix.
- Child nodes of the root never get the expected `├─` / `└─` indentation.

**Observed evidence**
- A reply rendered as another top-level entry instead of a visibly nested branch.

**Why it matters**
- The main human-facing log output does not match the design doc.

**Suggested improvement**
- Rework prefix propagation so root children always receive tree context.
- Add golden tests for single reply, forked replies, and deep nesting.

---

## 7) P2 — there are no automated tests yet

**Observed evidence**
- `go test ./...` reports `[no test files]` for every package.

**Why it matters**
- The project includes custom parsing, filesystem state transitions, threading logic, and renderers.
- Those are exactly the areas where subtle regressions are likely.

**Suggested improvement**
- Add unit tests for:
  - frontmatter marshal/unmarshal
  - thread construction
  - `check` recipient filtering
  - `init` idempotency
  - sequence allocation under concurrency
  - plaintext/HTML golden outputs

---

## 8) P2 — parsing and corruption handling are too permissive

**Where:**
- `internal/message/message.go:71-117`
- `internal/store/store.go:136-143`, `177-187`

**What is wrong**
- `Unmarshal()` does not validate required fields.
- `fmt.Sscanf` errors are ignored.
- `List()` silently skips unreadable or malformed files.
- `ReadMessage()` returns the first filename match, which becomes dangerous once seq uniqueness is violated.

**Why it matters**
- Corrupt or partially written messages can disappear silently from views.
- Operators get no signal that the store is damaged.

**Suggested improvement**
- Validate required frontmatter fields and reject malformed records explicitly.
- Surface warnings/errors instead of silently dropping bad files.
- Add tests for malformed frontmatter and partial writes.

---

## 9) P2 — `resolve` is inefficient and not written atomically

**Where:** `cmd/resolve.go:31-61`

**What is wrong**
- `resolve` reads the message by seq, then rescans the full store to rediscover its path.
- It overwrites the file directly instead of writing via temp+rename.
- The fallback path is effectively dead code.

**Why it matters**
- This adds avoidable I/O.
- A crash during rewrite can leave a truncated file.

**Suggested improvement**
- Return message path along with parsed content, or use the proposed seq index.
- Rewrite atomically.
- Add tests for resolving already-resolved and corrupted messages.

---

## 10) P2 — recipient validation is missing

**Where:** `cmd/send.go:31-88`

**What is wrong**
- `send` accepts any `--to` value.
- A typo creates a message that looks valid but is addressed to nobody meaningful.

**Why it matters**
- This is a low-friction way to lose messages silently.

**Suggested improvement**
- Validate `--to` against known agents from the store.
- If dynamic agents are desired later, gate it behind an explicit flag.

---

## 11) P2 — the hand-rolled frontmatter format is fragile

**Where:** `internal/message/message.go:47-117`

**What is wrong**
- `Marshal()` uses `%q` for some fields, but `Unmarshal()` only strips outer quotes; it does not properly unescape.
- The format will behave badly for edge cases like embedded quotes, escape sequences, or future structured fields.

**Why it matters**
- The protocol depends on durable, human-editable files.
- Parser drift will become expensive once more fields are added.

**Suggested improvement**
- Either use a small YAML library or define a stricter custom header format with explicit escaping rules.
- Add round-trip tests for tricky summaries and bodies.

---

## Suggested first implementation slice

If we want to tackle this incrementally, I would start with this order:

1. Add tests that capture the current failures:
   - concurrent sequence allocation
   - repeated `init`
   - `check` recipient filtering
   - log rendering
   - HTML export with unsafe content
2. Fix sequence allocation and make `init` safe.
3. Add an index/journal so `check`, `read`, `resolve`, `log`, and `export` stop rescanning everything.
4. Then tighten parser/validation behavior.
