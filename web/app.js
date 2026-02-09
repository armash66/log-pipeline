const els = {
  level: document.getElementById("level"),
  search: document.getElementById("search"),
  since: document.getElementById("since"),
  after: document.getElementById("after"),
  before: document.getElementById("before"),
  limit: document.getElementById("limit"),
  q: document.getElementById("q"),
  run: document.getElementById("run"),
  clear: document.getElementById("clear"),
  rows: document.getElementById("rows"),
  count: document.getElementById("count"),
  metrics: document.getElementById("metrics"),
  rate: document.getElementById("rate"),
  health: document.getElementById("health"),
  lastQuery: document.getElementById("last-query"),
  upload: document.getElementById("upload"),
  uploadFormat: document.getElementById("upload-format"),
  uploadMode: document.getElementById("upload-replace"),
  uploadBtn: document.getElementById("upload-btn"),
  uploadStatus: document.getElementById("upload-status"),
  sidebar: document.getElementById("sidebar"),
  sidebarOpen: document.getElementById("sidebar-open"),
  sidebarClose: document.getElementById("sidebar-close"),
  sidebarBackdrop: document.getElementById("sidebar-backdrop"),
};


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
  if (!logs || logs.length === 0) {
    const tr = document.createElement("tr");
    const td = document.createElement("td");
    td.colSpan = 3;
    td.textContent = "No logs yet. Run a query or upload a file.";
    td.style.color = "#a9b0bf";
    tr.appendChild(td);
    els.rows.appendChild(tr);
    return;
  }
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

function clearInputs() {
  els.level.value = "";
  els.search.value = "";
  els.since.value = "";
  els.after.value = "";
  els.before.value = "";
  els.limit.value = "50";
  els.q.value = "";
}

async function uploadFile() {
  if (!els.upload.files || els.upload.files.length === 0) {
    alert("Select a file to upload.");
    return;
  }
  const form = new FormData();
  form.append("file", els.upload.files[0]);
  form.append("format", els.uploadFormat.value || "auto");
  form.append("mode", els.uploadMode.value || "replace");

  els.uploadStatus.textContent = "Uploading...";
  const res = await fetch("/ingest/file", {
    method: "POST",
    body: form,
  });
  if (!res.ok) {
    const text = await res.text();
    els.uploadStatus.textContent = `Upload failed: ${text}`;
    throw new Error(text);
  }
  const data = await res.json();
  els.uploadStatus.textContent = `Uploaded: ${data.ingested || 0} logs`;
  if (els.uploadMode.value === "replace") {
    clearInputs();
  }
  await runQuery();
  await refreshMetrics();
}

els.run.addEventListener("click", async () => {
  try {
    await runQuery();
    await refreshMetrics();
  } catch (err) {
    alert(`Query failed: ${err.message}`);
  }
});

els.clear.addEventListener("click", clearInputs);
els.uploadBtn.addEventListener("click", async () => {
  try {
    await uploadFile();
  } catch (err) {
    alert(`Upload failed: ${err.message}`);
  }
});

function clearResults() {
  renderRows([]);
  els.count.textContent = "0 logs";
  els.rate.textContent = "rate: NA";
  els.metrics.innerHTML = "";
  els.lastQuery.textContent = "never";
}

function setPage(page) {
  document.querySelectorAll(".page").forEach((section) => {
    section.classList.add("hidden");
  });
  const active = document.getElementById(`page-${page}`);
  if (active) {
    active.classList.remove("hidden");
  }
  document.querySelectorAll(".nav-link").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.page === page);
  });
}

function openSidebar() {
  els.sidebar.classList.add("open");
  els.sidebarBackdrop.classList.add("show");
}

function closeSidebar() {
  els.sidebar.classList.remove("open");
  els.sidebarBackdrop.classList.remove("show");
}

document.querySelectorAll(".nav-link").forEach((btn) => {
  btn.addEventListener("click", () => {
    setPage(btn.dataset.page);
    closeSidebar();
  });
});

if (els.sidebarOpen) {
  els.sidebarOpen.addEventListener("click", openSidebar);
}
if (els.sidebarClose) {
  els.sidebarClose.addEventListener("click", closeSidebar);
}
if (els.sidebarBackdrop) {
  els.sidebarBackdrop.addEventListener("click", closeSidebar);
}

refreshHealth();
clearResults();
setPage("home");
