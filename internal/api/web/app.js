const BARS = 60;

const el = (tag, cls, text) => {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
};

const pct = (x) => (x * 100).toFixed(x === 1 ? 0 : 2) + "%";
const ago = (iso) => {
  const s = Math.round((Date.now() - new Date(iso)) / 1000);
  if (s < 60) return s + "s ago";
  if (s < 3600) return Math.round(s / 60) + "m ago";
  if (s < 86400) return Math.round(s / 3600) + "h ago";
  return Math.round(s / 86400) + "d ago";
};
const duration = (a, b) => {
  const m = Math.max(1, Math.round((new Date(b) - new Date(a)) / 60000));
  return m < 60 ? m + " min" : (m / 60).toFixed(1) + " h";
};

function renderMonitor(s) {
  const card = el("article", "monitor " + s.status);
  const row = el("div", "row");
  row.append(el("span", "name", s.monitor.name));
  const meta = s.checks ? `${pct(s.uptime)} uptime · ${s.avg_latency_ms.toFixed(0)} ms avg` : "waiting for first check";
  row.append(el("span", "meta", meta));
  card.append(row);
  const bars = el("div", "bars");
  const recent = (s.recent || []).slice(-BARS);
  for (let i = recent.length; i < BARS; i++) bars.append(el("span", "empty"));
  for (const ok of recent) bars.append(el("span", ok ? "" : "fail"));
  card.append(bars);
  if (s.status === "down" && s.last && s.last.error) card.append(el("div", "err", s.last.error));
  return card;
}

async function refresh() {
  try {
    const [monitors, incidents] = await Promise.all([
      fetch("/api/monitors").then((r) => r.json()),
      fetch("/api/incidents?limit=10").then((r) => r.json()),
    ]);
    const down = monitors.filter((m) => m.status === "down");
    const summary = document.getElementById("summary");
    summary.className = "summary " + (down.length ? "bad" : "ok");
    summary.textContent = down.length
      ? `${down.length} of ${monitors.length} ${monitors.length === 1 ? "service" : "services"} down`
      : "All systems operational";
    document.getElementById("monitors").replaceChildren(...monitors.map(renderMonitor));
    const list = document.getElementById("incidents");
    list.replaceChildren(
      ...(incidents.length
        ? incidents.map((i) => {
            const li = el("li");
            const status = i.resolved ? `resolved after ${duration(i.started, i.resolved)}` : "ongoing";
            li.textContent = `${i.monitor}: ${i.cause} (${ago(i.started)}, ${status})`;
            return li;
          })
        : [el("li", "meta", "No incidents recorded.")])
    );
    document.getElementById("updated").textContent = new Date().toLocaleTimeString();
  } catch (e) {
    document.getElementById("summary").textContent = "Could not reach the monitor.";
  }
}

refresh();
setInterval(refresh, 15000);
