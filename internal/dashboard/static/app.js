async function getJSON(path) {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${path}: ${res.status}`);
  return res.json();
}

const fmt = {
  int(n) {
    if (n == null) return "—";
    return new Intl.NumberFormat("en-US").format(Math.round(n));
  },
  usd(n) {
    if (n == null) return "—";
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      maximumFractionDigits: 2,
    }).format(n);
  },
  compact(n) {
    if (n == null) return "—";
    return new Intl.NumberFormat("en-US", {
      notation: "compact",
      maximumFractionDigits: 1,
    }).format(n);
  },
  day(s) {
    if (!s) return "—";
    return s.slice(5); // MM-DD
  },
  when(s) {
    if (!s) return "—";
    return s.replace("T", " ").replace(/\.\d+Z$/, "Z");
  },
};

function kpi(label, value) {
  return `<div class="kpi"><p class="label">${label}</p><p class="value">${value}</p></div>`;
}

function renderKpis(s) {
  document.getElementById("kpis").innerHTML = [
    kpi("Sources", fmt.int(s.sources)),
    kpi("Rated USD", fmt.usd(s.rated_cost_usd)),
    kpi("Input tokens", fmt.compact(s.tokens_input)),
    kpi("Output tokens", fmt.compact(s.tokens_output)),
    kpi("Cache read", fmt.compact(s.tokens_cache_read)),
    kpi("Cache write 5m/1h", `${fmt.compact(s.tokens_cache_write_5m)} / ${fmt.compact(s.tokens_cache_write_1h)}`),
  ].join("");

  const a = s.first_event_at ? s.first_event_at.slice(0, 10) : "?";
  const b = s.last_event_at ? s.last_event_at.slice(0, 10) : "?";
  document.getElementById("rangeLabel").textContent = `${a} → ${b}`;
}

function tokenTotal(r) {
  return (r.tokens_input || 0) + (r.tokens_output || 0) + (r.tokens_cache_read || 0) + (r.tokens_cache_write || 0);
}

function providerBox(vendor, total, isTotal) {
  return `<div class="provider-box${isTotal ? " provider-box-total" : ""}">
    <p class="label">${escapeHtml(vendor)}</p>
    <p class="value">${fmt.compact(total)}</p>
    <p class="sub">${fmt.int(total)} tokens</p>
  </div>`;
}

function renderProviderBoxes(rows) {
  const sorted = [...rows].sort((a, b) => tokenTotal(b) - tokenTotal(a));
  const grandTotal = sorted.reduce((sum, r) => sum + tokenTotal(r), 0);
  const boxes = sorted.map((r) => providerBox(r.vendor, tokenTotal(r), false));
  boxes.push(providerBox("Total", grandTotal, true));
  document.getElementById("providerBoxes").innerHTML = boxes.join("");
}

function renderVendorBars(rows) {
  const max = Math.max(...rows.map((r) => r.rated_cost_usd || 0), 1);
  document.getElementById("vendorBars").innerHTML = rows
    .map((r) => {
      const pct = ((r.rated_cost_usd || 0) / max) * 100;
      return `<div class="bar-row">
        <div class="name" title="${r.vendor}">${r.vendor}</div>
        <div class="bar-track"><div class="bar-fill" style="width:${pct}%"></div></div>
        <div class="bar-val">${fmt.usd(r.rated_cost_usd)}</div>
      </div>`;
    })
    .join("");
}

function renderDaily(rows) {
  const el = document.getElementById("dailyChart");
  if (!rows.length) {
    el.innerHTML = `<p class="empty">No dated events in range</p>`;
    return;
  }
  const max = Math.max(...rows.map((r) => r.rated_cost_usd || 0), 0.01);
  el.innerHTML = rows
    .map((r) => {
      const h = Math.max(2, ((r.rated_cost_usd || 0) / max) * 140);
      return `<div class="vbar" title="${r.day}: ${fmt.usd(r.rated_cost_usd)}">
        <div class="stem" style="height:${h}px"></div>
        <div class="lbl">${fmt.day(r.day)}</div>
      </div>`;
    })
    .join("");
}

function renderModels(rows) {
  document.getElementById("modelRows").innerHTML = rows
    .map(
      (r) => `<tr>
      <td>${escapeHtml(r.model)}</td>
      <td>${escapeHtml(r.vendor)}</td>
      <td class="num">${fmt.int(r.sources)}</td>
      <td class="num">${fmt.compact(r.tokens_input)}</td>
      <td class="num">${fmt.compact(r.tokens_output)}</td>
      <td class="num">${fmt.usd(r.rated_cost_usd)}</td>
    </tr>`
    )
    .join("");
}

function renderSnaps(rows) {
  document.getElementById("snapRows").innerHTML = rows
    .map((r) => {
      const when = r.last_event_at || r.started_at;
      const crw = `${fmt.compact(r.tokens_cache_read)} / ${fmt.compact(r.tokens_cache_write)}`;
      const win = `${fmt.compact(r.tokens_cache_write_5m)} / ${fmt.compact(r.tokens_cache_write_1h)}`;
      return `<tr>
        <td class="num">${fmt.when(when)}</td>
        <td>${escapeHtml(r.vendor)}</td>
        <td>${escapeHtml(r.model || "—")}</td>
        <td class="num">${fmt.compact(r.tokens_input)}</td>
        <td class="num">${fmt.compact(r.tokens_output)}</td>
        <td class="num">${crw}</td>
        <td class="num">${win}</td>
        <td class="num">${fmt.usd(r.rated_cost_usd)}</td>
        <td class="path" title="${escapeHtml(r.source_path)}">${escapeHtml(r.source_path)}</td>
      </tr>`;
    })
    .join("");
}

function escapeHtml(s) {
  return String(s ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function fillVendorFilter(vendors) {
  const sel = document.getElementById("vendorFilter");
  const cur = sel.value;
  sel.innerHTML =
    `<option value="">All vendors</option>` +
    vendors.map((v) => `<option value="${escapeHtml(v.vendor)}">${escapeHtml(v.vendor)}</option>`).join("");
  sel.value = cur;
}

async function loadSnaps() {
  const vendor = document.getElementById("vendorFilter").value;
  const q = vendor ? `?vendor=${encodeURIComponent(vendor)}&limit=80` : "?limit=80";
  const snaps = await getJSON(`/api/snapshots${q}`);
  renderSnaps(snaps);
}

async function loadAll() {
  const [summary, vendors, models, daily] = await Promise.all([
    getJSON("/api/summary"),
    getJSON("/api/by-vendor"),
    getJSON("/api/by-model?limit=40"),
    getJSON("/api/daily?days=45"),
  ]);
  renderKpis(summary);
  renderProviderBoxes(vendors);
  renderVendorBars(vendors);
  renderModels(models);
  renderDaily(daily);
  fillVendorFilter(vendors);
  await loadSnaps();
}

document.getElementById("refreshBtn").addEventListener("click", () => {
  loadAll().catch((e) => alert(e.message));
});
document.getElementById("vendorFilter").addEventListener("change", () => {
  loadSnaps().catch((e) => alert(e.message));
});

loadAll().catch((e) => {
  document.getElementById("kpis").innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
});
