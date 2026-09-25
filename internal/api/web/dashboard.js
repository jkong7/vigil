const PALETTE = ["#4f46e5", "#0891b2", "#16a34a", "#d97706", "#db2777", "#7c3aed", "#0d9488", "#ea580c", "#2563eb", "#65a30d"];
const $ = (id) => document.getElementById(id);
const state = { sessions: [], colors: {} };

const el = (tag, cls, text) => {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined && text !== null) e.textContent = text;
  return e;
};

const ago = (iso) => {
  if (!iso || iso.startsWith("0001")) return "never";
  const s = Math.max(0, Math.round((Date.now() - new Date(iso)) / 1000));
  if (s < 60) return "just now";
  if (s < 3600) return Math.round(s / 60) + "m ago";
  if (s < 86400) return Math.round(s / 3600) + "h ago";
  return Math.round(s / 86400) + "d ago";
};
const span = (a, b) => {
  const m = Math.round((new Date(b) - new Date(a)) / 60000);
  return m < 60 ? m + " min" : (m / 60).toFixed(1) + " h";
};
const compact = (n) => (n >= 1e6 ? (n / 1e6).toFixed(1) + "M" : n >= 1e3 ? Math.round(n / 1e3) + "k" : String(n));

const primaryRepo = (s) => {
  const entries = Object.entries(s.repos || {});
  if (!entries.length) return (s.project || "").split("/").pop() || "?";
  entries.sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  return entries[0][0];
};

const color = (repo) => {
  if (!state.colors[repo]) state.colors[repo] = PALETTE[Object.keys(state.colors).length % PALETTE.length];
  return state.colors[repo];
};

async function getJSON(url) {
  const r = await fetch(url);
  if (!r.ok) throw Object.assign(new Error(url), { status: r.status });
  return r.json();
}

function copyButton(text) {
  const b = el("button", "copy", "Copy");
  b.title = text;
  b.onclick = async () => {
    try { await navigator.clipboard.writeText(text); b.textContent = "Copied"; } catch { b.textContent = "Failed"; }
    setTimeout(() => (b.textContent = "Copy"), 1500);
  };
  return b;
}

function renderToday(plan) {
  const list = $("today");
  const items = plan.suggestions || [];
  if (!items.length) {
    list.replaceChildren(el("li", "empty", "Nothing pressing. Pick something fun."));
  } else {
    list.replaceChildren(...items.slice(0, 10).map((s, i) => {
      const li = el("li");
      li.append(el("span", "rank", i + 1));
      const body = el("div");
      const title = el("div", "title");
      title.append(el("span", "tag " + s.kind, s.kind));
      title.append(document.createTextNode(s.title));
      if (s.repo && s.kind !== "resume") {
        title.append(document.createTextNode(" "));
        title.append(el("span", "repo-chip", s.repo));
      }
      body.append(title, el("div", "why", s.why));
      li.append(body);
      li.append(s.action ? copyButton(s.action) : el("span"));
      return li;
    }));
  }
  const focus = Object.entries(plan.focus || {}).sort((a, b) => b[1] - a[1]).slice(0, 3);
  $("focus").textContent = focus.length ? "last 72h: " + focus.map(([r, n]) => `${r} (${n})`).join(", ") : "";
  const live = $("pill-live");
  live.textContent = `${plan.live_sessions} live · ${plan.busy_sessions} busy`;
}

function renderLive(sessions) {
  const live = sessions.filter((s) => s.live);
  $("live-count").textContent = live.length ? `${live.length} running` : "";
  $("live").replaceChildren(...(live.length ? live.map((s) => {
    const li = el("li");
    const name = el("div", "name");
    name.append(el("span", "dot " + (s.live.status === "busy" ? "busy" : "")));
    name.append(document.createTextNode(s.title || s.live.name || s.id.slice(0, 8)));
    li.append(name);
    li.append(el("div", "sub", `${primaryRepo(s)} · ${s.live.status} · ${s.prompts} prompts · ${ago(s.last_active || s.live.updated)}`));
    if (s.last_prompt) li.append(el("div", "sub", "› " + s.last_prompt));
    return li;
  }) : [el("li", "empty", "No Claude Code sessions running.")]));
}

function renderActivity(days) {
  const max = Math.max(1, ...days.map((d) => d.prompts));
  const repos = new Set();
  $("activity").replaceChildren(...days.map((d) => {
    const col = el("div", "day");
    col.append(el("div", "n", d.prompts || ""));
    const stack = el("div", "stack");
    const parts = Object.entries(d.projects || {}).sort((a, b) => b[1] - a[1]);
    for (const [repo, n] of parts) {
      repos.add(repo);
      const seg = el("span");
      seg.style.height = (n / max) * 100 + "%";
      seg.style.background = color(repo);
      seg.title = `${repo}: ${n} prompts`;
      stack.append(seg);
    }
    col.append(stack);
    const dt = new Date(d.date + "T12:00:00");
    col.append(el("div", "lbl", dt.toLocaleDateString(undefined, { weekday: "short" }).slice(0, 2) + " " + dt.getDate()));
    col.title = `${d.date}: ${d.prompts} prompts in ${d.sessions} sessions`;
    return col;
  }));
  $("legend").replaceChildren(...[...repos].map((r) => {
    const s = el("span");
    const sw = el("i");
    sw.style.background = color(r);
    s.append(sw, document.createTextNode(r));
    return s;
  }));
}

function renderRepos(repos) {
  $("repos").tBodies[0].replaceChildren(...repos.map((r) => {
    const tr = el("tr");
    const sync = !r.upstream ? el("span", "muted", "no upstream")
      : r.ahead && r.behind ? el("span", "bad", `↑${r.ahead} ↓${r.behind}`)
      : r.ahead ? el("span", "warn", `↑${r.ahead}`)
      : r.behind ? el("span", "warn", `↓${r.behind}`)
      : el("span", "ok", "in sync");
    const changes = r.changed + r.untracked;
    const cells = [
      el("strong", null, r.name),
      el("span", null, r.branch),
      sync,
      changes ? el("span", "warn", `${r.changed} modified, ${r.untracked} new`) : el("span", "muted", "clean"),
      el("span", null, `${r.commits_today} / ${r.commits_week}`),
    ];
    for (const c of cells) { const td = el("td"); td.append(c); tr.append(td); }
    const last = el("td", "subject", `${r.last_subject || ""} · ${ago(r.last_commit)}`);
    tr.append(last);
    return tr;
  }));
}

function renderMonitors(monitors) {
  const down = monitors.filter((m) => m.status === "down").length;
  const pill = $("pill-monitors");
  pill.textContent = monitors.length ? (down ? `${down} down` : `${monitors.length} monitors up`) : "no monitors";
  pill.className = "pill " + (down ? "bad" : monitors.length ? "ok" : "");
  $("monitors").replaceChildren(...(monitors.length ? monitors.map((m) => {
    const li = el("li");
    const name = el("span");
    name.append(el("span", "dot " + m.status), document.createTextNode(m.monitor.name));
    li.append(name, el("span", "muted small", m.checks ? `${(m.uptime * 100).toFixed(1)}% · ${Math.round(m.avg_latency_ms)} ms` : "pending"));
    return li;
  }) : [el("li", "empty", "Add monitors in vigil.yaml.")]));
}

function renderHistory() {
  const q = $("q").value.trim().toLowerCase();
  const repo = $("repo-filter").value;
  const rows = state.sessions.filter((s) => {
    if (repo && primaryRepo(s) !== repo) return false;
    if (!q) return true;
    return [s.title, s.first_prompt, s.last_prompt].some((t) => (t || "").toLowerCase().includes(q));
  }).slice(0, 200);
  $("history").replaceChildren(...(rows.length ? rows.map((s) => {
    const li = el("li");
    const t = el("div", "t");
    if (s.live) t.append(el("span", "dot " + (s.live.status === "busy" ? "busy" : "")));
    t.append(document.createTextNode(s.title || s.first_prompt || s.id));
    li.append(t);
    const tokens = (s.usage && s.usage.output) || 0;
    li.append(el("div", "sub",
      `${primaryRepo(s)} · ${ago(s.last_active)} · ${span(s.started, s.last_active)} · ${s.prompts} prompts · ${compact(tokens)} output tokens`));
    const detail = el("div", "detail");
    if (s.first_prompt) detail.append(el("div", null, "First: " + s.first_prompt));
    if (s.last_prompt && s.last_prompt !== s.first_prompt) detail.append(el("div", null, "Last: " + s.last_prompt));
    const tools = Object.entries(s.tools || {}).sort((a, b) => b[1] - a[1]).slice(0, 6).map(([k, v]) => `${k} ${v}`).join(", ");
    if (tools) detail.append(el("div", null, "Tools: " + tools));
    const resume = el("div");
    const cmd = `cd ${s.project} && claude --resume ${s.id}`;
    resume.append(el("code", null, cmd), document.createTextNode(" "), copyButton(cmd));
    detail.append(resume);
    li.append(detail);
    li.onclick = (e) => { if (e.target.tagName !== "BUTTON") li.classList.toggle("open"); };
    return li;
  }) : [el("li", "empty", "No sessions match.")]));
}

function renderBacklog(items) {
  $("backlog-count").textContent = `${items.length} open`;
  const groups = {};
  for (const it of items) (groups[it.repo] ||= []).push(it);
  const out = [];
  for (const [repo, list] of Object.entries(groups)) {
    out.push(el("h3", null, repo));
    const ul = el("ul");
    for (const it of list) {
      const li = el("li", null, it.text);
      li.title = `${it.file}:${it.line}${it.section ? " · " + it.section : ""}`;
      ul.append(li);
    }
    out.push(ul);
  }
  $("backlog").replaceChildren(...(out.length ? out : [el("div", "empty", "No open checklist items found.")]));
}

function fillRepoFilter() {
  const sel = $("repo-filter");
  const current = sel.value;
  const repos = [...new Set(state.sessions.map(primaryRepo))].sort();
  sel.replaceChildren(el("option", null, "All repos"), ...repos.map((r) => { const o = el("option", null, r); o.value = r; return o; }));
  sel.options[0].value = "";
  sel.value = repos.includes(current) ? current : "";
}

async function refresh() {
  $("date").textContent = new Date().toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric" });
  getJSON("/api/monitors").then(renderMonitors).catch(() => {});
  try {
    const [plan, sessions, activity, repos, backlog] = await Promise.all([
      getJSON("/api/today"), getJSON("/api/sessions?limit=1000"), getJSON("/api/activity?days=14"),
      getJSON("/api/repos"), getJSON("/api/backlog"),
    ]);
    state.sessions = sessions;
    renderToday(plan);
    renderLive(sessions);
    renderActivity(activity);
    renderRepos(repos);
    fillRepoFilter();
    renderHistory();
    renderBacklog(backlog);
  } catch (e) {
    if (e.status === 404) document.querySelectorAll(".wb").forEach((n) => n.classList.add("hidden"));
  }
}

$("q").addEventListener("input", renderHistory);
$("repo-filter").addEventListener("change", renderHistory);
refresh();
setInterval(refresh, 15000);
