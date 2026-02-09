const els = {
  level: document.getElementById("level"),
  search: document.getElementById("search"),
  since: document.getElementById("since"),
  after: document.getElementById("after"),
  before: document.getElementById("before"),
  limit: document.getElementById("limit"),
  q: document.getElementById("q"),
  run: document.getElementById("run"),
  poll: document.getElementById("poll"),
  rows: document.getElementById("rows"),
  count: document.getElementById("count"),
  metrics: document.getElementById("metrics"),
  rate: document.getElementById("rate"),
  health: document.getElementById("health"),
  lastQuery: document.getElementById("last-query"),
};

let pollTimer = null;

async function fetchJSON(url, options) {
  const res = await fetch(url, options);
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.json();
}

function buildQueryParams() {
  const params = new URLSearchParams();
  if (els.level.value) params.set("level", els.level.value);
  if (els.search.value) params.set("search", els.search.value);
  if (els.since.value) params.set("since", els.since.value);
  if (els.after.value) params.set("after", els.after.value);
  if (els.before.value) params.set("before", els.before.value);
  if (els.limit.value) params.set("limit", els.limit.value);
  if (els.q.value) params.set("q", els.q.value);
  return params.toString();
}

function renderRows(logs) {
  els.rows.innerHTML = "";
  logs.forEach((log) => {
    const tr = document.createElement("tr");
    const tdTime = document.createElement("td");
    tdTime.textContent = log.Timestamp || log.timestamp;
    const tdLevel = document.createElement("td");
    tdLevel.textContent = log.Level || log.level;
    tdLevel.className = `level ${tdLevel.textContent || ""}`;
    const tdMsg = document.createElement("td");
    tdMsg.textContent = log.Message || log.message;
    tr.append(tdTime, tdLevel, tdMsg);
    els.rows.appendChild(tr);
  });
}

function renderMetrics(metrics) {
  els.metrics.innerHTML = "";
  Object.keys(metrics).forEach((key) => {
    const div = document.createElement("div");
    div.textContent = `${key}: ${metrics[key]}`;
    els.metrics.appendChild(div);
  });
  if (metrics["metrics.rate_per_sec"]) {
    els.rate.textContent = `rate: ${metrics["metrics.rate_per_sec"]}`;
  }
}

async function refreshHealth() {
  try {
    const data = await fetchJSON("/health");
    els.health.textContent = data.status || "ok";
  } catch {
    els.health.textContent = "offline";
  }
}

async function runQuery() {
  const qs = buildQueryParams();
  const url = qs ? `/query?${qs}` : "/query";
  const data = await fetchJSON(url);
  renderRows(data.logs || []);
  els.count.textContent = `${data.count || 0} logs`;
  els.lastQuery.textContent = new Date().toLocaleTimeString();
}

async function refreshMetrics() {
  const data = await fetchJSON("/metrics");
  renderMetrics(data);
}

function togglePoll() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
    els.poll.textContent = "Start Live";
    return;
  }
  pollTimer = setInterval(async () => {
    await runQuery();
    await refreshMetrics();
  }, 4000);
  els.poll.textContent = "Stop Live";
}

els.run.addEventListener("click", async () => {
  try {
    await runQuery();
    await refreshMetrics();
  } catch (err) {
    alert(`Query failed: ${err.message}`);
  }
});

els.poll.addEventListener("click", togglePoll);

refreshHealth();
runQuery();
refreshMetrics();
