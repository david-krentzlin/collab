package render

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
)

// HTML renders a TaskExport as a self-contained HTML page with embedded JSON data.
func HTML(w io.Writer, export *TaskExport) error {
	data, err := json.Marshal(export)
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>collab — `)
	fmt.Fprint(w, html.EscapeString(export.Task))
	fmt.Fprint(w, `</title>
<style>
  :root {
    --bg: #1a1b26; --fg: #c0caf5; --dim: #565f89;
    --border: #292e42; --thread-bg: #1f2335;
    --agent0: #7aa2f7; --agent1: #bb9af7; --agent2: #2ac3de;
    --agent3: #9ece6a; --agent4: #e0af68;
    --open: #9ece6a; --resolved: #565f89;
    --inquiry: #7aa2f7; --reply: #c0caf5; --proposal: #e0af68;
    --review: #bb9af7; --info: #2ac3de;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: 'SF Mono', 'Cascadia Code', 'Fira Code', monospace;
    background: var(--bg); color: var(--fg);
    max-width: 960px; margin: 0 auto; padding: 2rem 1rem;
    line-height: 1.6; font-size: 14px;
  }
  h1 { color: var(--fg); font-size: 1.4rem; margin-bottom: 0.25rem; }
  .goal { color: var(--dim); margin-bottom: 2rem; }
  .thread {
    background: var(--thread-bg); border: 1px solid var(--border);
    border-radius: 8px; margin-bottom: 1.5rem; overflow: hidden;
  }
  .thread-header {
    padding: 0.75rem 1rem; border-bottom: 1px solid var(--border);
    display: flex; justify-content: space-between; align-items: center;
    cursor: pointer; user-select: none;
  }
  .thread-header:hover { background: var(--border); }
  .thread-title { font-weight: 600; }
  .thread-status {
    font-size: 0.75rem; padding: 2px 8px; border-radius: 4px;
    text-transform: uppercase; letter-spacing: 0.05em;
  }
  .thread-status.open { color: var(--open); border: 1px solid var(--open); }
  .thread-status.resolved { color: var(--resolved); border: 1px solid var(--resolved); }
  .thread-body { padding: 0.5rem 0; }
  .thread-body.collapsed { display: none; }
  .msg {
    padding: 0.5rem 1rem 0.5rem calc(1rem + var(--depth, 0) * 1.5rem);
    border-left: 3px solid transparent;
  }
  .msg:hover { background: rgba(255,255,255,0.02); }
  .msg-header {
    display: flex; gap: 0.75rem; align-items: baseline;
    margin-bottom: 0.25rem;
  }
  .msg-seq { color: var(--dim); font-size: 0.8rem; }
  .msg-agent { font-weight: 600; }
  .msg-type {
    font-size: 0.7rem; padding: 1px 6px; border-radius: 3px;
    text-transform: uppercase; letter-spacing: 0.05em;
  }
  .msg-type.inquiry { color: var(--inquiry); border: 1px solid var(--inquiry); }
  .msg-type.reply { color: var(--reply); border: 1px solid var(--reply); }
  .msg-type.proposal { color: var(--proposal); border: 1px solid var(--proposal); }
  .msg-type.review { color: var(--review); border: 1px solid var(--review); }
  .msg-type.info { color: var(--info); border: 1px solid var(--info); }
  .msg-summary { color: var(--dim); font-style: italic; font-size: 0.85rem; }
  .msg-re { color: var(--dim); font-size: 0.75rem; }
  .msg-body {
    margin-top: 0.25rem; white-space: pre-wrap;
    color: var(--fg); opacity: 0.9;
  }
  .msg-ts { color: var(--dim); font-size: 0.7rem; margin-left: auto; }
  .orphans { margin-top: 2rem; }
  .orphans h2 { color: var(--dim); font-size: 1rem; margin-bottom: 0.5rem; }
  .controls {
    margin-bottom: 1.5rem; display: flex; gap: 0.5rem; flex-wrap: wrap;
  }
  .controls button {
    background: var(--thread-bg); border: 1px solid var(--border);
    color: var(--fg); padding: 4px 12px; border-radius: 4px;
    cursor: pointer; font-family: inherit; font-size: 0.8rem;
  }
  .controls button:hover { border-color: var(--dim); }
  .controls button.active { border-color: var(--agent0); color: var(--agent0); }
</style>
</head>
<body>
<script type="application/json" id="collab-data">`)
	fmt.Fprintf(w, "%s", data)
	fmt.Fprint(w, `</script>
<script>
const DATA = JSON.parse(document.getElementById('collab-data').textContent);

const AGENT_COLORS = ['--agent0','--agent1','--agent2','--agent3','--agent4'];
const agentColorMap = {};
(DATA.agents || []).forEach((a, i) => {
  agentColorMap[a] = AGENT_COLORS[i % AGENT_COLORS.length];
});

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

function formatTs(ts) {
  if (!ts) return '';
  try {
    const d = new Date(ts);
    return d.toLocaleString(undefined, { month:'short', day:'numeric', hour:'2-digit', minute:'2-digit' });
  } catch { return ts; }
}

function renderMsg(msg, depth) {
  const color = agentColorMap[msg.from] || '--agent0';
  const re = msg.re ? '<span class="msg-re">re:#' + msg.re + '</span>' : '';
  const resolved = msg.status === 'resolved' ? ' ✓' : '';
  return '<div class="msg" style="--depth:' + depth + '">' +
    '<div class="msg-header">' +
      '<span class="msg-seq">#' + msg.seq + '</span>' +
      '<span class="msg-agent" style="color:var(' + color + ')">' + escapeHtml(msg.from) + '</span>' +
      '<span class="msg-type ' + msg.type + '">' + msg.type + '</span>' +
      re +
      '<span class="msg-ts">' + formatTs(msg.ts) + resolved + '</span>' +
    '</div>' +
    '<div class="msg-body">' + escapeHtml(msg.body || '') + '</div>' +
  '</div>';
}

function renderNode(node, depth) {
  let html = renderMsg(node.message, depth);
  if (node.children) {
    node.children.forEach(child => {
      html += renderNode(child, depth + 1);
    });
  }
  return html;
}

function renderThread(thread, idx) {
  const root = thread.root;
  const summary = escapeHtml(root.message.summary || 'Thread #' + root.message.seq);
  return '<div class="thread" data-status="' + thread.status + '">' +
    '<div class="thread-header" onclick="toggleThread(' + idx + ')">' +
      '<span class="thread-title">' + summary + '</span>' +
      '<span class="thread-status ' + thread.status + '">' + thread.status + '</span>' +
    '</div>' +
    '<div class="thread-body" id="thread-' + idx + '">' +
      renderNode(root, 0) +
    '</div>' +
  '</div>';
}

function toggleThread(idx) {
  const el = document.getElementById('thread-' + idx);
  el.classList.toggle('collapsed');
}

let showResolved = true;

function toggleResolved() {
  showResolved = !showResolved;
  document.querySelectorAll('.thread[data-status="resolved"]').forEach(el => {
    el.style.display = showResolved ? '' : 'none';
  });
  document.getElementById('btn-resolved').classList.toggle('active', !showResolved);
}

function render() {
  let html = '<h1>' + escapeHtml(DATA.task) + '</h1>';
  if (DATA.goal) html += '<div class="goal">' + escapeHtml(DATA.goal) + '</div>';

  html += '<div class="controls">';
  html += '<button id="btn-resolved" onclick="toggleResolved()">Hide Resolved</button>';
  html += '</div>';

  if (DATA.threads) {
    DATA.threads.forEach((t, i) => { html += renderThread(t, i); });
  }
  if (DATA.orphans && DATA.orphans.length > 0) {
    html += '<div class="orphans"><h2>Orphaned Messages</h2>';
    DATA.orphans.forEach(o => { html += renderMsg(o.message, 0); });
    html += '</div>';
  }
  document.body.innerHTML = html;
}

render();
</script>
</body>
</html>`)

	return nil
}
