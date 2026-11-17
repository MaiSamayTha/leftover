// Leftover window. Plain JavaScript, no dependencies, nothing loaded from
// the internet. The token comes from the URL fragment and is dropped from
// the address bar right away.
"use strict";

const token = location.hash.slice(1);
history.replaceState(null, "", location.pathname);

async function api(path, body) {
  const res = await fetch("/api/" + path, {
    method: body === undefined ? "GET" : "POST",
    headers: { "X-Leftover-Token": token, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    let msg = res.statusText;
    try { msg = (await res.json()).error || msg; } catch (_) {}
    throw new Error(msg);
  }
  return res.status === 204 || res.status === 202 ? null : res.json();
}

const $ = (sel) => document.querySelector(sel);

// ---------- formatting ----------

function bytes(n) {
  if (n < 1000) return n + " B";
  const units = ["KB", "MB", "GB", "TB"];
  let i = -1;
  do { n /= 1000; i++; } while (n >= 1000 && i < units.length - 1);
  return (n >= 100 ? n.toFixed(0) : n.toFixed(1)) + " " + units[i];
}

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

function tilde(p) {
  const home = state.home;
  if (home && (p === home || p.startsWith(home + "/") || p.startsWith(home + "\\"))) return "~" + p.slice(home.length);
  return p;
}

function ago(days) {
  if (days <= 0) return "today";
  if (days === 1) return "yesterday";
  if (days < 60) return days + " days ago";
  if (days < 730) return Math.round(days / 30) + " months ago";
  return Math.round(days / 365) + " years ago";
}

const VERDICTS = {
  rebuild: { label: "Rebuilds itself", cls: "safe" },
  redownload: { label: "Downloads again", cls: "safe" },
  review: { label: "Worth a look", cls: "review" },
  keep: { label: "Your data", cls: "keep" },
};
const cleanable = (it) => it.verdict === "rebuild" || it.verdict === "redownload";

// ---------- state ----------

const state = {
  view: "overview",
  scan: null,
  vault: [],
  settings: {},
  status: { running: false },
  home: "",
  selected: new Set(),
  focused: null,
  query: "",
  meta: {},
  rules: {},
};

const TITLES = { overview: "Overview", safe: "Safe to clear", review: "Worth a look", keep: "Your data", vault: "Vault", history: "History", settings: "Settings" };

async function refresh() {
  const s = await api("state");
  state.scan = s.scan;
  state.vault = s.vault;
  state.settings = s.settings;
  state.status = s.status;
  state.home = s.home;
  state.meta = s;
  state.rules = s.rules || {};
  $("#version").textContent = "Version " + s.version;
  render();
  if (s.status.running) pollScan();
}

function items() { return (state.scan && state.scan.items) || []; }

function itemsFor(view) {
  return items().filter((it) => {
    if (view === "safe") return cleanable(it);
    if (view === "review") return it.verdict === "review";
    if (view === "keep") return it.verdict === "keep";
    return true;
  });
}

function sum(list) { return list.reduce((n, it) => n + it.bytes, 0); }

// label prefers the specific name (a model, an Xcode version) over the
// rule's generic one.
function label(it) { return it.label || it.name; }

// ---------- rendering ----------

function render() {
  document.querySelectorAll(".nav").forEach((a) => a.classList.toggle("active", a.dataset.view === state.view));
  $("#title").textContent = TITLES[state.view];
  for (const v of ["safe", "review", "keep"]) {
    const list = itemsFor(v);
    $("#count-" + v).textContent = list.length ? bytes(sum(list)) : "";
  }
  const held = sum(state.vault);
  $("#count-vault").textContent = state.vault.length ? bytes(held) : "";
  const listView = ["safe", "review", "keep"].includes(state.view);
  $("#search").hidden = !listView;
  $("#scan-btn").textContent = state.status.running ? "Stop" : state.scan ? "Scan again" : "Scan";

  if (state.status.running) return renderScanning();
  if (!state.scan && ["overview", "safe", "review", "keep"].includes(state.view)) return renderEmpty();

  const views = { overview: renderOverview, safe: renderList, review: renderList, keep: renderList, vault: renderVault, history: renderHistory, settings: renderSettings };
  (views[state.view] || renderOverview)();
  renderInspector();
  renderActionBar();
}

// ---------- cleaning ----------

function selectedItems() { return items().filter((it) => state.selected.has(it.id)); }

function renderActionBar() {
  const sel = selectedItems();
  const bar = $("#actionbar");
  bar.hidden = state.view !== "safe" || sel.length === 0;
  if (bar.hidden) return;
  $("#selection-summary").innerHTML = `<b>${sel.length} selected</b> · ${bytes(sum(sel))}`;
}

function sheet({ title, body, ok, danger }) {
  return new Promise((resolve) => {
    $("#sheet-title").textContent = title;
    $("#sheet-body").innerHTML = body;
    const okBtn = $("#sheet-ok");
    okBtn.textContent = ok;
    okBtn.classList.toggle("btn-danger-fill", !!danger);
    $("#sheet").hidden = false;
    okBtn.focus();
    const done = (v) => {
      $("#sheet").hidden = true;
      okBtn.onclick = $("#sheet-cancel").onclick = null;
      document.removeEventListener("keydown", onKey);
      resolve(v);
    };
    const onKey = (e) => { if (e.key === "Escape") done(false); };
    document.addEventListener("keydown", onKey);
    okBtn.onclick = () => done(true);
    $("#sheet-cancel").onclick = () => done(false);
  });
}

async function clean() {
  const sel = selectedItems();
  if (!sel.length) return;
  const days = state.settings.keep_days || 14;
  const okd = await sheet({
    title: `Move ${sel.length} item${sel.length === 1 ? "" : "s"} to the vault?`,
    body: `<p>${bytes(sum(sel))} will be set aside, not deleted. You can put any of it back for ${days} days.</p>
      <ul class="sheet-list">${sel.slice(0, 6).map((it) => `<li><span>${esc(label(it))}</span><span>${bytes(it.bytes)}</span></li>`).join("")}${sel.length > 6 ? `<li class="muted">and ${sel.length - 6} more</li>` : ""}</ul>`,
    ok: "Move to Vault",
  });
  if (!okd) return;
  const btn = $("#clean-btn");
  btn.disabled = true;
  try {
    const out = await api("clean", { ids: sel.map((it) => it.id) });
    const failed = out.filter((o) => o.error);
    const moved = out.filter((o) => !o.error).reduce((n, o) => n + o.bytes, 0);
    state.selected.clear();
    state.focused = null;
    await refresh();
    toast(failed.length ? `Moved ${bytes(moved)}. ${failed.length} couldn't be moved: ${failed[0].error}` : `Moved ${bytes(moved)} to the vault.`);
  } catch (e) {
    toast(e.message);
  } finally {
    btn.disabled = false;
  }
}

// ---------- vault ----------

function daysLeft(e) {
  const ms = new Date(e.expires) - Date.now();
  if (ms <= 0) return "Expired";
  const d = Math.ceil(ms / 864e5);
  return d === 1 ? "1 day left" : d + " days left";
}

function renderVault() {
  $("#inspector").hidden = true;
  const v = state.vault;
  const total = sum(v);
  if (!v.length) {
    $("#view").innerHTML = `
      <div class="empty">
        <h2>The vault is empty.</h2>
        <p>When you clear something, it waits here for ${state.settings.keep_days || 14} days so you can change your mind. Nothing is deleted until then.</p>
      </div>`;
    return;
  }
  const expired = v.filter((e) => new Date(e.expires) <= Date.now());
  $("#view").innerHTML = `
    <div class="vault-head">
      <div>
        <div class="hero-number small-hero">${bytes(total)}</div>
        <div class="muted">held in the vault. The space comes back when it's emptied${state.settings.auto_empty === false ? "" : ", which happens automatically as items expire"}.</div>
      </div>
      <div class="vault-actions">
        ${expired.length ? `<button class="btn" data-act="empty-expired">Empty expired (${bytes(sum(expired))})</button>` : ""}
        <button class="btn btn-danger" data-act="empty-all">Empty Vault…</button>
      </div>
    </div>
    <div class="table">
      ${v.map((e) => `
        <div class="tr tr-vault">
          <span class="dot dot-safe"></span>
          <span class="name">${esc(e.name)}<small>${esc(e.tool)}${e.state === "moving" ? " · interrupted" : ""}</small></span>
          <span class="where">${esc(tilde(e.project || e.paths[0]))}</span>
          <span class="r size">${bytes(e.bytes)}</span>
          <span class="r muted small">${daysLeft(e)}</span>
          <span class="r"><button class="btn btn-small" data-restore="${esc(e.id)}">Put Back</button></span>
        </div>`).join("")}
    </div>`;
}

async function restore(id) {
  try {
    const out = await api("restore", { id });
    const e = out.entry;
    const beside = out.restored.some((p, i) => p !== e.paths[i]);
    await refresh();
    toast(beside ? `Put back next to the copy your tools already rebuilt.` : `Put back ${e.name}. Scan again to see it in your lists.`);
  } catch (err) { toast(err.message); }
}

async function emptyVault(expiredOnly) {
  const list = expiredOnly ? state.vault.filter((e) => new Date(e.expires) <= Date.now()) : state.vault;
  const okd = await sheet({
    title: expiredOnly ? "Empty expired items?" : "Empty the vault?",
    body: `<p>${bytes(sum(list))} will be deleted for good. This can't be undone.</p>`,
    ok: "Empty",
    danger: true,
  });
  if (!okd) return;
  try {
    const out = await api("empty", expiredOnly ? { expired: true } : { all: true });
    await refresh();
    toast(`Freed ${bytes(out.freed)}.`);
  } catch (err) { toast(err.message); }
}

// ---------- history ----------

async function renderHistory() {
  $("#inspector").hidden = true;
  $("#view").innerHTML = `<p class="muted">Loading…</p>`;
  const events = await api("history");
  if (state.view !== "history") return;
  const verbs = { move: "Moved to vault", restore: "Put back", empty: "Deleted for good" };
  $("#view").innerHTML = events.length ? `
    <p class="intro">Everything Leftover has changed on this computer, newest first. Scans aren't listed because they only read.</p>
    <div class="table">
      ${events.map((ev) => `
        <div class="tr tr-history">
          <span class="muted small">${new Date(ev.at).toLocaleString([], { dateStyle: "medium", timeStyle: "short" })}</span>
          <span class="name">${esc(verbs[ev.action] || ev.action)}<small>${esc(ev.name)}</small></span>
          <span class="where">${esc(tilde((ev.paths || [])[0] || ""))}${ev.paths && ev.paths.length > 1 ? ` +${ev.paths.length - 1}` : ""}</span>
          <span class="r size">${bytes(ev.bytes)}</span>
        </div>`).join("")}
    </div>` : `<div class="empty"><h2>Nothing yet.</h2><p>Leftover hasn't changed anything on this computer.</p></div>`;
}

function renderEmpty() {
  $("#inspector").hidden = true;
  $("#view").innerHTML = `
    <div class="empty">
      <img src="icon.svg" alt="">
      <h2>See what your tools left behind.</h2>
      <p>Leftover looks through caches, build folders and AI models, explains each one, and clears what's safe.</p>
      <button class="btn btn-primary" data-act="scan">Scan this computer</button>
      <div class="fine">Scanning only reads. Nothing is moved until you choose.</div>
    </div>`;
}

function renderScanning() {
  $("#inspector").hidden = true;
  const p = state.status.progress || {};
  const phase = { starting: "Getting ready", locations: "Checking tool caches", projects: "Looking through your projects", measuring: "Measuring what was found", done: "Finishing up" }[p.phase] || "Scanning";
  $("#view").innerHTML = `
    <div class="scanning">
      <div class="spinner"></div>
      <div class="big">${bytes(p.bytes || 0)}</div>
      <div class="sub">${esc(phase)} · ${(p.folders || 0).toLocaleString()} folders · ${p.found || 0} found</div>
      <div class="path">${esc(tilde(p.current || ""))}</div>
    </div>`;
}

const GROUP_COLORS = ["#0071e3", "#34c759", "#ff9f0a", "#af52de", "#ff375f", "#5ac8fa", "#ffcc00", "#8e8e93"];

function renderOverview() {
  const safe = itemsFor("safe");
  const safeBytes = sum(safe);
  const byGroup = new Map();
  for (const it of safe) byGroup.set(it.group, (byGroup.get(it.group) || 0) + it.bytes);
  const groups = [...byGroup.entries()].sort((a, b) => b[1] - a[1]);
  const top = safe.slice(0, 6);
  const idle = safe.filter((it) => it.project && it.idle_days >= 90);
  const s = state.scan;
  const took = (s.took_ns / 1e9).toFixed(1);
  const when = new Date(s.started).toLocaleString([], { dateStyle: "medium", timeStyle: "short" });

  $("#view").innerHTML = `
    <div class="hero">
      <div class="hero-number">${bytes(safeBytes)}</div>
      <div class="hero-label">can be cleared safely</div>
      <div class="bar" role="img" aria-label="Space by tool">
        ${groups.map(([g, n], i) => `<span data-flex="${n}" data-color="${GROUP_COLORS[i % GROUP_COLORS.length]}" title="${esc(g)} · ${bytes(n)}"></span>`).join("")}
      </div>
      <div class="legend">
        ${groups.map(([g, n], i) => `<span><i data-color="${GROUP_COLORS[i % GROUP_COLORS.length]}"></i>${esc(g)} <b>${bytes(n)}</b></span>`).join("")}
      </div>
      <div class="hero-actions">
        ${safe.length ? `<button class="btn btn-primary" data-act="review-safe">Review ${safe.length} item${safe.length === 1 ? "" : "s"}</button>` : ""}
        <span class="meta">Scanned ${esc(when)} · ${s.folders.toLocaleString()} folders in ${took}s</span>
      </div>
    </div>

    <div class="cards">
      <div class="card">
        <h3>Biggest</h3>
        ${top.length ? top.map(rowMini).join("") : `<p class="muted">Nothing safe to clear right now. Nice.</p>`}
      </div>
      <div class="card">
        <h3>Projects you haven't touched in 3 months</h3>
        ${idle.length ? idle.slice(0, 6).map(rowMini).join("") : `<p class="muted">Every project with leftovers has been worked on recently.</p>`}
      </div>
    </div>

    <div class="trust">
      <i class="ico ico-lock"></i>
      <div><b>Everything stays on this computer.</b> Leftover has no account, sends nothing anywhere, and only moves things you pick into a vault you can restore from for ${state.settings.keep_days || 14} days.</div>
    </div>`;
  applyStyles($("#view"));
}

// The page's security policy forbids inline style attributes, so sizes and
// colours are carried in data attributes and applied through the DOM.
function applyStyles(root) {
  root.querySelectorAll("[data-flex]").forEach((el) => { el.style.flex = el.dataset.flex; });
  root.querySelectorAll("[data-color]").forEach((el) => { el.style.background = el.dataset.color; });
}

function rowMini(it) {
  const where = it.project || it.paths[0];
  return `<button class="mini" data-focus="${it.id}">
    <span class="mini-name">${esc(label(it))}<small>${esc(tilde(where))}</small></span>
    <span class="mini-size">${bytes(it.bytes)}</span>
  </button>`;
}

function matches(it) {
  if (!state.query) return true;
  const q = state.query.toLowerCase();
  return [it.name, it.tool, it.kind, it.project, ...it.paths].some((s) => (s || "").toLowerCase().includes(q));
}

function renderList() {
  const list = itemsFor(state.view).filter(matches);
  const intro = {
    safe: "These come back on their own: your tools rebuild them or download them again when needed.",
    review: "Leftover knows what these are but can't promise they come back, so it only shows them. Each one explains how to check.",
    keep: "This is your data. It's listed so you can see where the space goes, and Leftover never moves it.",
  }[state.view];
  const selectable = state.view === "safe";
  const allOn = selectable && list.length > 0 && list.every((it) => state.selected.has(it.id));

  $("#view").innerHTML = `
    <p class="intro">${intro}</p>
    ${list.length === 0 ? `<p class="muted">${state.query ? "Nothing matches your search." : "Nothing here."}</p>` : `
    <div class="table">
      <div class="thead">
        ${selectable ? `<label class="check"><input type="checkbox" data-act="select-all" ${allOn ? "checked" : ""} aria-label="Select all"></label>` : `<span></span>`}
        <span>Name</span><span>Where</span><span class="r">Size</span>
      </div>
      ${list.map((it) => row(it, selectable)).join("")}
    </div>`}`;
}

function row(it, selectable) {
  const where = it.project || it.paths[0];
  const v = VERDICTS[it.verdict];
  const multi = it.paths.length > 1 ? ` · ${it.paths.length} folders` : "";
  const idle = it.project && it.idle_days >= 30 ? `<span class="tag">untouched ${ago(it.idle_days)}</span>` : "";
  const warn = it.notes && it.notes.length ? `<span class="tag tag-warn">!</span>` : "";
  return `<div class="tr ${state.focused === it.id ? "focused" : ""}" data-focus="${it.id}">
    ${selectable ? `<label class="check"><input type="checkbox" data-select="${it.id}" ${state.selected.has(it.id) ? "checked" : ""} aria-label="Select ${esc(it.name)}"></label>` : `<span class="dot dot-${v.cls}"></span>`}
    <span class="name">${esc(label(it))}<small>${esc(it.label ? it.name : it.tool)}${multi}</small></span>
    <span class="where">${esc(tilde(where))}${idle}${warn}</span>
    <span class="r size">${bytes(it.bytes)}</span>
  </div>`;
}

function renderInspector() {
  const el = $("#inspector");
  const it = items().find((x) => x.id === state.focused);
  if (!it || !["overview", "safe", "review", "keep"].includes(state.view)) { el.hidden = true; return; }
  const v = VERDICTS[it.verdict];
  const rule = state.rules[it.rule] || {};
  el.hidden = false;
  el.innerHTML = `
    <button class="close" data-act="close-inspector" aria-label="Close">×</button>
    <div class="pill pill-${v.cls}">${v.label}</div>
    <h2>${esc(label(it))}</h2>
    <div class="muted">${esc(it.tool)} · ${esc(it.kind)}</div>
    <div class="size-big">${bytes(it.bytes)}</div>
    <div class="muted small">${it.files.toLocaleString()} files${it.idle_days ? ` · last changed ${ago(it.idle_days)}` : ""}</div>
    ${(it.notes || []).map((n) => `<div class="note">${esc(n)}</div>`).join("")}
    <h4>What it is</h4><p>${esc(rule.what || "")}</p>
    ${rule.comeback ? `<h4>${cleanable(it) ? "How it comes back" : "Before you remove it"}</h4><p>${esc(rule.comeback)}</p>` : ""}
    ${rule.command ? `<h4>The tool's own command</h4><code class="cmd">${esc(rule.command)}</code>` : ""}
    ${it.details && it.details.length ? `<h4>Inside</h4><div class="details">${it.details.map((d) => `
      <div class="detail"><span>${esc(d.name)}${d.shared ? ` <small>shares layers</small>` : ""}${d.reclaimable ? ` <small>${bytes(d.reclaimable)} reclaimable</small>` : ""}</span><span>${bytes(d.bytes)}</span></div>`).join("")}</div>` : ""}
    <h4>Location</h4>
    ${it.paths.map((p) => `<div class="path-row"><code>${esc(tilde(p))}</code><button class="btn-link" data-reveal="${esc(p)}">Show</button></div>`).join("")}
    ${rule.source ? `<h4>Source</h4><p><a href="${esc(rule.source)}" target="_blank" rel="noopener noreferrer">${esc(rule.source.replace(/^https:\/\//, ""))}</a></p>` : ""}
    <div class="inspector-actions">
      <button class="btn btn-small" data-ignore="${esc(it.project || it.paths[0])}">Ignore this folder</button>
    </div>`;
}

// ---------- settings ----------

function renderSettings() {
  $("#inspector").hidden = true;
  const st = state.settings;
  const keep = st.keep_days || 14;
  const auto = st.auto_empty !== false;
  const ignored = st.ignore || [];
  $("#view").innerHTML = `
    <div class="settings">
      <div class="group">
        <div class="setting">
          <div><b>Keep cleared items for</b><small>How long the vault holds things before they can be emptied.</small></div>
          <select id="keep-days">${[3, 7, 14, 30, 60, 90].map((d) => `<option value="${d}" ${d === keep ? "selected" : ""}>${d} days</option>`).join("")}</select>
        </div>
        <div class="setting">
          <div><b>Empty expired items automatically</b><small>When a scan starts, items past their time are deleted to free the space.</small></div>
          <label class="switch"><input type="checkbox" id="auto-empty" ${auto ? "checked" : ""}><span></span></label>
        </div>
      </div>

      <h3>Ignored folders</h3>
      <div class="group">
        ${ignored.length ? ignored.map((p) => `
          <div class="setting"><code>${esc(tilde(p))}</code><button class="btn btn-small" data-unignore="${esc(p)}">Remove</button></div>`).join("")
          : `<div class="setting"><span class="muted">None. Use “Ignore this folder” on any item to keep Leftover out of it.</span></div>`}
      </div>

      <h3>Privacy</h3>
      <div class="group">
        <div class="setting setting-text">
          <p>Leftover runs entirely on this computer. It has no account, no analytics and no update checks, and it never sends anything over the network. This window is served only to this machine, on a private address with a key that changes every launch.</p>
          <p>Scanning only reads. Leftover writes to one folder of its own, for settings, the last scan, history and the vault: <code>${esc(tilde(state.meta.data_dir || ""))}</code>. Outside it, the only changes are the ones you choose, and every move, restore and deletion is listed under History.</p>
        </div>
      </div>
    </div>`;
  $("#keep-days").onchange = (e) => saveSettings({ keep_days: Number(e.target.value) });
  $("#auto-empty").onchange = (e) => saveSettings({ auto_empty: e.target.checked });
}

async function saveSettings(patch) {
  try {
    state.settings = await api("settings", patch);
    toast("Saved.");
  } catch (e) { toast(e.message); renderSettings(); }
}

// ---------- scanning ----------

let polling = false;
async function pollScan() {
  if (polling) return;
  polling = true;
  try {
    for (;;) {
      state.status = await api("status");
      if (!state.status.running) break;
      if (["overview", "safe", "review", "keep"].includes(state.view)) renderScanning();
      $("#scan-btn").textContent = "Stop";
      await new Promise((r) => setTimeout(r, 250));
    }
  } finally {
    polling = false;
  }
  if (state.status.error && !/cancel/i.test(state.status.error)) toast(state.status.error);
  state.selected.clear();
  await refresh();
}

async function scan() {
  if (state.status.running) { await api("scan/cancel", {}); return; }
  try {
    await api("scan", {});
    state.status.running = true;
    if (!["overview", "safe", "review", "keep"].includes(state.view)) go("overview");
    render();
    pollScan();
  } catch (e) { toast(e.message); }
}

// ---------- events ----------

function go(view) {
  state.view = view;
  state.focused = null;
  state.query = "";
  $("#search").value = "";
  render();
}

let toastTimer;
function toast(msg) {
  const t = $("#toast");
  t.textContent = msg;
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), 3200);
}

document.addEventListener("click", async (e) => {
  const nav = e.target.closest(".nav");
  if (nav) { e.preventDefault(); go(nav.dataset.view); return; }
  const act = e.target.closest("[data-act]");
  if (act) {
    const a = act.dataset.act;
    if (a === "scan") return scan();
    if (a === "review-safe") return go("safe");
    if (a === "close-inspector") { state.focused = null; return render(); }
    if (a === "empty-expired") return emptyVault(true);
    if (a === "empty-all") return emptyVault(false);
  }
  const un = e.target.closest("[data-unignore]");
  if (un) {
    await api("ignore", { path: un.dataset.unignore, remove: true });
    await refresh();
    toast("Leftover will scan that folder again.");
    return;
  }
  const rs = e.target.closest("[data-restore]");
  if (rs) return restore(rs.dataset.restore);
  if (e.target.closest("input[type=checkbox], label.check")) return;
  const rev = e.target.closest("[data-reveal]");
  if (rev) { api("reveal", { path: rev.dataset.reveal }).catch((err) => toast(err.message)); return; }
  const ign = e.target.closest("[data-ignore]");
  if (ign) {
    await api("ignore", { path: ign.dataset.ignore });
    state.focused = null;
    toast("Leftover will leave that folder alone.");
    return refresh();
  }
  const f = e.target.closest("[data-focus]");
  if (f) {
    state.focused = f.dataset.focus;
    const it = items().find((x) => x.id === state.focused);
    if (it && state.view === "overview") state.view = cleanable(it) ? "safe" : it.verdict;
    render();
  }
});

document.addEventListener("change", (e) => {
  const id = e.target.dataset.select;
  if (id) {
    e.target.checked ? state.selected.add(id) : state.selected.delete(id);
    render();
  } else if (e.target.dataset.act === "select-all") {
    const list = itemsFor("safe").filter(matches);
    list.forEach((it) => (e.target.checked ? state.selected.add(it.id) : state.selected.delete(it.id)));
    render();
  }
});

$("#scan-btn").addEventListener("click", scan);
$("#clean-btn").addEventListener("click", clean);
$("#clear-selection").addEventListener("click", () => { state.selected.clear(); render(); });
$("#search").addEventListener("input", (e) => { state.query = e.target.value; renderList(); });

// Keyboard: ⌘R / Ctrl+R scans, ⌘F / Ctrl+F or / searches, arrows move
// through a list, space selects, Escape closes things.
document.addEventListener("keydown", (e) => {
  const mod = e.metaKey || e.ctrlKey;
  const typing = e.target.matches("input[type=search], input[type=text], select");
  if (mod && e.key.toLowerCase() === "r") { e.preventDefault(); scan(); return; }
  if ((mod && e.key.toLowerCase() === "f") || (e.key === "/" && !typing)) {
    if (!$("#search").hidden) { e.preventDefault(); $("#search").focus(); }
    return;
  }
  if (e.key === "Escape" && $("#sheet").hidden) {
    if (typing) { e.target.blur(); return; }
    if (state.focused) { state.focused = null; render(); }
    return;
  }
  if (typing || !["safe", "review", "keep"].includes(state.view)) return;
  const list = itemsFor(state.view).filter(matches);
  if (!list.length) return;
  const i = list.findIndex((it) => it.id === state.focused);
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    e.preventDefault();
    const next = e.key === "ArrowDown" ? Math.min(i + 1, list.length - 1) : Math.max(i - 1, 0);
    state.focused = list[next].id;
    render();
    document.querySelector(`[data-focus="${state.focused}"]`)?.scrollIntoView({ block: "nearest" });
  } else if (e.key === " " && state.view === "safe" && i >= 0) {
    e.preventDefault();
    const id = list[i].id;
    state.selected.has(id) ? state.selected.delete(id) : state.selected.add(id);
    render();
  }
});

setInterval(() => api("ping", {}).catch(() => {}), 15000);
refresh().catch((e) => toast(e.message));
