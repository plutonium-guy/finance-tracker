// charts.js — reads the JSON data island (#chart-data) and renders the dashboard
// charts with Chart.js. Re-initializes after htmx swaps that replace the island.
(function () {
  const COLORS = {
    green: "#16a34a", red: "#dc2626", blue: "#1e40af",
    yellow: "#fbbf24", purple: "#7c3aed", gray: "#6b7280",
  };
  const palette = [
    "#1e40af", "#16a34a", "#dc2626", "#7c3aed", "#fbbf24", "#0891b2",
    "#db2777", "#65a30d", "#ea580c", "#4f46e5", "#0d9488", "#b45309",
    "#9333ea", "#15803d", "#be123c", "#2563eb",
  ];

  // Indian abbreviation: 100000 -> "₹1 L", 10000000 -> "₹1 Cr".
  function inrShort(v) {
    const a = Math.abs(v);
    if (a >= 1e7) return "₹" + (v / 1e7).toFixed(2).replace(/\.00$/, "") + " Cr";
    if (a >= 1e5) return "₹" + (v / 1e5).toFixed(2).replace(/\.00$/, "") + " L";
    return "₹" + v.toLocaleString("en-IN");
  }

  const registry = {};
  function draw(id, cfg) {
    const el = document.getElementById(id);
    if (!el) return;
    if (registry[id]) registry[id].destroy();
    cfg.options = cfg.options || {};
    cfg.options.responsive = true;
    cfg.options.maintainAspectRatio = false;
    cfg.options.plugins = cfg.options.plugins || {};
    cfg.options.plugins.tooltip = {
      callbacks: {
        label: (c) => (c.dataset.label ? c.dataset.label + ": " : "") + inrShort(c.parsed.y ?? c.parsed),
      },
    };
    registry[id] = new Chart(el, cfg);
  }

  function render() {
    const island = document.getElementById("chart-data");
    if (!island || typeof Chart === "undefined") return;
    let d;
    try { d = JSON.parse(island.textContent); } catch (e) { return; }
    const months = (d.months || []).map((m) => {
      const [y, mo] = m.split("-");
      return new Date(y, mo - 1, 1).toLocaleString("en-IN", { month: "short", year: "2-digit" });
    });

    draw("chartIncomeExpense", {
      type: "bar",
      data: { labels: months, datasets: [
        { label: "Income", data: d.income, backgroundColor: COLORS.green },
        { label: "Expense", data: d.expense, backgroundColor: COLORS.red },
      ] },
    });
    draw("chartNet", {
      type: "line",
      data: { labels: months, datasets: [
        { label: "Monthly Net", data: d.net, borderColor: COLORS.blue, tension: 0.3 },
        { label: "Cumulative Net", data: d.cumulativeNet, borderColor: COLORS.purple, tension: 0.3 },
      ] },
    });
    draw("chartPayCycle", {
      type: "bar",
      data: { labels: months, datasets: [
        { label: "Prev Salary", data: d.prevSalary, backgroundColor: COLORS.green },
        { label: "This Month Bills", data: d.expense, backgroundColor: COLORS.red },
      ] },
    });
    draw("chartBuffer", {
      type: "line",
      data: { labels: months, datasets: [
        { label: "Running Buffer", data: d.runningBuffer, borderColor: COLORS.blue, tension: 0.3, fill: false },
      ] },
    });
    draw("chartCategory", {
      type: "doughnut",
      data: { labels: d.categoryLabels, datasets: [
        { data: d.categoryValues, backgroundColor: palette },
      ] },
      options: { plugins: { legend: { position: "right" } } },
    });
    draw("chartTopCategories", {
      type: "bar",
      data: { labels: d.categoryLabels, datasets: [
        { label: "Spent", data: d.categoryValues, backgroundColor: COLORS.blue },
      ] },
      options: { indexAxis: "y" },
    });

    // Month detail doughnut (separate island #month-chart-data).
    const mi = document.getElementById("month-chart-data");
    if (mi) {
      let md;
      try { md = JSON.parse(mi.textContent); } catch (e) { md = null; }
      if (md) {
        draw("chartMonthDoughnut", {
          type: "doughnut",
          data: { labels: md.categoryLabels, datasets: [{ data: md.categoryValues, backgroundColor: palette }] },
          options: { plugins: { legend: { position: "right" } } },
        });
      }
    }
  }

  window.renderCharts = render;
  document.addEventListener("DOMContentLoaded", render);
  document.body && document.body.addEventListener("htmx:afterSwap", render);
  document.addEventListener("htmx:afterSwap", render);
})();
