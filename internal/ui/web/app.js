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

function since(date) {
  const s = (Date.now() - new Date(date)) / 1000;
  if (s < 60) return "just now";
  if (s < 3600) return Math.round(s / 60) + " min ago";
  if (s < 86400) { const h = Math.round(s / 3600); return h + (h === 1 ? " hour ago" : " hours ago"); }
  return new Date(date).toLocaleDateString([], { dateStyle: "medium" });
}

const plural = (n, one, many) => n + " " + (n === 1 ? one : many || one + "s");

// mono makes the two-letter monogram on an item's tile.
const itemMono = (it) => mono(/[,/]/.test(it.tool) ? it.name : it.tool);
function mono(s) {
  const w = String(s || "?").replace(/[^A-Za-z0-9]+/g, " ").trim().split(" ").filter(Boolean);
  if (w.length >= 2) return (w[0][0] + w[1][0]).toUpperCase();
  const x = w[0] || "?";
  return x[0].toUpperCase() + (x[1] || "").toLowerCase();
}

// ---------- verdicts and categories ----------

const VERDICTS = {
  rebuild: { short: "Rebuilds", long: "Rebuilds automatically", cls: "safe" },
  redownload: { short: "Re-downloads", long: "Downloads again", cls: "safe" },
  review: { short: "Review", long: "Worth a look first", cls: "review" },
  keep: { short: "Yours", long: "Your data", cls: "keep" },
};
const cleanable = (it) => it.verdict === "rebuild" || it.verdict === "redownload";

const CATS = [
  { key: "ai", label: "AI models", mono: "AI", blurb: "Local model weights and hub caches" },
  { key: "containers", label: "Containers", mono: "Dk", blurb: "Docker, OrbStack, Colima and virtual machines" },
  { key: "packages", label: "Packages", mono: "Pk", blurb: "Dependencies your lockfiles can restore" },
  { key: "build", label: "Build output", mono: "Bd", blurb: "Compiled output your builds recreate" },
  { key: "caches", label: "Caches", mono: "Ca", blurb: "Downloads and app caches your tools refill" },
];
const CAT = Object.fromEntries(CATS.map((c) => [c.key, c]));
const DOCKER = new Set(["docker-disk", "docker-engine", "orbstack-data", "colima-disk"]);

// category sorts a rule into one of the five groups the window shows.
function category(ruleID, kind) {
  const k = String(kind || "").toLowerCase();
  if (/^(model cache|dataset cache|models)$/.test(k)) return "ai";
  if (DOCKER.has(ruleID) || /virtual (disk|machines)|containers/.test(k)) return "containers";
  if (/depend|package|environment|module cache|providers|classpath/.test(k)) return "packages";
  if (/build|compiled|generated|report|header cache|import cache|asset cache|debug symbols|archives/.test(k)) return "build";
  return "caches";
}
const itemCat = (it) => category(it.rule, it.kind);
const entryCat = (e) => category(e.rule, (state.rules[e.rule] || {}).kind);

// ---------- state ----------

const state = {
  view: "overview",
  scan: null,
  vault: [],
  settings: {},
  status: { running: false },
  home: "",
  excluded: new Set(),
  focused: null,
  query: "",
  meta: {},
  rules: {},
};

const TITLES = { overview: "Overview", keep: "Your data", vault: "Vault", explore: "Explore", why: "What's this folder?", history: "History", settings: "Settings" };
const MACHINE = () => ({ darwin: "Mac", windows: "PC" }[state.meta.os] || "computer");
const SHOW = () => ({ darwin: "Show in Finder", windows: "Show in Explorer" }[state.meta.os] || "Open folder");

async function refresh() {
  const s = await api("state");
  state.scan = s.scan;
  state.vault = s.vault;
  state.settings = s.settings;
  state.status = s.status;
  state.home = s.home;
  state.meta = s;
  state.rules = s.rules || {};
  $("#version").textContent = s.version;
  render();
  if (s.status.running) pollScan();
}

function items() { return (state.scan && state.scan.items) || []; }
function sum(list) { return list.reduce((n, it) => n + it.bytes, 0); }
const label = (it) => it.label || it.name;
const included = (it) => cleanable(it) && !state.excluded.has(it.id);
const keepDays = () => state.settings.keep_days || 14;

// shown lists what the categories hold: everything Leftover can clean or
// thinks is worth a look. Your data is listed on its own.
const shown = () => items().filter((it) => it.verdict !== "keep");
const inCat = (key) => shown().filter((it) => itemCat(it) === key);

function matches(it) {
  const q = state.query.toLowerCase();
  return [it.name, it.label, it.tool, it.kind, it.project, ...it.paths].some((s) => (s || "").toLowerCase().includes(q));
}

// rowsFor is the list a view shows, biggest first.
function rowsFor(view) {
  let list = [];
  if (state.query) list = items().filter(matches);
  else if (view === "keep") list = items().filter((it) => it.verdict === "keep");
  else if (view.startsWith("cat:")) list = inCat(view.slice(4));
  return list.slice().sort((a, b) => b.bytes - a.bytes);
}

const isList = () => !!state.query || state.view === "keep" || state.view.startsWith("cat:");

// ---------- rendering ----------

function render() {
  renderNav();
  renderDisk();
  const running = state.status.running;
  $("#scan-btn").classList.toggle("running", running);
  $("#scan-btn").title = running ? "Stop scanning" : "Scan again (⌘R)";
  $("#scanbar").hidden = !running;
  $("#view").classList.toggle("dim", running && !!state.scan && (state.view === "overview" || isList()));

  if (running && !state.scan) { setTitle("Scanning"); return renderScanning(); }
  if (!state.scan && (state.view === "overview" || isList())) { setTitle("Overview", ""); return renderWelcome(); }

  if (isList()) return renderList();
  const views = { overview: renderOverview, vault: renderVault, explore: renderExplore, why: renderWhy, history: renderHistory, settings: renderSettings };
  $("#inspector").hidden = true;
  (views[state.view] || renderOverview)();
}

function setTitle(title, sub) {
  $("#title").textContent = title;
  if (sub !== undefined) $("#subtitle").textContent = sub;
}

function renderNav() {
  const v = state.query ? "" : state.view;
  $("#nav-cats").innerHTML = CATS.map((c) => {
    const n = sum(inCat(c.key));
    return `<a class="nav ${v === "cat:" + c.key ? "active" : ""}" data-view="cat:${c.key}" href="#${c.key}">
      <span class="tile cat-${c.key}">${c.mono}</span><span class="nav-text">${c.label}</span><span class="meta">${n ? bytes(n) : "—"}</span></a>`;
  }).join("");
  document.querySelectorAll("#nav > .nav").forEach((a) => a.classList.toggle("active", a.dataset.view === v));
  const keep = items().filter((it) => it.verdict === "keep");
  $("#count-keep").textContent = keep.length ? bytes(sum(keep)) : "";
  const badge = $("#count-vault");
  badge.hidden = !state.vault.length;
  badge.textContent = state.vault.length;
}

function renderDisk() {
  const d = state.meta.disk;
  $("#disk").hidden = !d;
  if (!d) return;
  const left = sum(items().filter(cleanable)) + sum(state.vault);
  const used = Math.max(0, d.total - d.free);
  const pct = (n) => Math.min(100, (n / d.total) * 100).toFixed(2) + "%";
  $("#disk-name").textContent = "This " + MACHINE();
  $("#disk-total").textContent = bytes(d.total);
  $("#disk-used").style.width = pct(Math.max(0, used - left));
  $("#disk-left").style.width = pct(Math.min(used, left));
  $("#disk-note").textContent = `${bytes(d.free)} available` + (left ? ` · ${bytes(left)} leftovers` : "");
}

// The page's security policy forbids inline style attributes, so widths
// are carried in data attributes and applied through the DOM.
function applyStyles(root) {
  root.querySelectorAll("[data-width]").forEach((el) => { el.style.width = el.dataset.width; });
}

// ---------- welcome and scanning ----------

function renderWelcome() {
  $("#inspector").hidden = true;
  $("#view").innerHTML = `
    <div class="welcome">
      <img src="icon.svg" alt="">
      <h2>See what your tools left behind.</h2>
      <p>Leftover looks through caches, build folders and AI models, explains each one, and clears what's safe.</p>
      <button class="btn btn-primary btn-lg" data-act="scan">Scan this ${MACHINE()}</button>
      <div class="fine">Scanning only reads. Nothing is moved until you choose.</div>
    </div>`;
}

const PHASES = { starting: "Getting ready", locations: "Checking tool caches", projects: "Looking through your projects", measuring: "Measuring what was found", done: "Finishing up" };

function scanLine() {
  const p = state.status.progress || {};
  return `${PHASES[p.phase] || "Scanning"} · ${(p.folders || 0).toLocaleString()} folders checked · ${p.found || 0} found`;
}

function renderScanning() {
  $("#inspector").hidden = true;
  const p = state.status.progress || {};
  $("#subtitle").textContent = scanLine();
  $("#view").innerHTML = `
    <div class="scanning">
      <div class="spinner"></div>
      <div class="big">${bytes(p.bytes || 0)}</div>
      <div class="sub">${esc(scanLine())}</div>
      <div class="path">${esc(tilde(p.current || ""))}</div>
    </div>`;
}

// ---------- overview ----------

function ring(segs, total) {
  const R = 88, W = 18, C = 2 * Math.PI * R, gap = segs.length > 1 ? 4 : 0;
  let off = 0;
  const parts = segs.map(([key, n]) => {
    const len = total ? (n / total) * C : 0;
    const o = off;
    off += len;
    return `<circle class="seg cat-${key}" cx="104" cy="104" r="${R}" fill="none" stroke-width="${W}" stroke-dasharray="${Math.max(0, len - gap).toFixed(2)} ${C.toFixed(2)}" stroke-dashoffset="${(-o).toFixed(2)}"/>`;
  });
  return `<svg width="208" height="208" viewBox="0 0 208 208" aria-hidden="true">
    <circle class="seg-track" cx="104" cy="104" r="${R}" fill="none" stroke-width="${W}"/>${parts.join("")}</svg>`;
}

function renderOverview() {
  const live = items().filter(cleanable);
  const review = items().filter((it) => it.verdict === "review");
  const liveT = sum(live);
  const chosenT = sum(live.filter(included));
  const s = state.scan;
  setTitle("Overview", `${plural(shown().length, "item")} found · scanned ${since(s.started)}`);

  const segs = CATS.map((c) => [c.key, sum(live.filter((it) => itemCat(it) === c.key))]).filter(([, n]) => n > 0);
  const firstCat = CATS.find((c) => inCat(c.key).length);
  const free = state.meta.disk ? state.meta.disk.free : 0;
  const num = (n) => { const t = bytes(n).split(" "); return { n: t[0], u: t[1] }; };
  const hero = live.length ? num(liveT) : num(free);

  const stat = (cls, title, sub, n) => `
    <div class="stat"><span class="stat-label"><i class="swatch sw-${cls}"></i>${title}</span>
      <span class="stat-size">${bytes(n)}</span><span class="stat-sub">${sub}</span></div>`;

  const allT = sum(shown()) + sum(state.vault) || 1;
  const card = (key, title, tileCls, monoText, size, count) => `
    <button class="cat-card ${tileCls}" data-go="${key}">
      <div class="cat-card-top"><span class="tile tile-lg ${tileCls}">${monoText}</span><i class="ico ico-chevron"></i></div>
      <div class="cat-card-name"><b>${title}</b><span>${count}</span></div>
      <div><div class="cat-card-size">${bytes(size)}</div>
      <div class="share"><i data-width="${((size / allT) * 100).toFixed(2)}%"></i></div></div>
    </button>`;
  const cards = CATS.map((c) => {
    const list = inCat(c.key);
    return card("cat:" + c.key, c.label, "cat-" + c.key, c.mono, sum(list), list.length ? `${plural(list.length, "item")} · ${c.blurb.toLowerCase()}` : "All clean");
  });
  cards.push(card("vault", "Vault", "tile-vault", "V", sum(state.vault), state.vault.length ? `${plural(state.vault.length, "item")} · restorable for up to ${keepDays()} days` : "Empty"));

  $("#view").innerHTML = `
    <div class="page">
      <div class="hero">
        <div class="hero-top">
          <div class="ring">${ring(segs, liveT)}
            <div class="ring-label"><span class="ring-num">${hero.n}</span><span class="ring-unit">${hero.u} ${live.length ? "clearable" : "available"}</span></div>
          </div>
          <div class="hero-copy">
            <span class="eyebrow">${live.length ? "Ready to clean" : "All clean"}</span>
            <h2 class="hero-head">${live.length ? `${bytes(liveT)} of leftovers on this ${MACHINE()}` : "Nothing left to clean"}</h2>
            <p class="hero-sub">${live.length
              ? `Caches, dependencies and build output your tools bring back on their own. Anything you clean waits in the vault for ${keepDays()} days.`
              : free ? `You have ${bytes(free)} free. Scan again any time.` : "Scan again any time."}</p>
            <div class="hero-actions">
              ${live.length
                ? `<button class="btn btn-primary btn-lg" data-act="clean-all" ${chosenT ? "" : "disabled"}>${chosenT ? `Clean ${bytes(chosenT)}` : "Nothing selected"}</button>`
                : `<button class="btn btn-primary btn-lg" data-act="scan">Scan again</button>`}
              ${firstCat ? `<button class="btn btn-lg" data-go="cat:${firstCat.key}">Review items</button>` : ""}
            </div>
          </div>
        </div>
        <div class="stats">
          ${stat("rebuild", "Rebuilds automatically", "Your next build recreates it", sum(live.filter((it) => it.verdict === "rebuild")))}
          ${stat("redownload", "Downloads again", "Your tools fetch it when needed", sum(live.filter((it) => it.verdict === "redownload")))}
          ${stat("review", "Worth a look", "Shown, never cleaned for you", sum(review))}
        </div>
      </div>

      <h3 class="section-title">By category</h3>
      <div class="cards">${cards.join("")}</div>

      <div class="foot-note"><i class="ico ico-lock"></i>Scanned ${esc(new Date(s.started).toLocaleString([], { dateStyle: "medium", timeStyle: "short" }))} · ${s.folders.toLocaleString()} folders in ${(s.took_ns / 1e9).toFixed(1)}s. Nothing left this ${MACHINE()}.</div>
    </div>`;
  applyStyles($("#view"));
}

// ---------- lists ----------

function activity(it) {
  const parts = [it.project ? tilde(it.project) : it.label ? it.name : it.tool];
  if (it.paths.length > 1) parts.push(it.paths.length + " folders");
  if (it.idle_days >= 1) parts.push((it.project ? "untouched " : "last changed ") + ago(it.idle_days));
  return parts.join(" · ");
}

function renderList() {
  const rows = rowsFor(state.view);
  if (!rows.some((it) => it.id === state.focused)) state.focused = rows.length ? rows[0].id : null;
  const chosen = rows.filter(included);
  let head;
  if (state.query) {
    const t = `Results for “${state.query}”`;
    setTitle("Search", `${plural(rows.length, "item")} · ${bytes(sum(rows))}`);
    head = `<div class="list-title"><div><h2>${esc(t)}</h2><span>${plural(rows.length, "item")} · ${bytes(sum(rows))}</span></div></div>`;
  } else if (state.view === "keep") {
    setTitle("Your data", `${plural(rows.length, "item")} · ${bytes(sum(rows))}`);
    head = `<div class="list-title"><span class="tile tile-xl tile-keep"><i class="ico ico-lock"></i></span><div><h2>Your data</h2><span>Listed so you can see where the space goes. Leftover never moves it.</span></div></div>`;
  } else {
    const c = CAT[state.view.slice(4)];
    setTitle(c.label, `${plural(rows.length, "item")} · ${bytes(sum(rows))}`);
    head = `<div class="list-title"><span class="tile tile-xl cat-${c.key}">${c.mono}</span><div><h2>${c.label}</h2><span>${c.blurb}</span></div></div>`;
  }
  const canClean = rows.some(cleanable);
  $("#view").innerHTML = `
    <div class="page page-list">
      <div class="list-head">${head}
        ${canClean ? `<button class="btn btn-primary" data-act="clean-list" ${chosen.length ? "" : "disabled"}>${chosen.length ? `Clean ${bytes(sum(chosen))}` : "Nothing selected"}</button>` : ""}
      </div>
      ${rows.length ? `<div class="rows" role="list">${rows.map(row).join("")}</div>`
        : `<div class="empty-state"><b>${state.query ? "No matches" : "All clean here"}</b><span>${state.query ? `Nothing matches “${esc(state.query)}”.` : "Nothing left in this category."}</span></div>`}
      ${rows.some((it) => it.verdict === "review") ? `<p class="hint">Items marked Review hold data only you can judge. Leftover explains them but never cleans them for you.</p>` : ""}
    </div>`;
  renderInspector();
}

function row(it) {
  const v = VERDICTS[it.verdict];
  const cat = it.verdict === "keep" ? "tile-keep" : "cat-" + itemCat(it);
  const toggle = cleanable(it)
    ? `<button class="switch" role="switch" aria-checked="${included(it)}" aria-label="Include ${esc(label(it))} in clean" data-toggle="${esc(it.id)}"></button>`
    : `<span class="row-spacer"></span>`;
  return `<div class="row ${state.focused === it.id ? "focused" : ""}" role="listitem" data-focus="${esc(it.id)}">
    <span class="tile tile-md ${cat}">${esc(itemMono(it))}</span>
    <div class="row-main">
      <div class="row-text"><b>${esc(label(it))}</b><span>${esc(activity(it))}</span></div>
      ${it.notes && it.notes.length ? `<span class="flag" title="${esc(it.notes[0])}">!</span>` : ""}
      <span class="pill pill-${v.cls}">${v.short}</span>
      <span class="row-size">${bytes(it.bytes)}</span>
      ${toggle}
    </div>
  </div>`;
}

function renderInspector() {
  const el = $("#inspector");
  const it = items().find((x) => x.id === state.focused);
  if (!it || !isList()) { el.hidden = true; return; }
  const v = VERDICTS[it.verdict];
  const rule = state.rules[it.rule] || {};
  const catKey = itemCat(it);
  const tileCls = it.verdict === "keep" ? "tile-keep" : "cat-" + catKey;
  const comebackTitle = cleanable(it) ? "How it comes back" : it.verdict === "review" ? "Before you remove it" : "About removing it";
  let foot;
  if (cleanable(it)) foot = `<div class="insp-foot"><span>Include in clean</span><button class="switch" role="switch" aria-checked="${included(it)}" aria-label="Include in clean" data-toggle="${esc(it.id)}"></button></div>`;
  else if (it.verdict === "review") foot = `<div class="insp-foot quiet">Leftover only shows this. Check it, then remove it yourself if you don't need it.</div>`;
  else foot = `<div class="insp-foot quiet">This is your data. Leftover never moves it.</div>`;

  el.hidden = false;
  el.innerHTML = `
    <div class="insp-head"><span>Details</span><button class="insp-close" data-act="close-inspector" aria-label="Close details">×</button></div>
    <div class="insp-body">
      <div class="insp-hero">
        <span class="tile tile-xxl ${tileCls}">${esc(itemMono(it))}</span>
        <h2>${esc(label(it))}</h2>
        <span class="sub">${esc(it.tool)} · ${esc(it.kind)}</span>
        <span class="insp-size">${bytes(it.bytes)}</span>
        <span class="pill pill-${v.cls}">${v.long}</span>
      </div>
      ${(it.notes || []).map((n) => `<div class="note">${esc(n)}</div>`).join("")}
      ${rule.what ? `<div class="block"><b>What it is</b><p>${esc(rule.what)}</p></div>` : ""}
      ${rule.comeback || rule.command ? `<div class="callout">
        ${rule.comeback ? `<b>${comebackTitle}</b><p>${esc(rule.comeback)}</p>` : ""}
        ${rule.command ? `${rule.comeback ? "" : "<b>The tool's own command</b>"}<div class="cmdline"><span>$</span><code>${esc(rule.command)}</code><button class="btn-link" data-copy="${esc(rule.command)}">Copy</button></div>
          ${rule.comeback ? `<p class="small">The tool's own command for this, if you'd rather use it.</p>` : ""}` : ""}
      </div>` : ""}
      ${it.details && it.details.length ? `<div class="block"><b>Inside</b><div class="details">${it.details.map((d) => `
        <div class="detail"><span>${esc(d.name)}${d.shared ? ` <small>shares layers</small>` : ""}${d.reclaimable ? ` <small>${bytes(d.reclaimable)} reclaimable</small>` : ""}</span><span>${bytes(d.bytes)}</span></div>`).join("")}</div></div>` : ""}
      ${DOCKER.has(it.rule) && it.details && it.details.length ? `<div><button class="btn btn-small" data-act="docker-prune">Reclaim Docker space…</button></div>` : ""}
      <div class="facts">
        <div class="fact"><span>Category</span><span>${it.verdict === "keep" ? "Your data" : CAT[catKey].label}</span></div>
        <div class="fact"><span>Files</span><span>${(it.files || 0).toLocaleString()}</span></div>
        ${it.idle_days ? `<div class="fact"><span>Last changed</span><span>${ago(it.idle_days)}</span></div>` : ""}
        ${it.project ? `<div class="fact fact-col"><span>Project</span><span class="path"><code>${esc(tilde(it.project))}</code><button class="btn-link" data-reveal="${esc(it.project)}">Show</button></span></div>` : ""}
        <div class="fact fact-col"><span>Location${it.paths.length > 1 ? "s" : ""}</span>
          <span>${it.paths.map((p) => `<span class="path"><code>${esc(tilde(p))}</code><button class="btn-link" data-reveal="${esc(p)}" title="${SHOW()}">Show</button></span>`).join("")}</span></div>
        ${rule.source ? `<div class="fact"><span>Source</span><span><a href="${esc(rule.source)}" target="_blank" rel="noopener noreferrer">${esc(rule.source.replace(/^https:\/\//, "").replace(/\/.*$/, ""))}</a></span></div>` : ""}
      </div>
      <div class="insp-actions"><button class="btn btn-small" data-ignore="${esc(it.project || it.paths[0])}">Ignore this folder</button></div>
    </div>
    ${foot}`;
}

// ---------- cleaning ----------

// confirmClean sums up a clean you haven't looked at item by item: what
// goes, per category, and the parts that cost something to get back.
function confirmClean(chosen) {
  const byCat = CATS.map((c) => [c.label, sum(chosen.filter((it) => itemCat(it) === c.key))]).filter(([, n]) => n > 0);
  const recent = new Set(chosen.filter((it) => it.project && it.idle_days < 7).map((it) => it.project));
  const again = sum(chosen.filter((it) => it.verdict === "redownload"));
  const warns = [];
  if (recent.size) warns.push(`<b>${plural(recent.size, "project")} you changed this week</b>${recent.size === 1 ? "Its" : "Their"} next build will be slower.`);
  if (again >= 1e9) warns.push(`<b>${bytes(again)} will be downloaded again</b>Your tools fetch it when they next need it.`);
  return sheet({
    title: `Move ${bytes(sum(chosen))} to the vault?`,
    body: `<div class="sum-list">${byCat.map(([l, n]) => `<div><span>${l}</span><span>${bytes(n)}</span></div>`).join("")}</div>
      ${warns.map((w) => `<div class="sum-warn">${w}</div>`).join("")}
      <p class="sum-fine">The space comes back when the vault is emptied: automatically after ${keepDays()} days, or from the Vault page.</p>`,
    ok: "Move to Vault",
    wide: true,
  });
}

async function clean(list, ask) {
  const chosen = list.filter(included);
  const ids = chosen.map((it) => it.id);
  if (!ids.length) return;
  if (ask && !(await confirmClean(chosen))) return;
  try {
    const out = await api("clean", { ids });
    const moved = out.filter((o) => !o.error && o.entry);
    const failed = out.filter((o) => o.error);
    const freed = moved.reduce((n, o) => n + o.bytes, 0);
    await refresh();
    let msg = moved.length ? `Moved ${bytes(freed)} to the vault · frees up when emptied` : "Nothing was moved.";
    if (failed.length) msg += `. ${plural(failed.length, "item")} couldn't be moved: ${failed[0].error}`;
    const entries = moved.map((o) => o.entry.id);
    toast(msg, entries.length ? () => undoClean(entries) : null);
  } catch (e) {
    toast(e.message);
  }
}

async function undoClean(ids) {
  let back = 0;
  let err = "";
  for (const id of ids) {
    try { await api("restore", { id }); back++; } catch (e) { err = e.message; }
  }
  await refresh();
  toast(err ? `Put back ${back} of ${ids.length}: ${err}` : `Put back ${plural(back, "item")}. Scan again to see ${back === 1 ? "it" : "them"} in your lists.`);
}

function toggleInclude(id) {
  state.excluded.has(id) ? state.excluded.delete(id) : state.excluded.add(id);
  render();
}

// ---------- alert ----------

function sheet({ title, body, ok, danger, wide }) {
  return new Promise((resolve) => {
    $(".sheet").classList.toggle("sheet-wide", !!wide);
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

// ---------- docker ----------

// Docker's own prune, for what Docker re-creates on demand. Leftover never
// moves Docker's disk itself, and never prunes containers or volumes.
async function dockerPrune() {
  const okd = await sheet({
    title: "Reclaim space inside Docker?",
    body: `<p>Leftover asks Docker to remove its build cache and dangling images. Docker rebuilds the cache and pulls images again when they're next needed.</p>
      <p>This is Docker's own cleanup, so it can't be undone from the vault. Containers and volumes are never touched.</p>
      <label class="sheet-check"><input type="checkbox" id="prune-images"> Also remove every image no container uses</label>`,
    ok: "Reclaim",
    danger: true,
  });
  if (!okd) return;
  const images = !!$("#prune-images")?.checked;
  try {
    const steps = await api("docker/prune", { images });
    const failed = steps.filter((s) => s.error);
    const total = steps.reduce((n, s) => n + (s.reclaimed || 0), 0);
    toast(failed.length ? `Docker reclaimed ${bytes(total)}. ${failed[0].command} failed: ${failed[0].error}` : `Docker reclaimed ${bytes(total)}. Scan again to see the new sizes.`);
  } catch (e) {
    toast(e.message);
  }
}

// ---------- vault ----------

function daysLeft(e) {
  const ms = new Date(e.expires) - Date.now();
  return ms <= 0 ? 0 : Math.ceil(ms / 864e5);
}

function renderVault() {
  const v = state.vault.slice().sort((a, b) => new Date(b.created) - new Date(a.created));
  const total = sum(v);
  const auto = state.settings.auto_empty !== false;
  setTitle("Vault", v.length ? `${plural(v.length, "item")} · ${bytes(total)}` : "Empty");
  const expired = v.filter((e) => daysLeft(e) === 0);
  $("#view").innerHTML = `
    <div class="page page-narrow">
      <div class="vault-head">
        <div>
          <h2>${v.length ? bytes(total) + " held" : "Nothing held"}</h2>
          <p>Cleaned items wait here for ${keepDays()} days${auto ? ", then clear themselves" : " until you empty them"}. Put Back returns them exactly where they were. The space comes back when they're emptied.</p>
        </div>
        ${v.length ? `<div class="vault-actions">
          ${expired.length ? `<button class="btn" data-act="empty-expired">Empty expired</button>` : ""}
          <button class="btn btn-danger" data-act="empty-all">Empty Vault…</button>
        </div>` : ""}
      </div>
      ${v.length ? `<div class="rows">${v.map((e) => {
        const d = daysLeft(e);
        const when = d === 0 ? (auto ? "Expired · clears at the next scan" : "Expired") : `Clears in ${plural(d, "day")}`;
        return `<div class="row">
          <span class="tile tile-md cat-${entryCat(e)}">${esc(mono(/[,/]/.test(e.tool) ? e.name : e.tool))}</span>
          <div class="row-main">
            <div class="row-text"><b>${esc(e.name)}</b><span>${esc(tilde(e.project || e.paths[0]))} · ${when}${e.state === "moving" ? " · interrupted" : ""}</span></div>
            <div class="life"><i data-width="${Math.min(100, (d / keepDays()) * 100).toFixed(1)}%"></i></div>
            <span class="row-size">${bytes(e.bytes)}</span>
            <button class="btn btn-small" data-restore="${esc(e.id)}">Put Back</button>
          </div>
        </div>`;
      }).join("")}</div>`
      : `<div class="empty-state"><b>The vault is empty</b><span>Anything you clean waits here for ${keepDays()} days, so you can change your mind.</span></div>`}
    </div>`;
  applyStyles($("#view"));
}

async function restore(id) {
  try {
    const out = await api("restore", { id });
    const e = out.entry;
    const beside = out.restored.some((p, i) => p !== e.paths[i]);
    await refresh();
    toast(beside ? "Put back next to the copy your tools already rebuilt." : `Put back ${e.name}. Scan again to see it in your lists.`);
  } catch (err) { toast(err.message); }
}

async function emptyVault(expiredOnly) {
  const list = expiredOnly ? state.vault.filter((e) => daysLeft(e) === 0) : state.vault;
  const okd = await sheet({
    title: expiredOnly ? "Empty expired items?" : "Empty the vault?",
    body: `<p>${plural(list.length, "item")} (${bytes(sum(list))}) will be deleted for good. This can't be undone.</p>`,
    ok: "Delete",
    danger: true,
  });
  if (!okd) return;
  try {
    const out = await api("empty", expiredOnly ? { expired: true } : { all: true });
    await refresh();
    toast(`${bytes(out.freed)} freed`);
  } catch (err) { toast(err.message); }
}

// ---------- history ----------

async function renderHistory() {
  setTitle("History", "Everything Leftover has changed");
  $("#view").innerHTML = `<div class="page page-narrow"><p class="muted">Loading…</p></div>`;
  const events = await api("history");
  if (state.view !== "history" || state.query) return;
  const verbs = { move: "Moved to vault", restore: "Put back", empty: "Deleted for good", "docker-prune": "Pruned by Docker" };
  $("#view").innerHTML = `<div class="page page-narrow">${events.length ? `
    <p class="intro">Everything Leftover has changed on this ${MACHINE()}, newest first. Scans aren't listed because they only read.</p>
    <div class="table">
      ${events.map((ev) => `
        <div class="tr tr-history">
          <span class="muted small">${new Date(ev.at).toLocaleString([], { dateStyle: "medium", timeStyle: "short" })}</span>
          <span class="name">${esc(verbs[ev.action] || ev.action)}<small>${esc(ev.name)}</small></span>
          <span class="where">${esc(tilde((ev.paths || [])[0] || ""))}${ev.paths && ev.paths.length > 1 ? ` +${ev.paths.length - 1}` : ""}</span>
          <span class="r size">${bytes(ev.bytes)}</span>
        </div>`).join("")}
    </div>` : `<div class="empty-state"><b>Nothing yet</b><span>Leftover hasn't changed anything on this ${MACHINE()}.</span></div>`}</div>`;
}

// ---------- explore ----------

// A disk map of any folder, biggest first. The first look measures the
// folder; opening anything inside it reuses that measurement.
const ex = { path: "", view: null, loading: false, error: "" };

async function explore(path, again) {
  ex.loading = true;
  ex.error = "";
  if (state.view === "explore") render();
  try {
    ex.view = await api("explore", { path: path || "", refresh: !!again });
    ex.path = ex.view.path;
  } catch (e) {
    ex.error = e.message;
  }
  ex.loading = false;
  if (state.view === "explore") render();
}

function crumbs(path) {
  const sep = path.includes("\\") ? "\\" : "/";
  const home = state.home;
  let base = "", parts = path.split(sep).filter(Boolean);
  const out = [];
  if (home && (path === home || path.startsWith(home + sep))) {
    out.push(`<button class="crumb" data-explore="${esc(home)}">~</button>`);
    base = home;
    parts = path.slice(home.length).split(sep).filter(Boolean);
  } else if (sep === "/") {
    out.push(`<button class="crumb" data-explore="/">/</button>`);
  }
  for (const p of parts) {
    base = base ? (base.endsWith(sep) ? base + p : base + sep + p) : p + sep;
    out.push(`<button class="crumb" data-explore="${esc(base)}">${esc(p)}</button>`);
  }
  return out.join(`<span class="crumb-sep">${sep}</span>`);
}

function renderExplore() {
  setTitle("Explore", "What any folder holds, biggest first");
  if (!ex.view && !ex.loading && !ex.error) { explore(""); return; }
  const v = ex.view;
  const head = `
    <form id="explore-form" class="field-row">
      <input id="explore-path" class="field" type="text" placeholder="~/code" autocomplete="off" spellcheck="false" value="${esc(v ? tilde(v.path) : "")}" aria-label="Folder to explore">
      <button class="btn" type="submit">Open</button>
      ${v ? `<button class="btn" type="button" data-act="explore-refresh">Measure again</button>` : ""}
    </form>`;
  if (ex.loading) {
    $("#view").innerHTML = `<div class="page page-narrow">${head}<div class="scanning"><div class="spinner"></div><p class="muted">Measuring${v ? "" : " your home folder"}… the first look takes a few seconds.</p></div></div>`;
    return;
  }
  if (ex.error) {
    $("#view").innerHTML = `<div class="page page-narrow">${head}<div class="note">${esc(ex.error)}</div></div>`;
    return;
  }
  const max = Math.max(1, ...v.entries.map((e) => e.bytes));
  const note = (e) => {
    if (e.rule && e.rule.at === e.path) {
      const vd = VERDICTS[e.rule.verdict] || VERDICTS.keep;
      return `<span class="pill pill-${vd.cls}">${esc(e.rule.name)} · ${vd.short}</span>`;
    }
    if (e.rule) return `<span class="muted small">inside ${esc(e.rule.name)}</span>`;
    if (e.clearable) return `<span class="safe-text small">${bytes(e.clearable)} safe to clear</span>`;
    return "";
  };
  $("#view").innerHTML = `
    <div class="page page-narrow">
      ${head}
      <div class="crumbs">${crumbs(v.path)}</div>
      <div>
        <div class="big-num">${bytes(v.bytes)}</div>
        <span class="muted small">${v.files.toLocaleString()} files${v.clearable ? ` · <b class="safe-text">${bytes(v.clearable)} safe to clear</b> from your last scan` : ""}</span>
      </div>
      ${v.rule ? `<div class="note">Leftover knows this as ${esc(v.rule.name)} (${esc(v.rule.tool)}): ${(VERDICTS[v.rule.verdict] || VERDICTS.keep).long}.</div>` : ""}
      <div class="table">
        ${v.entries.map((e) => `
          <div class="tr tr-explore" ${e.dir ? `data-explore="${esc(e.path)}" tabindex="0" role="button"` : ""}>
            <span class="xbar"><i data-width="${Math.max(2, (e.bytes / max) * 100).toFixed(1)}%"></i></span>
            <span class="name">${esc(e.name)}${e.dir ? "/" : ""}</span>
            <span class="x-note">${note(e)}</span>
            <span class="r size">${bytes(e.bytes)}</span>
            <button class="btn-link" data-reveal="${esc(e.path)}" title="${SHOW()}">Show</button>
          </div>`).join("")}
        ${v.rest ? `<div class="tr tr-explore muted"><span></span><span class="name">Smaller items</span><span></span><span class="r size">${bytes(v.rest)}</span><span></span></div>` : ""}
      </div>
      ${v.unread ? `<p class="muted small">${plural(v.unread, "folder")} couldn't be read, so the real total may be higher.</p>` : ""}
      <p class="muted small">Measured ${new Date(v.measured).toLocaleTimeString([], { timeStyle: "short" })}. Leftover only reads here; to clear something, use the categories.</p>
    </div>`;
  applyStyles($("#view"));
}

// ---------- what's this folder? ----------

// The browser can't hand over a folder's real path, so the path is typed or
// pasted. The answer comes from the same rules as 'leftover why'.
const why = { path: "", result: null, error: "" };

function renderWhy() {
  setTitle("What's this folder?", "Nothing is read or changed");
  const r = why.result;
  let out = "";
  if (why.error) {
    out = `<div class="note">${esc(why.error)}</div>`;
  } else if (r && !r.known) {
    out = `
      <div class="why-card">
        <span class="pill pill-keep">Not recognised</span>
        <h2>${esc(tilde(r.path))}</h2>
        <p class="muted">Leftover doesn't know what this is, so it treats it as yours and never offers to clean it.</p>
        <p class="muted small">If it belongs to a dev tool, a rule for it would be a welcome contribution.</p>
      </div>`;
  } else if (r) {
    const rule = r.rule;
    const v = VERDICTS[rule.verdict] || VERDICTS.keep;
    const safe = rule.verdict === "rebuild" || rule.verdict === "redownload";
    out = `
      <div class="why-card">
        <span class="pill pill-${v.cls}">${v.long}</span>
        <div><h2>${esc(rule.name)}</h2><span class="muted">${esc(rule.tool)} · ${esc(rule.kind)}</span></div>
        ${r.matched ? `<div class="cmdline"><code>${esc(tilde(r.matched))}</code></div>` : ""}
        <div class="block"><b>What it is</b><p>${esc(rule.what || "")}</p></div>
        ${rule.comeback ? `<div class="block"><b>${safe ? "How it comes back" : "Before you remove it"}</b><p>${esc(rule.comeback)}</p></div>` : ""}
        ${safe ? "" : `<p class="muted small">Leftover shows this but never cleans it.</p>`}
        ${rule.command ? `<div class="block"><b>The tool's own command</b><div class="cmdline"><span>$</span><code>${esc(rule.command)}</code><button class="btn-link" data-copy="${esc(rule.command)}">Copy</button></div></div>` : ""}
        ${rule.source ? `<div class="block"><b>Source</b><p><a href="${esc(rule.source)}" target="_blank" rel="noopener noreferrer">${esc(rule.source.replace(/^https:\/\//, ""))}</a></p></div>` : ""}
      </div>`;
  }
  $("#view").innerHTML = `
    <div class="page page-narrow">
      <p class="intro">Paste the path of any folder to see what it is, which tool made it, and whether it's safe to clear.</p>
      <form id="why-form" class="field-row">
        <input id="why-path" class="field" type="text" placeholder="~/code/app/node_modules" autocomplete="off" spellcheck="false" value="${esc(why.path)}" aria-label="Folder path">
        <button class="btn btn-primary" type="submit">Explain</button>
      </form>
      ${out}
    </div>`;
}

async function explain(path) {
  why.path = path;
  why.error = "";
  why.result = null;
  if (path.trim()) {
    try { why.result = await api("why", { path }); } catch (err) { why.error = err.message; }
  }
  if (state.view === "why") { render(); $("#why-path")?.focus(); }
}

// ---------- settings ----------

function renderSettings() {
  setTitle("Settings", "");
  const st = state.settings;
  const keep = keepDays();
  const auto = st.auto_empty !== false;
  const ignored = st.ignore || [];
  $("#view").innerHTML = `
    <div class="page page-narrow">
      <div class="settings-group">
        <h3>Vault</h3>
        <div class="group">
          <div class="setting">
            <div><b>Keep cleared items for</b><small>How long the vault holds things before they can be emptied.</small></div>
            <select id="keep-days">${[3, 7, 14, 30, 60, 90].map((d) => `<option value="${d}" ${d === keep ? "selected" : ""}>${d} days</option>`).join("")}</select>
          </div>
          <div class="setting">
            <div><b>Empty expired items automatically</b><small>When a scan starts, items past their time are deleted to free the space.</small></div>
            <button class="switch" role="switch" id="auto-empty" aria-checked="${auto}" aria-label="Empty expired items automatically"></button>
          </div>
        </div>
      </div>

      <div class="settings-group">
        <h3>Ignored folders</h3>
        <div class="group">
          ${ignored.length ? ignored.map((p) => `
            <div class="setting"><code>${esc(tilde(p))}</code><button class="btn btn-small" data-unignore="${esc(p)}">Remove</button></div>`).join("")
            : `<div class="setting"><span class="muted">None. Use “Ignore this folder” on any item to keep Leftover out of it.</span></div>`}
        </div>
      </div>

      <div class="settings-group">
        <h3>Updates</h3>
        <div class="group">
          <div class="setting">
            <div><b>Leftover ${esc(state.meta.version || "")}</b><small id="update-status">${updateStatus()}</small></div>
            <div class="update-actions">${updateButtons()}</div>
          </div>
        </div>
      </div>

      <div class="settings-group">
        <h3>Privacy</h3>
        <div class="group">
          <div class="setting setting-text">
            <p>Leftover runs entirely on this ${MACHINE()}. It has no account and no analytics, and it never sends anything about you or your files anywhere. The only time it goes online is when you click Check for updates: it asks GitHub for the latest version, and downloads it if you choose Update now. Each update is checked against the release's signature before anything is replaced. This window is served only to this machine, on a private address with a key that changes every launch.</p>
            <p>Scanning only reads. Leftover writes to one folder of its own, for settings, the last scan, history and the vault: <code>${esc(tilde(state.meta.data_dir || ""))}</code>. Outside it, the only changes are the ones you choose, and every move, restore and deletion is listed under History.</p>
          </div>
        </div>
      </div>
    </div>`;
  $("#keep-days").onchange = (e) => saveSettings({ keep_days: Number(e.target.value) });
  $("#auto-empty").onclick = (e) => {
    const on = e.currentTarget.getAttribute("aria-checked") !== "true";
    e.currentTarget.setAttribute("aria-checked", on);
    saveSettings({ auto_empty: on });
  };
}

async function saveSettings(patch) {
  try {
    state.settings = await api("settings", patch);
    toast("Saved");
  } catch (e) { toast(e.message); renderSettings(); }
}

// ---------- updates ----------

// Nothing is checked until the button is pressed: Leftover only goes
// online when asked to.
const upd = { busy: "", info: null, applied: null, error: "" };

function updateStatus() {
  if (upd.error) return esc(upd.error);
  if (upd.busy === "check") return "Asking GitHub…";
  if (upd.busy === "apply") return "Downloading and checking the signature…";
  if (upd.applied) return esc(upd.applied.message) + (upd.applied.method === "installer" ? "" : " Restart Leftover to use it.");
  const i = upd.info;
  if (!i) return "Checks only when you ask. Nothing about you is sent.";
  if (i.method === "dev") return `This is a development build. The latest release is ${esc(i.latest)}.`;
  if (!i.newer) return "You're up to date.";
  let s = `Version ${esc(i.latest)} is available. <a href="${esc(i.page)}" target="_blank" rel="noopener noreferrer">What's new</a>`;
  if (i.command) s += `<br>Update it with <code>${esc(i.command)}</code>`;
  else if (!i.can_apply) s += `<br>Download it from the release page.`;
  return s;
}

function updateButtons() {
  if (upd.busy) return `<button class="btn btn-small" disabled>Working…</button>`;
  if (upd.applied) return `<button class="btn btn-small btn-primary" data-act="update-restart">${upd.applied.method === "installer" ? "Install and restart" : "Restart now"}</button>`;
  const i = upd.info;
  if (i && i.newer && i.can_apply) return `<button class="btn btn-small btn-primary" data-act="update-apply">Update now</button>`;
  if (i && i.newer && !i.command && i.download) return `<a class="btn btn-small" href="${esc(i.download)}" target="_blank" rel="noopener noreferrer">Download</a>`;
  return `<button class="btn btn-small" data-act="update-check">Check for updates</button>`;
}

function rerenderUpdate() {
  if (state.view !== "settings") return;
  const st = $("#update-status"), act = document.querySelector(".update-actions");
  if (st) st.innerHTML = updateStatus();
  if (act) act.innerHTML = updateButtons();
}

async function checkUpdate() {
  upd.busy = "check"; upd.error = ""; rerenderUpdate();
  try { upd.info = await api("update/check", {}); } catch (e) { upd.error = e.message; }
  upd.busy = ""; rerenderUpdate();
}

async function applyUpdate() {
  upd.busy = "apply"; upd.error = ""; rerenderUpdate();
  try { upd.applied = await api("update/apply", {}); } catch (e) { upd.error = e.message; }
  upd.busy = ""; rerenderUpdate();
}

async function restartUpdate() {
  try {
    await api("update/restart", {});
    $("#view").innerHTML = `<div class="scanning"><div class="spinner"></div><div class="big">Restarting…</div><div class="sub">The new version opens in its own window. You can close this one.</div></div>`;
    setTimeout(() => window.close(), 1500);
  } catch (e) { upd.error = e.message; rerenderUpdate(); }
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
      if (!state.scan) renderScanning();
      else if (state.view === "overview" || isList()) $("#subtitle").textContent = scanLine();
      await new Promise((r) => setTimeout(r, 250));
    }
  } finally {
    polling = false;
  }
  if (state.status.error && !/cancel/i.test(state.status.error)) toast(state.status.error);
  state.excluded.clear();
  await refresh();
}

async function scan() {
  if (state.status.running) { await api("scan/cancel", {}); return; }
  try {
    await api("scan", {});
    state.status.running = true;
    if (!(state.view === "overview" || isList())) go("overview");
    render();
    pollScan();
  } catch (e) { toast(e.message); }
}

// ---------- toast ----------

let toastTimer;
function toast(msg, undo) {
  const t = $("#toast");
  $("#toast-text").textContent = msg;
  const btn = $("#toast-undo");
  btn.hidden = !undo;
  t.classList.toggle("has-undo", !!undo);
  btn.onclick = undo ? () => { t.hidden = true; clearTimeout(toastTimer); undo(); } : null;
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), undo ? 9000 : Math.max(3200, msg.length * 55));
}

// ---------- events ----------

function go(view) {
  state.view = view;
  state.focused = null;
  state.query = "";
  $("#search").value = "";
  $("#view").scrollTop = 0;
  render();
}

async function copy(text) {
  try { await navigator.clipboard.writeText(text); toast("Command copied"); }
  catch (_) { toast("Couldn't copy. Select the command and copy it instead."); }
}

document.addEventListener("click", async (e) => {
  const nav = e.target.closest(".nav");
  if (nav) { e.preventDefault(); go(nav.dataset.view); return; }
  const tg = e.target.closest("[data-toggle]");
  if (tg) { e.stopPropagation(); return toggleInclude(tg.dataset.toggle); }
  const gt = e.target.closest("[data-go]");
  if (gt) return go(gt.dataset.go);
  const cp = e.target.closest("[data-copy]");
  if (cp) return copy(cp.dataset.copy);
  const act = e.target.closest("[data-act]");
  if (act) {
    const a = act.dataset.act;
    if (a === "scan") return scan();
    if (a === "clean-all") return clean(items(), true);
    if (a === "clean-list") return clean(rowsFor(state.view));
    if (a === "close-inspector") { $("#inspector").hidden = true; return; }
    if (a === "empty-expired") return emptyVault(true);
    if (a === "empty-all") return emptyVault(false);
    if (a === "docker-prune") return dockerPrune();
    if (a === "explore-refresh") return explore(ex.path, true);
    if (a === "update-check") return checkUpdate();
    if (a === "update-apply") return applyUpdate();
    if (a === "update-restart") return restartUpdate();
  }
  const un = e.target.closest("[data-unignore]");
  if (un) {
    try { await api("ignore", { path: un.dataset.unignore, remove: true }); } catch (err) { return toast(err.message); }
    await refresh();
    toast("Leftover will scan that folder again.");
    return;
  }
  const rs = e.target.closest("[data-restore]");
  if (rs) return restore(rs.dataset.restore);
  const rev = e.target.closest("[data-reveal]");
  if (rev) { api("reveal", { path: rev.dataset.reveal }).catch((err) => toast(err.message)); return; }
  const xp = e.target.closest("[data-explore]");
  if (xp) return explore(xp.dataset.explore);
  const ign = e.target.closest("[data-ignore]");
  if (ign) {
    try { await api("ignore", { path: ign.dataset.ignore }); } catch (err) { return toast(err.message); }
    state.focused = null;
    toast("Leftover will leave that folder alone from the next scan.");
    return refresh();
  }
  const f = e.target.closest("[data-focus]");
  if (f) {
    state.focused = f.dataset.focus;
    renderList();
  }
});

document.addEventListener("submit", (e) => {
  e.preventDefault();
  if (e.target.id === "explore-form") return explore($("#explore-path").value);
  if (e.target.id === "why-form") return explain($("#why-path").value);
});

$("#scan-btn").addEventListener("click", scan);
$("#search").addEventListener("input", (e) => {
  state.query = e.target.value.trim();
  state.focused = null;
  render();
});

// Keyboard: ⌘R / Ctrl+R scans, ⌘F / Ctrl+F or / searches, arrows move
// through a list, space includes or leaves out the focused item, Escape
// leaves the search.
document.addEventListener("keydown", (e) => {
  if (!$("#sheet").hidden) return;
  const mod = e.metaKey || e.ctrlKey;
  const typing = e.target.matches("input[type=search], input[type=text], select");
  if (mod && e.key.toLowerCase() === "r") { e.preventDefault(); scan(); return; }
  if ((mod && e.key.toLowerCase() === "f") || (e.key === "/" && !typing)) { e.preventDefault(); $("#search").focus(); return; }
  if (e.key === "Enter" && e.target.matches(".tr-explore[data-explore]")) { explore(e.target.dataset.explore); return; }
  if (e.key === "Escape") {
    if (e.target.id === "search" && state.query) { e.target.value = ""; state.query = ""; render(); }
    if (typing) e.target.blur();
    return;
  }
  if (typing || !isList()) return;
  const list = rowsFor(state.view);
  if (!list.length) return;
  const i = list.findIndex((it) => it.id === state.focused);
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    e.preventDefault();
    const next = e.key === "ArrowDown" ? Math.min(i + 1, list.length - 1) : Math.max(i - 1, 0);
    state.focused = list[next].id;
    renderList();
    document.querySelector(`[data-focus="${CSS.escape(state.focused)}"]`)?.scrollIntoView({ block: "nearest" });
  } else if (e.key === " " && i >= 0 && cleanable(list[i]) && !e.target.matches("button")) {
    e.preventDefault();
    toggleInclude(list[i].id);
  }
});

setInterval(() => api("ping", {}).catch(() => {}), 15000);
refresh().catch((e) => toast(e.message));
