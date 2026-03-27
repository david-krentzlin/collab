# Optimization ideas

## Streaming index reads for `check`

Current `List` / `ListForRecipient` behavior reads and decodes the full
`.index.jsonl` file into memory on every call, then filters in Go.

### Proposed optimization

Stream `.index.jsonl` with a JSON decoder and apply filters while reading:

- for `check --since N`: skip records with `seq <= N` immediately
- for recipient mode: skip records where `to` is neither recipient nor `all`
- exclude self-sent records early (`from == recipient`)

This avoids allocating an in-memory slice of all index records for each poll.

### Optional follow-up

Add periodic checkpoint files (e.g. `.index.offsets`) storing byte offsets every
K sequence numbers to allow seeking near `--since` instead of scanning from the
start on large histories.
