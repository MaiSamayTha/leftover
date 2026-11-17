// Leftover site. A little scroll choreography and nothing else: no
// analytics, no cookies, no requests to anyone.
"use strict";

const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

// Fade sections up as they come into view.
const revealer = new IntersectionObserver((entries) => {
  for (const e of entries) {
    if (e.isIntersecting) {
      e.target.classList.add("in");
      revealer.unobserve(e.target);
    }
  }
}, { threshold: 0.15, rootMargin: "0px 0px -40px 0px" });
document.querySelectorAll(".reveal").forEach((el) => revealer.observe(el));

// Run a scene once, the first time it is mostly on screen.
function onView(el, fn, threshold = 0.45) {
  if (!el) return;
  if (reduced) { fn(true); return; }
  const io = new IntersectionObserver((entries) => {
    if (entries.some((e) => e.isIntersecting)) {
      io.disconnect();
      fn(false);
    }
  }, { threshold });
  io.observe(el);
}

const wait = (ms) => new Promise((r) => setTimeout(r, reduced ? 0 : ms));

// Hero: the disk fills up almost to red, then splits into what's filling it.
onView(document.getElementById("disk"), async () => {
  const disk = document.getElementById("disk");
  disk.querySelector(".disk-fill").style.width = "96.4%";
  await wait(1900);
  disk.querySelectorAll(".disk-segs span").forEach((s) => { s.style.flexGrow = s.dataset.grow; });
  disk.classList.add("is-split");
  const legend = disk.querySelectorAll(".disk-legend span");
  legend.forEach((s, i) => { s.style.transitionDelay = 200 + i * 90 + "ms"; });
  document.getElementById("disk-label").textContent = "134 GB of it is leftovers";
}, 0.6);

// Scan: folders stream in while the totals count up, then the list sorts
// itself largest first.
const found = [
  ["Llama-3.1-8B", "~/.cache/huggingface/hub", 16.1e9],
  ["node_modules", "~/code/acme-web", 1.4e9],
  ["Derived Data", "~/Library/Developer/Xcode", 9.8e9],
  ["target", "~/code/engine", 6.2e9],
  ["Virtual environment", "~/code/ml-notebooks/.venv", 4.1e9],
  ["npm cache", "~/.npm/_cacache", 2.7e9],
  ["Gradle caches", "~/.gradle/caches", 3.3e9],
];

function fmt(n) {
  if (n < 1e3) return Math.round(n) + " B";
  const u = ["KB", "MB", "GB", "TB"];
  let i = -1;
  do { n /= 1000; i++; } while (n >= 1000 && i < 3);
  return (n >= 100 ? n.toFixed(0) : n.toFixed(1)) + " " + u[i];
}

function countUp(el, to, ms) {
  if (reduced) { el.textContent = fmt(to); return; }
  const start = performance.now();
  const from = parseFloat(el.dataset.v || 0);
  const tick = (t) => {
    const p = Math.min(1, (t - start) / ms);
    const v = from + (to - from) * (1 - Math.pow(1 - p, 3));
    el.textContent = fmt(v);
    el.dataset.v = v;
    if (p < 1) requestAnimationFrame(tick);
  };
  requestAnimationFrame(tick);
}

onView(document.getElementById("scan-window"), async () => {
  const list = document.getElementById("scan-rows");
  const total = document.getElementById("scan-total");
  const folders = document.getElementById("scan-folders");
  list.style.height = found.length * 46 + "px";
  let sum = 0;
  const rows = [];
  for (let i = 0; i < found.length; i++) {
    const [name, where, size] = found[i];
    const li = document.createElement("li");
    li.className = "row";
    li.innerHTML = `<span class="row-name"></span><span class="row-size">0 B</span>`;
    li.querySelector(".row-name").innerHTML = `${name}<small></small>`;
    li.querySelector("small").textContent = where;
    li.style.transform = `translateY(${i * 46}px)`;
    li.style.opacity = "0";
    list.appendChild(li);
    rows.push({ li, size });
    await wait(60);
    li.style.opacity = "1";
    countUp(li.querySelector(".row-size"), size, 900);
    sum += size;
    countUp(total, sum, 900);
    folders.textContent = (12000 + i * 31877).toLocaleString();
    await wait(380);
  }
  await wait(700);
  document.getElementById("scan-status").textContent = "Done. Sorted largest first.";
  rows.sort((a, b) => b.size - a.size).forEach((r, i) => { r.li.style.transform = `translateY(${i * 46}px)`; });
});

// Explain: a row unfolds into its verdict card.
onView(document.getElementById("explain-stage"), async () => {
  await wait(300);
  document.getElementById("explain-stage").classList.add("open");
}, 0.5);

// Vault: picked items lift out of the list into the vault, then one floats
// back to where it came from.
onView(document.getElementById("vault-stage"), async () => {
  const stage = document.getElementById("vault-stage");
  const items = [...stage.querySelectorAll(".v-row[data-pick]")];
  for (const it of items) { it.classList.add("picked"); await wait(260); }
  await wait(500);
  for (const it of items) { it.classList.add("lifted"); await wait(180); }
  await wait(700);
  stage.classList.add("held");
  await wait(1600);
  stage.classList.add("restoring");
  await wait(900);
  stage.classList.add("restored");
  stage.querySelector(".v-count").textContent = "16.0 GB · 14 days left";
}, 0.5);

// Privacy: the machine scans inside its glowing boundary; anything reaching
// outward fades out at the edge.
onView(document.getElementById("boundary"), () => {
  document.getElementById("boundary").classList.add("go");
}, 0.4);

// Command line: type out a short session.
const session = [
  ["cmd", "leftover scan"],
  ["out", `<span class="b">Safe to clear</span>  <span class="m">38.4 GB</span>
  <span class="g">●</span> 16.1 GB  Llama-3.1-8B   <span class="m">~/.cache/huggingface</span>
  <span class="g">●</span>  9.8 GB  Derived Data   <span class="m">~/Library/Developer</span>
  <span class="g">●</span>  6.2 GB  target         <span class="m">~/code/engine</span>
  <span class="g">●</span>  1.2 GB  node_modules   <span class="m">~/code/old-site</span>

<span class="b">Worth a look</span>  <span class="m">4.6 GB</span>
  <span class="o">●</span>  4.6 GB  pnpm store     <span class="m">~/Library/pnpm</span>
`],
  ["cmd", "leftover why ~/code/engine/target"],
  ["out", `<span class="g">●</span> <span class="b">target</span>  <span class="m">Cargo · Build output</span>
Everything Cargo compiled for this project.
<span class="b">Verdict:</span> Rebuilds itself
<span class="b">Source:</span> doc.rust-lang.org/cargo/…/build-cache
`],
  ["cmd", "leftover clean --idle 90"],
  ["out", `Move 2 item(s), 7.4 GB, to the vault? [y/N] y
<span class="g">Moved 7.4 GB to the vault.</span>
<span class="m">Restorable for 14 days.</span>
`],
];

onView(document.getElementById("terminal"), async (instant) => {
  const term = document.getElementById("term");
  let html = "";
  const show = (extra = "") => { term.innerHTML = html + extra; };
  for (const [kind, text] of session) {
    if (kind === "cmd") {
      html += `<span class="p">~ %</span> `;
      if (instant) { html += text + "\n"; continue; }
      for (let i = 1; i <= text.length; i++) {
        show(text.slice(0, i) + `<span class="cursor"></span>`);
        await wait(28 + Math.random() * 40);
      }
      html += text + "\n";
      show();
      await wait(300);
    } else {
      html += text + "\n";
      show();
      await wait(900);
    }
  }
  html += `<span class="p">~ %</span> `;
  show(`<span class="cursor"></span>`);
}, 0.4);

// Download: highlight the build for this machine, and show the right
// install command.
(() => {
  const ua = navigator.userAgent;
  const isWin = /Windows/.test(ua);
  const isMac = /Macintosh|Mac OS X/.test(ua);
  const pick = isWin ? "windows" : isMac ? "mac" : /Linux/.test(ua) ? "linux" : "";
  if (pick) document.querySelector(`.dl[data-os="${pick}"]`)?.classList.add("mine");
  document.querySelector("[data-win]").hidden = !isWin;
  document.querySelector("[data-unix]").hidden = isWin;
  document.querySelectorAll(".copy").forEach((b) => {
    b.addEventListener("click", async () => {
      const text = b.parentElement.querySelector("code").textContent;
      try {
        await navigator.clipboard.writeText(text);
        b.textContent = "Copied";
      } catch (_) {
        b.textContent = "Select and copy";
      }
      setTimeout(() => (b.textContent = "Copy"), 1600);
    });
  });
})();
