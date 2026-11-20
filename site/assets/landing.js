// Leftover landing page: scroll-triggered scenes, the throwable pile in the
// hero and the download button. Plain JavaScript; the one library (Matter.js)
// is served from this site, so nothing is loaded from anywhere else.
"use strict";

(() => {
  const root = document.getElementById("top");
  if (!root) return;
  document.documentElement.classList.remove("no-js");

  const clamp = (v, a = 0, b = 1) => Math.min(b, Math.max(a, v));
  const seg = (p, a, b) => clamp((p - a) / (b - a));
  const ease = (x) => 1 - Math.pow(1 - x, 3);
  const eio = (x) => (x < 0.5 ? 4 * x * x * x : 1 - Math.pow(-2 * x + 2, 3) / 2);

  const DUR = { hero: 4.6, scan: 3.4, explain: 2.4, quar: 4.2, privacy: 2.2 };
  const DELAY = { hero: 0.8, scan: 0.1, explain: 0.1, quar: 0.1, privacy: 0.1 };
  // Sizes and categories of the scan window's rows, in page order.
  const SCAN = [
    { size: 31.2, cat: "Containers", path: "~/Library/Containers/com.docker.docker" },
    { size: 29.1, cat: "AI models", path: "~/.ollama/models" },
    { size: 21.3, cat: "Packages", path: "~/code/**/node_modules" },
    { size: 18.2, cat: "AI models", path: "~/.cache/huggingface/hub" },
    { size: 16.0, cat: "Build output", path: "~/Library/Developer/Xcode/DerivedData" },
    { size: 12.4, cat: "Build output", path: "~/code/**/target" },
    { size: 7.2, cat: "AI models", path: "~/.lmstudio/models" },
    { size: 4.8, cat: "Packages", path: "~/.npm/_cacache" },
    { size: 4.1, cat: "Packages", path: "~/Library/Caches/pip" },
    { size: 3.7, cat: "Packages", path: "~/code/**/.venv" },
  ];
  // The order rows are discovered in during the scan animation.
  const DISC = [2, 7, 0, 9, 4, 1, 5, 8, 3, 6];
  const SCAN_ITEMS = 2184;
  const LEFTOVER_GB = 148;
  const LABELS = ["node_modules", ".venv", "target/", "DerivedData", ".cache", "dist/", "build/", "__pycache__", ".next", "blobs/", ".gradle", "Pods/", ".turbo", ".pnpm-store", "models/"];

  const S = { t0: performance.now(), sp: {}, applied: {}, seenAt: {}, vel: 0, fc: 0 };
  const rm = matchMedia("(prefers-reduced-motion: reduce)");
  const dark = matchMedia("(prefers-color-scheme: dark)");

  function applyTheme() {
    document.documentElement.dataset.theme = dark.matches ? "dark" : "light";
    S.bgCol = null;
  }
  dark.addEventListener("change", applyTheme);
  applyTheme();

  function q(el, k) {
    const c = el.__q || (el.__q = {});
    return c[k] || (c[k] = Array.from(el.querySelectorAll(`[data-x="${k}"]`)));
  }
  function rise(arr, e, d = 24) { arr.forEach((x) => { x.style.opacity = e; x.style.transform = `translateY(${(1 - e) * d}px)`; }); }
  function setT(x, txt) { if (x && x.__t !== txt) { x.textContent = txt; x.__t = txt; } }

  function setCompact(compact) {
    const s = root.style;
    ["--side", "--path", "--navl"].forEach((k) => (compact ? s.setProperty(k, "none") : s.removeProperty(k)));
  }

  // ---------- frame loop ----------

  function frame(now) {
    const dt = Math.min(0.05, (now - (S.ln || now)) / 1000); S.ln = now;
    S.fc++;
    const t = (now - S.t0) / 1000, H = innerHeight;
    const mode = rm.matches ? "static" : "reveal";
    const compact = innerWidth < 760;
    if (mode !== S.mode || compact !== S.compact) { S.mode = mode; S.compact = compact; setCompact(compact); S.applied = {}; S.seenAt = {}; S.sp = {}; }
    const y = scrollY; S.vel = S.vel * 0.9 + Math.abs(y - (S.ly ?? y)) / Math.max(dt, 0.001) * 0.1; S.ly = y;
    const refresh = S.fc % 60 === 0;

    root.querySelectorAll("[data-scene]").forEach((el) => {
      if (refresh) el.__q = null;
      const name = el.dataset.scene, r = el.getBoundingClientRect();
      let target;
      if (mode === "static") target = 1;
      else {
        if (S.seenAt[name] == null && r.top < H * 0.8) S.seenAt[name] = t;
        target = S.seenAt[name] == null ? 0 : clamp((t - S.seenAt[name] - (DELAY[name] || 0)) / (DUR[name] || 2.5));
      }
      S.sp[name] = target;
      const vis = r.bottom > -50 && r.top < H + 50;
      const key = target.toFixed(4);
      if (!vis && S.applied[name] === key) return;
      S.applied[name] = key;
      const fn = SCENES[name]; if (fn) fn(el, target, t, dt);
    });

    const mqs = root.querySelectorAll('[data-x="mq"]');
    if (mqs.length && mode !== "static") {
      const r = mqs[0].getBoundingClientRect();
      if (r.bottom > -100 && r.top < H + 100) {
        S.mqx = (S.mqx || 0) + dt * (30 + Math.min(S.vel, 3000) * 0.05);
        mqs.forEach((m) => {
          const half = m.scrollWidth / 2; if (!half) return;
          const o = S.mqx % half; const x = (+m.dataset.dir || 1) > 0 ? -o : -(half - o);
          m.style.transform = `translate3d(${x.toFixed(1)}px,0,0)`;
        });
      }
    }

    root.querySelectorAll("[data-reveal]").forEach((e) => {
      if (!e.__rv) {
        e.__rv = 1;
        const r = e.getBoundingClientRect();
        if (r.top > H * 0.92 && mode !== "static") {
          e.style.opacity = 0; e.style.transform = "translateY(28px)";
          e.style.transition = "opacity .8s cubic-bezier(.2,.7,.2,1), transform .8s cubic-bezier(.2,.7,.2,1)";
          e.style.transitionDelay = (+e.dataset.reveal || 0) + "ms";
          e.__hidden = 1;
        }
      } else if (e.__hidden && e.getBoundingClientRect().top < H * 0.9) {
        e.style.opacity = 1; e.style.transform = "none"; e.__hidden = 0;
      }
    });

    cli(t, mode, H);
    nav(H);
    pileFrame(now, dt, H);
    root.querySelectorAll('[data-x="lbg"]').forEach((cv) => { const r = cv.getBoundingClientRect(); if (r.bottom > 0 && r.top < H) bgStep(cv, dt); });
  }

  // ---------- background field ----------

  function bgStep(cv, dt) {
    const W = cv.clientWidth, H = cv.clientHeight; if (!W || !H) return;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    if (cv.width !== Math.round(W * dpr) || cv.height !== Math.round(H * dpr)) { cv.width = Math.round(W * dpr); cv.height = Math.round(H * dpr); cv.__B = null; }
    const still = S.mode === "static";
    let B = cv.__B;
    if (!B || B.still !== still) {
      const d = cv.dataset;
      B = cv.__B = { nodes: [], crumbs: [], st: 0, nextSpawn: 0, nextDel: 2.5, nextSweep: 6, sweep: null, W, H, still,
        cfg: { cy: +d.cy || 0.4, rx: W < 760 ? 0.62 : (+d.rx || 0.34), ry: +d.ry || 0.36, bar: d.bar === "1" } };
      bgSeed(B);
      for (let i = 0; i < (still ? 420 : 90); i++) bgSim(B, 1 / 30);
    }
    if (!still) bgSim(B, dt);
    if (!S.bgCol || S.fc % 30 === 0) {
      const cs = getComputedStyle(document.documentElement);
      S.bgCol = { ink: cs.getPropertyValue("--ink").trim() || "#1D1D1F", bg: cs.getPropertyValue("--bg").trim() || "#FBFBFD", acc: cs.getPropertyValue("--accent").trim() || "#2f9e66" };
    }
    bgDraw(cv, B, dpr);
  }

  function bgField(B, x, y) {
    const { W, H, cfg } = B;
    const dx = (x - W * 0.5) / (W * cfg.rx), dy = (y - H * cfg.cy) / (H * cfg.ry);
    let a = clamp((Math.sqrt(dx * dx + dy * dy) - 0.7) / 0.55);
    if (cfg.bar) a *= 1 - 0.92 * clamp((W * 0.47 - Math.abs(x - W * 0.5)) / (W * 0.06)) * clamp((y - H * 0.6) / (H * 0.08));
    return a * (1 - clamp((y - H * 0.9) / (H * 0.1))) * clamp((y - 40) / (H * 0.08));
  }

  function bgSeed(B) {
    let made = 0;
    for (let i = 0; i < 300 && made < 6; i++) {
      const x = 20 + Math.random() * (B.W - 40), y = 60 + Math.random() * (B.H - 120);
      const a = bgField(B, x, y); if (a < 0.5) continue;
      if (B.nodes.some((n) => Math.hypot(n.x - x, n.y - y) < 160)) continue;
      B.nodes.push({ x, y, parent: null, kids: 0, born: -1, dead: false, a, label: null, type: "root" }); made++;
    }
  }

  function bgSpawn(B, T, alive, maxN) {
    if (alive.length < 4) bgSeed(B);
    if (alive.length >= maxN) return;
    const cands = alive.filter((n) => n.kids < (n.parent ? 3 : 5) && T - n.born > 0.6);
    if (!cands.length) return;
    const p = cands[Math.floor(Math.random() * cands.length)];
    for (let k = 0; k < 10; k++) {
      const ang = Math.random() * 6.283, dist = 60 + Math.random() * 90;
      const x = p.x + Math.cos(ang) * dist, y = p.y + Math.sin(ang) * dist;
      if (x < 12 || y < 12 || x > B.W - 12 || y > B.H - 12) continue;
      const a = bgField(B, x, y); if (a < 0.08) continue;
      if (B.nodes.some((n) => !n.dead && Math.hypot(n.x - x, n.y - y) < 46)) continue;
      let alt = null, best = 150;
      if (Math.random() < 0.55) for (const n of alive) { if (n === p) continue; const d = Math.hypot(n.x - x, n.y - y); if (d < best) { best = d; alt = n; } }
      const label = Math.random() < 0.14 ? LABELS[Math.floor(Math.random() * LABELS.length)] : null;
      B.nodes.push({ x, y, parent: p, alt, kids: 0, born: T, dead: false, a: Math.min(a, p.a), label, type: "node" });
      p.kids++; if (alt) alt.kids++; return;
    }
  }

  function bgSim(B, dt) {
    B.st += dt; const T = B.st;
    const alive = B.nodes.filter((n) => !n.dead);
    const maxN = Math.max(24, Math.min(80, B.W * B.H / 14000));
    B.nextSpawn -= dt;
    let guard = 0;
    while (B.nextSpawn <= 0 && guard++ < 4) { B.nextSpawn += alive.length < maxN * 0.7 ? 0.2 : 0.5; bgSpawn(B, T, alive, maxN); }
    B.nextDel -= dt;
    if (B.nextDel <= 0) {
      B.nextDel = alive.length > maxN * 0.8 ? 0.3 : 0.7 + Math.random() * 0.7;
      const leaves = alive.filter((n) => n.kids === 0 && n.parent && T - n.born > 3);
      if (leaves.length) { const n = leaves[Math.floor(Math.random() * leaves.length)]; n.dead = true; n.deadAt = T; n.parent.kids--; if (n.alt) n.alt.kids--; }
    }
    for (const n of B.nodes) if (n.dead && !n.crumbed && T - n.deadAt > 0.5) {
      n.crumbed = true;
      const k = 3 + Math.floor(Math.random() * 4);
      for (let i = 0; i < k; i++) { const ang = Math.random() * 6.283, sp = 10 + Math.random() * 26; B.crumbs.push({ x: n.x, y: n.y, vx: Math.cos(ang) * sp, vy: Math.sin(ang) * sp, r: Math.random() < 0.3 ? 2 : 1.2, rot: Math.random() < 0.35 ? Math.random() * 6.283 : null, born: T, clean: null }); }
      if (n.label) B.crumbs.push({ x: n.x + 8, y: n.y + 3.5, vx: 0, vy: 5, label: n.label, born: T, clean: null });
    }
    B.nodes = B.nodes.filter((n) => !(n.dead && T - n.deadAt > 1));
    B.pulses = (B.pulses || []).filter((p) => T - p.t0 < 1.4 && !p.n.dead);
    B.nextPulse = (B.nextPulse ?? 0) - dt;
    if (B.nextPulse <= 0) { B.nextPulse = 0.35 + Math.random() * 0.5; const ls = B.nodes.filter((n) => n.parent && !n.dead && T - n.born > 1); if (ls.length) B.pulses.push({ n: ls[Math.floor(Math.random() * ls.length)], t0: T }); }
    const f = Math.exp(-dt * 3);
    for (const c of B.crumbs) { c.vx *= f; c.vy *= f; c.x += c.vx * dt; c.y += c.vy * dt; }
    if (B.crumbs.length > 500) B.crumbs.splice(0, B.crumbs.length - 500);
    B.nextSweep -= dt;
    if (!B.sweep && B.nextSweep <= 0) B.sweep = { t0: T, x: -80 };
    if (B.sweep) {
      const k = (T - B.sweep.t0) / 2.8;
      B.sweep.x = -80 + (B.W + 160) * eio(Math.min(1, k));
      for (const c of B.crumbs) if (c.clean == null && c.x < B.sweep.x && T - c.born > 0.6) c.clean = T;
      if (k >= 1) { B.sweep = null; B.nextSweep = 10 + Math.random() * 4; }
    }
    B.crumbs = B.crumbs.filter((c) => c.clean == null || T - c.clean < 1);
  }

  function bgDraw(cv, B, dpr) {
    const ctx = cv.getContext("2d"), T = B.st, { ink, acc, bg } = S.bgCol;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0); ctx.clearRect(0, 0, B.W, B.H);
    ctx.lineWidth = 1; ctx.strokeStyle = ink; ctx.fillStyle = ink;
    const line = (ax, ay, bx, by, qq, al) => { if (qq <= 0 || al <= 0) return; ctx.globalAlpha = al; ctx.beginPath(); ctx.moveTo(ax, ay); ctx.lineTo(ax + (bx - ax) * qq, ay + (by - ay) * qq); ctx.stroke(); };
    for (const n of B.nodes) {
      if (!n.parent) continue;
      const qq = n.dead ? 1 - clamp((T - n.deadAt) / 0.5) : eio(clamp((T - n.born) / 0.7));
      line(n.parent.x, n.parent.y, n.x, n.y, qq, 0.2 * n.a);
      if (n.alt && !n.alt.dead) { const q2 = n.dead ? qq : eio(clamp((T - n.born - 0.5) / 0.7)); line(n.x, n.y, n.alt.x, n.alt.y, q2, 0.14 * Math.min(n.a, n.alt.a)); }
    }
    for (const p of B.pulses || []) {
      const n = p.n, k = eio(clamp((T - p.t0) / 1.4)); const x = n.parent.x + (n.x - n.parent.x) * k, y = n.parent.y + (n.y - n.parent.y) * k;
      ctx.globalAlpha = 0.45 * n.a * Math.sin(Math.PI * k); ctx.fillStyle = acc; ctx.beginPath(); ctx.arc(x, y, 2.6, 0, 6.283); ctx.fill();
    }
    ctx.fillStyle = ink;
    ctx.font = "10px ui-monospace, SFMono-Regular, Menlo, monospace";
    for (const n of B.nodes) {
      const ap = n.parent ? clamp((T - n.born - 0.55) / 0.3) : 1;
      const fade = n.dead ? 1 - clamp((T - n.deadAt) / 0.35) : 1;
      const a = n.a * ap * fade; if (a <= 0) continue;
      const r = n.type === "root" ? 4.5 : 3;
      ctx.globalAlpha = a;
      ctx.fillStyle = bg; ctx.beginPath(); ctx.arc(n.x, n.y, r, 0, 6.283); ctx.fill();
      ctx.globalAlpha = 0.45 * a; ctx.beginPath(); ctx.arc(n.x, n.y, r, 0, 6.283); ctx.stroke();
      ctx.fillStyle = ink;
      if (n.label) { ctx.globalAlpha = 0.38 * a; ctx.fillText(n.label, n.x + 9, n.y + 3.5); }
    }
    for (const k of B.crumbs) {
      const fa = bgField(B, k.x, k.y); if (fa <= 0) continue;
      let a = 0.38 * fa * clamp((T - k.born) / 0.3);
      if (k.clean != null) { const qq = (T - k.clean) / 1; ctx.fillStyle = ctx.strokeStyle = acc; a = fa * (1 - qq) * 0.9; }
      else { ctx.fillStyle = ctx.strokeStyle = ink; }
      ctx.globalAlpha = a;
      if (k.label) {
        ctx.globalAlpha = a * 0.7; ctx.fillText(k.label, k.x, k.y);
        const w = ctx.measureText(k.label).width; ctx.fillRect(k.x, k.y - 3.5, w, 1);
      } else if (k.rot != null) {
        ctx.beginPath(); ctx.moveTo(k.x, k.y); ctx.lineTo(k.x + Math.cos(k.rot) * 3.5, k.y + Math.sin(k.rot) * 3.5); ctx.stroke();
      } else ctx.fillRect(k.x, k.y, k.r, k.r);
    }
    if (B.sweep) {
      const x = B.sweep.x;
      for (let y = 0; y < B.H; y += 16) {
        const fa = bgField(B, x, y); if (fa <= 0) continue;
        ctx.globalAlpha = 0.55 * fa; ctx.fillStyle = acc; ctx.fillRect(x, y, 1, 16);
        ctx.globalAlpha = 0.06 * fa; ctx.fillRect(x - 60, y, 60, 16);
      }
    }
    ctx.globalAlpha = 1;
  }

  // ---------- nav ----------

  function nav(H) {
    const el = root.querySelector('[data-x="nav"]'); if (!el) return;
    const sc = scrollY > 10;
    if (el.__sc !== sc) { el.__sc = sc; el.style.borderBottomColor = sc ? "var(--hair)" : "transparent"; }
    const links = Array.from(root.querySelectorAll("[data-nav]")), ind = root.querySelector('[data-x="navind"]');
    let act = null;
    links.forEach((l) => { const sec = document.getElementById(l.dataset.nav); if (sec && sec.getBoundingClientRect().top < H * 0.45) act = l; });
    if (act === S.navAct) return; S.navAct = act;
    links.forEach((l) => (l.style.color = l === act ? "var(--ink)" : "var(--ink2)"));
    if (ind) { if (act) { ind.style.transform = `translateX(${act.offsetLeft}px)`; ind.style.width = act.offsetWidth + "px"; ind.style.opacity = 1; } else ind.style.opacity = 0; }
  }

  // ---------- scenes ----------

  const SCENES = {
    hero(el, p, t) {
      q(el, "hin").forEach((e) => { const d = +e.dataset.d || 0; const k = S.mode === "static" ? 1 : ease(clamp((t - 0.1 - d * 0.12) / 0.9)); e.style.opacity = k; e.style.transform = `translateY(${(1 - k) * 26}px)`; });
    },

    scan(el, p, t, dt) {
      const X = (k) => q(el, k);
      const hd = X("shead")[0]; if (hd) rise([hd], ease(seg(p, 0, 0.12)));
      const ww = X("swin")[0];
      if (ww) { const we = ease(seg(p, 0, 0.16)); ww.style.opacity = we; ww.style.transform = `translateY(${((1 - we) * 80).toFixed(1)}px)`; }
      const rowH = 44; let total = 0; const cat = { All: 0 }; const cur = []; let lastVis = null, lastA = -1;
      X("srow").forEach((r) => {
        const i = +r.dataset.i, d = SCAN[i]; if (!d) return;
        const k = DISC.indexOf(i), a = 0.16 + k * 0.042;
        const g = ease(seg(p, a, a + 0.15)), vis = p >= a, sz = d.size * g;
        cur.push({ r, i, sz, vis, a });
        if (vis) { total += sz; cat.All += sz; cat[d.cat] = (cat[d.cat] || 0) + sz; if (a > lastA) { lastA = a; lastVis = d; } }
      });
      const vl = cur.filter((c) => c.vis).sort((x, y) => y.sz - x.sz || x.i - y.i);
      vl.forEach((c, rank) => (c.rank = rank));
      const sm = 1 - Math.exp(-dt * 10);
      cur.forEach((c) => {
        const target = (c.vis ? c.rank : vl.length) * rowH;
        if (c.r.__y == null || S.mode === "static") c.r.__y = target; else c.r.__y += (target - c.r.__y) * sm;
        const o = c.vis ? ease(seg(p, c.a, c.a + 0.03)) : 0;
        c.r.style.opacity = o; c.r.style.transform = `translateY(${(c.r.__y + (1 - o) * 10).toFixed(1)}px)`;
        setT(c.r.querySelector('[data-x="ssize"]'), c.sz.toFixed(1) + " GB");
      });
      setT(X("stotal")[0], total.toFixed(1) + " GB");
      X("scat").forEach((e) => setT(e, (cat[e.dataset.c] || 0).toFixed(1)));
      const prog = seg(p, 0.16, 0.58), done = p >= 0.6;
      const pg = X("sprog")[0]; if (pg) pg.style.width = prog * 100 + "%";
      const pw = X("sprogw")[0]; if (pw) pw.style.opacity = done ? 0 : 1;
      let st = "Ready to scan";
      if (done) st = `Scan complete · ${SCAN_ITEMS.toLocaleString()} folders · 4.2s`;
      else if (lastVis) st = `Scanning ${lastVis.path} · ${Math.round(SCAN_ITEMS * prog).toLocaleString()} folders`;
      setT(X("sstat")[0], st);
      const b = X("sbtn")[0]; if (b) b.style.opacity = done ? 1 : 0.45;
    },

    explain(el, p) {
      const X = (k) => q(el, k);
      rise(X("ehead"), ease(seg(p, 0, 0.16)));
      const z = eio(seg(p, 0.14, 0.36));
      X("eg").forEach((g) => { g.style.opacity = (1 - z) * 0.55; g.style.maxHeight = (1 - z) * 68 + "px"; });
      const f = X("ef")[0];
      if (f) { f.style.transform = `scale(${1 + z * 0.035})`; f.style.boxShadow = `${(z * 6).toFixed(1)}px ${(z * 6).toFixed(1)}px 0 #141614`; }
      const u = ease(seg(p, 0.3, 0.58)); const c = X("ecard")[0];
      if (c) { c.style.opacity = Math.min(1, u * 1.6); c.style.transform = `perspective(1600px) rotateX(${((1 - u) * -80).toFixed(2)}deg)`; }
      X("eblk").forEach((b, k) => { const e = ease(seg(p, 0.42 + k * 0.05, 0.6 + k * 0.05)); b.style.opacity = e; b.style.transform = `translateY(${(1 - e) * 18}px)`; });
    },

    quar(el, p, t) {
      const X = (k) => q(el, k);
      rise(X("qhead"), ease(seg(p, 0, 0.12)));
      const stg = X("qstage")[0]; if (stg) { const e = ease(seg(p, 0.02, 0.16)); stg.style.opacity = e; stg.style.transform = `translateY(${(1 - e) * 40}px)`; }
      const trays = X("qtray"); const sizes = [16.0, 12.4, 4.8];
      const r = eio(seg(p, 0.76, 0.92)); let total = 0, any = 0;
      X("qslot").forEach((s) => {
        const i = +s.dataset.i;
        const card = s.querySelector('[data-x="qcard"]'), ph = s.querySelector('[data-x="qph"]'), chk = s.querySelector('[data-x="qchk"]'), tick = s.querySelector('[data-x="qtick"]');
        const sel = i < 3; let on = sel && p >= 0.12 + i * 0.05; if (i === 2 && r >= 1) on = false;
        if (chk) { chk.style.background = on ? "var(--accent)" : "transparent"; chk.style.borderColor = on ? "var(--accent)" : "var(--ink3)"; }
        if (tick) tick.style.opacity = on ? 1 : 0;
        if (!sel) return;
        const tr = trays.find((x) => +x.dataset.i === i); if (!tr || !card || !ph) return;
        const f = eio(seg(p, 0.32 + i * 0.08, 0.5 + i * 0.08)); const k = i === 2 ? f * (1 - r) : f;
        const a = ph.getBoundingClientRect(), b = tr.getBoundingClientRect();
        const dx = b.left - a.left, dy = b.top - a.top, arc = Math.sin(Math.PI * k);
        card.style.transform = `translate(${(dx * k).toFixed(1)}px,${(dy * k - arc * 40).toFixed(1)}px) scale(${(1 + arc * 0.035).toFixed(4)})`;
        card.style.boxShadow = `0 ${arc * 24}px ${arc * 48}px -12px rgba(0,0,0,${(arc * 0.3).toFixed(3)})`;
        card.style.opacity = 1 - seg(k, 0.92, 1);
        ph.style.opacity = seg(k, 0, 0.12);
        s.style.zIndex = k > 0 ? 5 : 1;
        const tro = i === 2 && r > 0 ? 0 : seg(f, 0.9, 1);
        tr.style.opacity = tro; total += sizes[i] * tro; any = Math.max(any, tro);
        if (tro >= 1) { if (tr.__arr == null) tr.__arr = t; } else tr.__arr = null;
        let txt = "Restorable for 14 days";
        if (S.mode !== "static" && tr.__arr != null) {
          const rem = 14 * 86400 - (t - tr.__arr), pad = (n) => String(n).padStart(2, "0");
          txt = `Restorable for ${Math.floor(rem / 86400)}d ${pad(Math.floor((rem % 86400) / 3600))}:${pad(Math.floor((rem % 3600) / 60))}:${pad(Math.floor(rem % 60))}`;
        }
        setT(tr.querySelector('[data-x="qcd"]'), txt);
        if (i === 2) {
          const rb = tr.querySelector('[data-x="qrbtn"]');
          if (rb) { const hi = seg(p, 0.68, 0.73); rb.style.background = `color-mix(in oklch, var(--accent) ${(hi * 100).toFixed(0)}%, transparent)`; rb.style.color = hi > 0.5 ? "var(--on-accent)" : "var(--ink)"; rb.style.borderColor = hi > 0.5 ? "transparent" : "var(--hair)"; }
        }
      });
      const em = X("qempty")[0]; if (em) em.style.opacity = 1 - any;
      setT(X("qtotal")[0], total.toFixed(1) + " GB");
      X("qrest").forEach((e) => { e.style.opacity = +e.dataset.i === 2 ? seg(p, 0.9, 0.95) : 0; });
      const btn = X("qbtn")[0]; if (btn) { const pr = seg(p, 0.27, 0.3) - seg(p, 0.3, 0.33); btn.style.transform = `scale(${1 - 0.05 * pr})`; btn.style.opacity = p >= 0.12 && p < 0.34 ? 1 : 0.45; }
      const n = [0, 1, 2].filter((i) => p >= 0.12 + i * 0.05).length;
      let st = "Nothing selected";
      if (p >= 0.92) st = "npm cache restored";
      else if (p >= 0.66) st = "3 items in the vault";
      else if (p >= 0.32) st = "Moving to the vault…";
      else if (n) st = `${n} selected · ${[16.0, 28.4, 33.2][n - 1].toFixed(1)} GB`;
      setT(X("qstat")[0], st);
    },

    privacy(el, p, t) {
      const X = (k) => q(el, k);
      rise(X("phead"), ease(seg(p, 0, 0.22)));
      const st = X("pstage")[0], e = ease(seg(p, 0.08, 0.36));
      if (st) { st.style.opacity = e; st.style.transform = `scale(${(0.94 + 0.06 * e).toFixed(4)})`; }
      const g = ease(seg(p, 0.22, 0.5));
      X("pring").concat(X("pglow")).forEach((x) => (x.style.opacity = g));
      const a = seg(p, 0.3, 0.5), still = S.mode === "static";
      X("pdot").forEach((d, i) => {
        const ph = still ? ((i * 0.37) % 1) * 0.55 : (t * 0.14 + i * 0.383) % 1;
        const x = 12 + ph * 32;
        const o = a * seg(x, 12, 17) * (1 - seg(x + 6, 35, 41));
        d.style.transform = `translateX(${x.toFixed(2)}cqw)`; d.style.opacity = o.toFixed(3);
      });
      X("pbar").forEach((b, i) => { const w = still ? 40 + i * 10 : 22 + 68 * (0.5 + 0.5 * Math.sin(t * (0.9 + i * 0.21) + i * 1.9)); b.style.width = w.toFixed(1) + "%"; });
    },
  };

  // ---------- hero pile ----------

  // A tank of leftovers you can throw around. "Clean up leftovers" sends
  // the safe ones to the vault and leaves the two that need a person's call.
  // Matter.js is served from this site; without it the pile just sits still.
  const PILE = [
    ["node_modules", 6, "pkg"], ["node_modules", 5, "pkg"], ["node_modules", 4, "pkg"], ["node_modules", 3, "pkg"], [".venv", 3, "pkg"], ["pnpm-store", 4, "pkg"], [".cargo/registry", 3, "pkg"],
    ["ollama/models", 14, "models"], ["huggingface/hub", 10, "models"], ["LM Studio models", 7, "models"],
    ["Docker build cache", 12, "docker"], ["buildx cache", 4, "docker"],
    ["DerivedData", 8, "build"], ["target/", 6, "build"], ["target/", 4, "build"], ["CoreSimulator", 6, "build"], [".next", 2, "build"], ["dist/", 1, "build"], ["build/", 2, "build"], [".gradle", 3, "build"], ["Pods/", 3, "build"],
    ["__pycache__", 1, "cache"], [".turbo", 1, "cache"], [".expo", 1, "cache"], ["~/.cache", 3, "cache"], [".npm/_cacache", 3, "cache"], ["uv cache", 2, "cache"], ["Homebrew cache", 2, "cache"],
  ];
  const REVIEW = [["Docker volumes", 5.1], ["postgres data", 2.1]];
  const PCOL = { pkg: "#B7F36B", models: "#CDB8FF", docker: "#9FD3FF", build: "#FFB27A", cache: "#FFE27A", review: "#FFFFFF" };
  const PL = { want: performance.now() + 450, pile: null, eng: null, drag: null, mode: "full", used: 332 };
  const $x = (k) => root.querySelector(`[data-x="${k}"]`);

  function pileLabels(btn, msg) {
    if (btn != null) setT($x("hbtnl"), btn);
    if (msg != null) setT($x("hmsg"), msg);
  }

  function pileClear() {
    if (PL.pile) PL.pile.chips.forEach((c) => c.el.remove());
    if (PL.eng && window.Matter) { Matter.Composite.clear(PL.eng.world, false); Matter.Engine.clear(PL.eng); }
    PL.eng = null; PL.drag = null; PL.pile = null;
  }

  function pileBuild() {
    const hero = $x("hero"), box = $x("pile"), tank = $x("tank");
    if (!hero || !box || !tank) return;
    pileClear();
    PL.hero = hero;
    const M = window.Matter, W = hero.clientWidth, H = hero.clientHeight, hr = hero.getBoundingClientRect(), tr = tank.getBoundingClientRect();
    const T = { x0: tr.left - hr.left + 2, x1: tr.right - hr.left - 2, y0: tr.top - hr.top, y1: tr.bottom - hr.top - 2 };
    const sc = W < 640 ? 0.74 : W < 1000 ? 0.88 : 1;
    const target = (T.x1 - T.x0) * (T.y1 - T.y0) * 0.6;
    const pool = PILE.slice().sort(() => Math.random() - 0.5);
    const ink = "#141614", mono = "ui-monospace,'SF Mono',SFMono-Regular,Menlo,monospace", sans = "'Schibsted Grotesk',-apple-system,sans-serif";
    const chips = []; let area = 0, i = 0;
    const mk = (label, gb, cat, review) => {
      const h = Math.round((36 + Math.min(1, gb / 14) * 26) * sc), fs = Math.round(h * 0.34);
      const el = document.createElement("div");
      el.setAttribute("data-chip", "");
      el.style.cssText = `position:absolute;left:0;top:0;height:${h}px;padding:0 ${Math.round(h * 0.4)}px;display:flex;align-items:center;gap:${Math.round(h * 0.2)}px;border:2px solid ${ink};border-radius:999px;background:${PCOL[cat]};color:${ink};box-shadow:3px 3px 0 ${ink};white-space:nowrap;font-family:${mono};font-size:${fs}px;font-weight:600;letter-spacing:-0.02em;cursor:grab;user-select:none;-webkit-user-select:none;touch-action:manipulation;pointer-events:auto;will-change:transform;transform:translate(-9999px,0);`;
      const lab = document.createElement("span"); lab.textContent = label; el.appendChild(lab);
      const tag = document.createElement("span");
      if (review) { tag.textContent = "review"; tag.style.cssText = `font-family:${sans};font-size:${Math.round(fs * 0.8)}px;font-weight:700;background:#FF9F0A;border:1.5px solid ${ink};border-radius:999px;padding:1px ${Math.round(fs * 0.5)}px;`; }
      else { tag.setAttribute("data-gb", ""); tag.style.cssText = `font-family:${sans};font-size:${Math.round(fs * 0.86)}px;font-weight:500;opacity:.62;`; }
      el.appendChild(tag);
      box.appendChild(el);
      const c = { el, w: el.offsetWidth, h, gb, cat, review, crumb: false };
      el.__c = c; chips.push(c); area += c.w * h; return c;
    };
    REVIEW.forEach(([l, g]) => mk(l, g, "review", true));
    while (area < target && chips.length < 56) { const [l, wg, cat] = pool[i++ % pool.length]; mk(l, wg * (0.6 + Math.random() * 0.8), cat, false); }
    const safe = chips.filter((c) => !c.review), total = safe.reduce((a, c) => a + c.gb, 0) || 1;
    safe.forEach((c) => { c.gb = Math.max(0.1, Math.round((c.gb * 140.8) / total * 10) / 10); c.el.querySelector("[data-gb]").textContent = c.gb.toFixed(1) + " GB"; c.w = c.el.offsetWidth; });
    const nCr = Math.round(8 + W / 110);
    for (let k = 0; k < nCr; k++) {
      const d = Math.round((7 + Math.random() * 9) * sc), el = document.createElement("div");
      el.style.cssText = `position:absolute;left:0;top:0;width:${d}px;height:${d}px;border-radius:50%;background:var(--ink);pointer-events:none;will-change:transform;transform:translate(-9999px,0);`;
      box.appendChild(el);
      const c = { el, w: d, h: d, gb: 0, crumb: true }; el.__c = c; chips.push(c);
    }
    PL.pile = { W, H, chips, sc, T };
    if (M) {
      const eng = M.Engine.create(); eng.gravity.y = 1.15; eng.positionIterations = 8; eng.velocityIterations = 6;
      const wall = { isStatic: true, friction: 0.6 };
      const cx = (T.x0 + T.x1) / 2, tw = T.x1 - T.x0;
      M.Composite.add(eng.world, [
        M.Bodies.rectangle(cx, T.y1 + 60, tw + 400, 120, wall),
        M.Bodies.rectangle(T.x0 - 60, T.y1 - H * 1.5, 120, H * 3, wall),
        M.Bodies.rectangle(T.x1 + 60, T.y1 - H * 1.5, 120, H * 3, wall),
        M.Bodies.rectangle(T.x0 + 14, T.y1 - 14, 40, 40, Object.assign({ angle: Math.PI / 4 }, wall)),
        M.Bodies.rectangle(T.x1 - 14, T.y1 - 14, 40, 40, Object.assign({ angle: Math.PI / 4 }, wall)),
      ]);
      PL.eng = eng;
      chips.slice().sort(() => Math.random() - 0.5).forEach((c, k) => { pileSpawn(c, k); M.Composite.add(eng.world, c.body); });
      if (S.mode === "static") for (let k = 0; k < 700; k++) M.Engine.update(eng, 1000 / 60);
    } else pileStatic();
    if (!box.__pd) { box.__pd = 1; box.addEventListener("pointerdown", pileDown); }
    PL.mode = "full";
    pileLabels("Clean up leftovers", "Everything in there is throwable. Go on.");
  }

  function pileSpawn(c, k) {
    const M = window.Matter, P = PL.pile, T = P.T;
    const x = T.x0 + c.w / 2 + 4 + Math.random() * Math.max(1, T.x1 - T.x0 - c.w - 8), y = -c.h - 40 - k * 58 * P.sc;
    const opt = { restitution: c.crumb ? 0.45 : 0.22, friction: 0.5, frictionAir: 0.012, density: 0.0015, angle: (Math.random() - 0.5) * 0.9 };
    c.body = c.crumb ? M.Bodies.circle(x, y, c.w / 2, opt) : M.Bodies.rectangle(x, y, c.w, c.h, Object.assign({ chamfer: { radius: c.h / 2 - 0.5 } }, opt));
  }

  function pileStatic() {
    const P = PL.pile, T = P.T; let x = T.x0 + 10, y = T.y1 - 6, rowH = 0;
    P.chips.filter((c) => !c.gone).slice().sort(() => Math.random() - 0.5).forEach((c) => {
      if (x + c.w > T.x1 - 10) { x = T.x0 + 10; y -= rowH + 4; rowH = 0; }
      c.sx = x + c.w / 2; c.sy = y - c.h / 2; c.sa = (Math.random() - 0.5) * 0.14; x += c.w + 6; rowH = Math.max(rowH, c.h);
    });
  }

  function pileAct() {
    const P = PL.pile; if (!P) return;
    const now = performance.now(), M = window.Matter;
    if (PL.mode === "full") {
      const hr = PL.hero.getBoundingClientRect(), br = $x("hbtn").getBoundingClientRect();
      const tx = br.left + br.width / 2 - hr.left, ty = br.top + br.height / 2 - hr.top;
      const pos = (c) => (c.body ? c.body.position : { x: c.sx, y: c.sy });
      const list = P.chips.filter((c) => !c.review && !c.gone);
      list.sort((a, b) => Math.hypot(pos(a).x - tx, pos(a).y - ty) - Math.hypot(pos(b).x - tx, pos(b).y - ty));
      const still = S.mode === "static";
      list.forEach((c, k) => { c.fly = { t0: now + (still ? 0 : k * 26), d: still ? 1 : 520 + Math.random() * 180, tx, ty }; });
      PL.mode = "cleaning";
      pileLabels("Cleaning…", "Moving everything to the vault…");
    } else if (PL.mode === "clean") {
      const back = P.chips.filter((c) => c.gone);
      back.forEach((c, k) => { c.gone = false; c.el.style.display = ""; c.el.style.opacity = 1; if (M && PL.eng) { pileSpawn(c, k); M.Composite.add(PL.eng.world, c.body); } });
      if (M && PL.eng && S.mode === "static") for (let k = 0; k < 700; k++) M.Engine.update(PL.eng, 1000 / 60);
      if (!M) pileStatic();
      PL.mode = "full";
      pileLabels("Clean up leftovers", "Put back from the vault, exactly as it was.");
    }
  }

  function pileDown(e) {
    const el = e.target.closest && e.target.closest("[data-chip]"), M = window.Matter;
    if (!el || !el.__c || !M || !PL.eng || !PL.hero) return;
    const c = el.__c; if (c.fly || c.gone || !c.body) return;
    if (e.pointerType === "touch") { M.Body.setVelocity(c.body, { x: (Math.random() - 0.5) * 14, y: -15 - Math.random() * 7 }); M.Body.setAngularVelocity(c.body, (Math.random() - 0.5) * 0.35); return; }
    e.preventDefault();
    const hr = PL.hero.getBoundingClientRect(), px = e.clientX - hr.left, py = e.clientY - hr.top, b = c.body;
    pileUp();
    PL.drag = M.Constraint.create({ pointA: { x: px, y: py }, bodyB: b, pointB: { x: px - b.position.x, y: py - b.position.y }, stiffness: 0.2, damping: 0.08, length: 0 });
    M.Composite.add(PL.eng.world, PL.drag);
    el.style.cursor = "grabbing"; PL.dragEl = el;
    window.addEventListener("pointermove", pileMove);
    window.addEventListener("pointerup", pileUp);
  }
  function pileMove(e) { if (!PL.drag || !PL.hero) return; const hr = PL.hero.getBoundingClientRect(); PL.drag.pointA.x = e.clientX - hr.left; PL.drag.pointA.y = e.clientY - hr.top; }
  function pileUp() {
    if (PL.drag && PL.eng) Matter.Composite.remove(PL.eng.world, PL.drag);
    PL.drag = null;
    if (PL.dragEl) { PL.dragEl.style.cursor = "grab"; PL.dragEl = null; }
    window.removeEventListener("pointermove", pileMove); window.removeEventListener("pointerup", pileUp);
  }

  function pileFrame(now, dt, VH) {
    const P = PL.pile;
    if (!P) { if (PL.want && now >= PL.want) { PL.want = 0; pileBuild(); } return; }
    const hero = PL.hero;
    if (Math.abs(hero.clientWidth - P.W) > 40 || Math.abs(hero.clientHeight - P.H) > 120) { if (!PL.rb) PL.rb = now + 350; }
    if (PL.rb && now >= PL.rb) { PL.rb = 0; pileBuild(); return; }
    const r = hero.getBoundingClientRect(); if (r.bottom < 0 || r.top > VH) return;
    const M = window.Matter, eng = PL.eng, still = S.mode === "static";
    if (eng && !still) M.Engine.update(eng, Math.min(dt, 1 / 30) * 1000);
    let used = 332, flying = 0, freed = 0;
    for (const c of P.chips) {
      if (c.gone) { freed += c.gb; continue; }
      if (c.fly && now >= c.fly.t0) {
        const f = c.fly;
        if (!f.on) {
          f.on = 1;
          const p = c.body ? c.body.position : { x: c.sx, y: c.sy };
          f.x0 = p.x; f.y0 = p.y; f.a0 = c.body ? c.body.angle : c.sa;
          if (c.body && eng) { if (PL.drag && PL.drag.bodyB === c.body) pileUp(); M.Composite.remove(eng.world, c.body); c.body = null; }
        }
        const k = clamp((now - f.t0) / f.d), e = k * k;
        const cx = f.x0 + (f.tx - f.x0) * 0.25, cy = Math.min(f.y0, f.ty) - 160, u = 1 - e;
        const x = u * u * f.x0 + 2 * u * e * cx + e * e * f.tx, y = u * u * f.y0 + 2 * u * e * cy + e * e * f.ty;
        c.el.style.transform = `translate3d(${(x - c.w / 2).toFixed(1)}px,${(y - c.h / 2).toFixed(1)}px,0) rotate(${(f.a0 + e * 5).toFixed(3)}rad) scale(${(1 - 0.88 * e).toFixed(3)})`;
        c.el.style.opacity = (1 - seg(k, 0.72, 1)).toFixed(3);
        used += c.gb * (1 - e); freed += c.gb * e;
        if (k >= 1) { c.gone = true; c.fly = null; c.el.style.display = "none"; } else flying++;
        continue;
      }
      if (c.fly) flying++;
      let x, y, a;
      if (c.body) { x = c.body.position.x; y = c.body.position.y; a = c.body.angle; }
      else { x = c.sx; y = c.sy; a = c.sa; }
      if (x == null) continue;
      c.el.style.transform = `translate3d(${(x - c.w / 2).toFixed(1)}px,${(y - c.h / 2).toFixed(1)}px,0) rotate(${a.toFixed(3)}rad)`;
      if (y > -c.h / 2) used += c.gb;
    }
    if (PL.mode === "cleaning" && !flying) {
      PL.mode = "clean";
      pileLabels("Undo", `Moved ${freed.toFixed(1)} GB to the vault. Kept 2 items that need your call. Undo works for 14 days.`);
    }
    PL.used = still ? used : PL.used + (used - PL.used) * (1 - Math.exp(-dt * 7));
    const u = PL.used;
    setT($x("hgb"), Math.round(u) + "");
    const bar = $x("hbar"); if (bar) { bar.style.width = (u / 5).toFixed(2) + "%"; bar.style.background = u > 440 ? "var(--warn)" : "var(--accent)"; }
    const st = $x("hstate"); if (st) { const full = u > 440; setT(st, full ? "Almost full" : "Plenty of room"); st.style.color = full ? "var(--warn-ink)" : "var(--accent-ink)"; }
  }

  $x("hbtn")?.addEventListener("click", pileAct);
  $x("brew")?.addEventListener("click", async (e) => {
    const b = e.currentTarget, lab = b.querySelector("[aria-live]");
    try { await navigator.clipboard.writeText(b.dataset.copy); setT(lab, "Copied"); } catch (_) { setT(lab, "Select it"); }
    setTimeout(() => setT(lab, "Copy"), 1600);
  });

  // ---------- terminal ----------

  function cli(t, mode, H) {
    const term = S.termEl && S.termEl.isConnected ? S.termEl : (S.termEl = root.querySelector('[data-x="term"]'));
    if (!term) return;
    const r = term.getBoundingClientRect();
    if (S.cliStart == null && r.top < H * 0.72 && r.bottom > 0) S.cliStart = t;
    if ((r.bottom < -50 || r.top > H + 50) && S.cliPainted) return;
    S.cliPainted = 1;
    const T = mode === "static" ? 1e6 : S.cliStart == null ? -1 : t - S.cliStart;
    let c = 0.5, car = null, typing = false;
    const carets = [];
    term.querySelectorAll('[data-x="tl"]').forEach((ln) => {
      const pause = parseFloat(ln.dataset.pause) || 0;
      if (ln.dataset.kind === "cmd") {
        const txt = ln.dataset.text || "", start = c, cps = 0.055;
        const typed = T < start ? 0 : Math.min(txt.length, Math.floor((T - start) / cps));
        ln.style.opacity = T >= start ? 1 : 0;
        setT(ln.querySelector('[data-x="tcmd"]'), txt.slice(0, typed));
        const ce = ln.querySelector('[data-x="tcar"]'); if (ce) carets.push(ce);
        if (T >= start) { car = ce; typing = typed < txt.length; }
        c = start + txt.length * cps + pause;
      } else {
        const on = T >= c; ln.style.opacity = on ? 1 : 0;
        if (on) car = null;
        c += pause || 0.05;
      }
    });
    carets.forEach((ce) => { ce.style.opacity = ce === car ? (typing || mode === "static" || Math.floor(t * 1.8) % 2 === 0 ? 1 : 0) : 0; });
  }

  // ---------- downloads ----------

  // Point the Download buttons at the build for this visitor's OS. Anything
  // we can't tell (phones, unknown systems) goes to the full list.
  function detectOS() {
    const p = ((navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || navigator.userAgent || "").toLowerCase();
    const ua = navigator.userAgent.toLowerCase();
    if (/iphone|ipad|android/.test(ua)) return null;
    if (p.includes("mac")) return { key: "mac", name: "Mac" };
    if (p.includes("win")) return { key: "windows", name: "Windows" };
    if (p.includes("linux")) return { key: /aarch64|arm64/.test(p + ua) ? "linux-arm" : "linux", name: "Linux" };
    return null;
  }

  // Show the install steps for one platform (both Linux builds share theirs).
  function showSteps(key) {
    const which = key.startsWith("linux") ? "linux" : key;
    const box = document.getElementById("next-steps");
    if (!box) return;
    box.querySelectorAll("[data-steps]").forEach((d) => (d.hidden = d.dataset.steps !== which));
    box.classList.add("shown");
  }

  const os = detectOS();
  if (os) {
    const link = document.querySelector(`.dl[data-os="${os.key}"]`);
    if (link) {
      link.classList.add("mine");
      document.querySelectorAll("[data-dl]").forEach((a) => (a.href = link.href));
      document.querySelectorAll("[data-dl-os]").forEach((s) => (s.textContent = " for " + os.name));
      showSteps(os.key);
    }
  }

  // Downloading starts as usual; the steps for that platform appear beside it.
  document.addEventListener("click", (e) => {
    const tile = e.target.closest(".dl");
    if (tile) {
      document.querySelectorAll(".dl.mine").forEach((d) => d.classList.remove("mine"));
      tile.classList.add("mine");
      showSteps(tile.dataset.os);
      return;
    }
    if (e.target.closest("[data-dl]") && os) {
      showSteps(os.key);
      setTimeout(() => document.getElementById("next-steps")?.scrollIntoView({ behavior: rm.matches ? "auto" : "smooth", block: "center" }), 150);
    }
  });

  document.addEventListener("click", async (e) => {
    const b = e.target.closest(".copy");
    if (!b) return;
    try {
      await navigator.clipboard.writeText(b.dataset.copy);
      b.textContent = "Copied";
    } catch (_) {
      b.textContent = "Select it";
    }
    setTimeout(() => (b.textContent = "Copy"), 1600);
  });

  // Comparison table: on phones, show Leftover beside one other tool at a
  // time. Wider screens ignore this and show every column.
  const cmp = root.querySelector(".cmp"), pick = root.querySelector(".cmp-pick");
  if (cmp && pick) {
    const show = (key) => {
      cmp.querySelectorAll("[data-col]").forEach((c) => c.classList.toggle("off", c.dataset.col !== "leftover" && c.dataset.col !== key));
      pick.querySelectorAll("[data-pick]").forEach((b) => {
        const on = b.dataset.pick === key;
        b.setAttribute("aria-pressed", String(on));
        // Keep the chosen chip in view without scrolling the page.
        if (on && (b.offsetLeft < pick.scrollLeft || b.offsetLeft + b.offsetWidth > pick.scrollLeft + pick.clientWidth)) pick.scrollLeft = b.offsetLeft - 60;
      });
    };
    pick.hidden = false;
    cmp.classList.add("picked");
    show(pick.querySelector('[aria-pressed="true"]')?.dataset.pick || "devcleaner");
    pick.addEventListener("click", (e) => { const b = e.target.closest("[data-pick]"); if (b) show(b.dataset.pick); });
  }

  const loop = (now) => {
    requestAnimationFrame(loop);
    try { frame(now); } catch (err) { if (!S.err) { S.err = 1; console.error(err); } }
  };
  requestAnimationFrame(loop);
})();
