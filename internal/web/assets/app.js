/* FFBatch dashboard — vanilla JS, no build step. */
(() => {
  "use strict";

  // -------------------------------------------------------------------------
  // Small utilities
  // -------------------------------------------------------------------------
  const $ = (selector, root = document) => root.querySelector(selector);
  const $$ = (selector, root = document) => Array.from(root.querySelectorAll(selector));

  const numberFormat = new Intl.NumberFormat();

  function fmtBytes(bytes) {
    const n = Number(bytes) || 0;
    if (n < 1024) return `${n} B`;
    const units = ["KiB", "MiB", "GiB", "TiB", "PiB"];
    let value = n;
    let index = -1;
    while (value >= 1024 && index < units.length - 1) {
      value /= 1024;
      index += 1;
    }
    return `${value.toFixed(value >= 100 ? 0 : 1)} ${units[index]}`;
  }

  function filesText(count) {
    const n = Number(count) || 0;
    return `${numberFormat.format(n)} file${n === 1 ? "" : "s"}`;
  }

  function fmtClock(ms) {
    const total = Math.max(0, Math.round((Number(ms) || 0) / 1000));
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const seconds = total % 60;
    if (hours > 0) return `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
    return `${minutes}:${String(seconds).padStart(2, "0")}`;
  }

  function fmtSeconds(seconds) {
    const total = Math.max(0, Math.round(Number(seconds) || 0));
    const minutes = Math.floor(total / 60);
    const rest = total % 60;
    return `${minutes}:${String(rest).padStart(2, "0")}`;
  }

  function fmtDurationMs(ms) {
    const total = Math.max(0, Number(ms) || 0) / 1000;
    if (total < 60) return `${total.toFixed(total < 10 ? 1 : 0)}s`;
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const seconds = Math.round(total % 60);
    if (hours > 0) return `${hours}h ${String(minutes).padStart(2, "0")}m`;
    return `${minutes}m ${String(seconds).padStart(2, "0")}s`;
  }

  function fmtTime(iso) {
    if (!iso) return "—";
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return "—";
    return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  }

  function fmtRelative(iso) {
    if (!iso) return "";
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return "";
    const delta = Date.now() - date.getTime();
    if (delta < 45_000) return "just now";
    if (delta < 3_600_000) return `${Math.round(delta / 60_000)}m ago`;
    if (delta < 86_400_000) return `${Math.round(delta / 3_600_000)}h ago`;
    return date.toLocaleDateString();
  }

  function elapsedOf(run) {
    if (!run || !run.startedAt) return 0;
    const start = new Date(run.startedAt).getTime();
    const end = run.finishedAt ? new Date(run.finishedAt).getTime() : Date.now();
    return Math.max(0, end - start);
  }

  function titleCase(value) {
    const text = String(value || "");
    return text ? text[0].toUpperCase() + text.slice(1) : "";
  }

  function modeLabel(mode) {
    return titleCase(mode || "remux");
  }

  function statusLabel(status) {
    switch (status) {
      case "pending": return "queued";
      case "running": return "running";
      case "done": return "done";
      case "skipped": return "skipped";
      case "failed": return "failed";
      case "canceled": return "cancelled";
      case "planned": return "planned";
      case "partial": return "partial";
      default: return status || "unknown";
    }
  }

  function statusTone(status) {
    switch (status) {
      case "done": return "ok";
      case "failed": return "bad";
      case "skipped": case "partial": return "warn";
      case "running": return "info";
      case "planned": return "info";
      default: return "";
    }
  }

  function isAbsolutePath(path) {
    return /^[a-zA-Z]:[\\/]/.test(path) || String(path).startsWith("/");
  }

  function normalizePath(path) {
    return String(path || "").trim().replace(/[\\/]+$/, "").toLowerCase();
  }

  function splitList(value) {
    return String(value || "")
      .split(/[,;]/)
      .map((part) => part.trim().replace(/^\./, ""))
      .filter(Boolean);
  }

  function timecodeSeconds(value) {
    const text = String(value || "").trim();
    if (!text) return 0;
    const parts = text.split(":").map((part) => Number(part));
    if (parts.some((part) => !Number.isFinite(part) || part < 0)) return NaN;
    if (parts.length === 1) return parts[0];
    if (parts.length === 2) return parts[0] * 60 + parts[1];
    if (parts.length === 3) return parts[0] * 3600 + parts[1] * 60 + parts[2];
    return NaN;
  }

  function svgIcon(name) {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("class", "icon");
    svg.setAttribute("aria-hidden", "true");
    const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", `#i-${name}`);
    svg.append(use);
    return svg;
  }

  function spinner() {
    const dot = document.createElement("span");
    dot.className = "spinner";
    return dot;
  }

  function debounce(fn, wait) {
    let handle = 0;
    return (...args) => {
      window.clearTimeout(handle);
      handle = window.setTimeout(() => fn(...args), wait);
    };
  }

  // -------------------------------------------------------------------------
  // State
  // -------------------------------------------------------------------------
  const state = {
    config: null,
    configPath: "",
    status: null,
    recipes: [],
    capabilities: null,
    scan: null,
    selected: new Set(),
    runs: [],
    currentRunID: null,
    runDetail: null,
    activeRun: null,
    renderedRunID: null,
    jobNodes: new Map(),
    filter: "all",
    view: "convert",
    browser: { mode: "source", path: "" },
    eventSource: null,
    live: false,
    doctor: null,
    renderedLogCount: 0,
    lastFinishedRunID: "",
    lastFinishedAt: 0,
    watch: null,
    watchTimer: 0,
    configSyncTimer: 0,
  };

  const RECENT_KEY = "ffbatch.recentFolders";
  const SOURCE_KEY = "ffbatch.lastSource";
  const THEME_KEY = "ffbatch.theme";

  // -------------------------------------------------------------------------
  // API
  // -------------------------------------------------------------------------
  async function api(path, options = {}) {
    const init = { ...options };
    const method = String(init.method || "GET").toUpperCase();
    if (init.body !== undefined && typeof init.body !== "string") {
      init.body = JSON.stringify(init.body);
    }
    if (method !== "GET" && method !== "HEAD") {
      init.headers = { "Content-Type": "application/json", ...(init.headers || {}) };
    }
    const response = await fetch(path, init);
    const text = await response.text();
    let data = null;
    if (text) {
      try { data = JSON.parse(text); } catch { data = { error: text }; }
    }
    if (!response.ok) {
      const message = data && data.error ? data.error : `Request failed (${response.status})`;
      throw new Error(message);
    }
    return data;
  }

  // -------------------------------------------------------------------------
  // Toasts
  // -------------------------------------------------------------------------
  function toast(tone, title, message = "") {
    const holder = $("#toasts");
    const node = document.createElement("div");
    node.className = "toast";
    node.dataset.tone = tone || "info";
    const iconName = tone === "bad" ? "alert" : tone === "ok" ? "check" : tone === "warn" ? "alert" : "spark";
    node.append(svgIcon(iconName));
    const text = document.createElement("div");
    text.className = "toast-text";
    const strong = document.createElement("strong");
    strong.textContent = title;
    text.append(strong);
    if (message) {
      const detail = document.createElement("span");
      detail.textContent = message;
      text.append(detail);
    }
    node.append(text);
    holder.append(node);
    window.setTimeout(() => {
      node.classList.add("is-leaving");
      window.setTimeout(() => node.remove(), 220);
    }, 5200);
  }

  // -------------------------------------------------------------------------
  // Theme
  // -------------------------------------------------------------------------
  function applyTheme(theme) {
    document.documentElement.dataset.theme = theme;
    const icon = $("#themeIcon");
    if (icon) icon.setAttribute("href", theme === "dark" ? "#i-moon" : "#i-sun");
    const button = $("#themeBtn");
    if (button) {
      button.setAttribute("aria-label", theme === "dark" ? "Switch to light theme" : "Switch to dark theme");
    }
    try { localStorage.setItem(THEME_KEY, theme); } catch { /* private mode */ }
  }

  function initTheme() {
    let theme = "";
    try { theme = localStorage.getItem(THEME_KEY) || ""; } catch { /* ignore */ }
    if (!theme) {
      theme = window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
    }
    applyTheme(theme);
  }

  // -------------------------------------------------------------------------
  // Navigation
  // -------------------------------------------------------------------------
  const VIEW_COPY = {
    convert: {
      title: "Convert",
      subtitle: "Stream-copy or re-encode a folder, then watch it happen live.",
    },
    watch: {
      title: "Watch",
      subtitle: "Drop files into a folder; FFBatch converts them once they settle.",
    },
    runs: {
      title: "Runs",
      subtitle: "Live progress, per-file detail and downloadable reports.",
    },
    doctor: {
      title: "Doctor",
      subtitle: "Verify FFmpeg, encoders, configuration and write access.",
    },
  };

  function switchView(view, { updateHash = true } = {}) {
    if (!VIEW_COPY[view]) view = "convert";
    state.view = view;
    $$(".nav-item").forEach((button) => {
      const active = button.dataset.view === view;
      button.classList.toggle("is-active", active);
      if (active) button.setAttribute("aria-current", "page");
      else button.removeAttribute("aria-current");
    });
    $$("[data-view-panel]").forEach((panel) => {
      panel.hidden = panel.dataset.viewPanel !== view;
    });
    $("#viewTitle").textContent = VIEW_COPY[view].title;
    $("#viewSubtitle").textContent = VIEW_COPY[view].subtitle;
    if (updateHash && location.hash !== `#/${view}`) {
      history.replaceState(null, "", `#/${view}`);
    }
    if (view === "runs") refreshRuns({ select: true });
    if (view === "watch") {
      refreshWatch();
      startWatchPolling();
    } else {
      stopWatchPolling();
    }
    if (view === "doctor") loadDoctor();
  }

  // -------------------------------------------------------------------------
  // Theme + navigation wiring
  // -------------------------------------------------------------------------
  function bindChrome() {
    $("#themeBtn").addEventListener("click", () => {
      applyTheme(document.documentElement.dataset.theme === "dark" ? "light" : "dark");
    });
    $$(".nav-item").forEach((button) => {
      button.addEventListener("click", () => switchView(button.dataset.view));
    });
    $("#copyCommand").addEventListener("click", copyCommand);
    $("#activeRunLink").addEventListener("click", () => {
      if (!state.activeRun) return;
      state.currentRunID = state.activeRun.id;
      switchView("runs");
      selectRun(state.activeRun.id);
    });
    const hash = (location.hash || "").replace(/^#\/?/, "");
    if (VIEW_COPY[hash]) switchView(hash, { updateHash: false });
    window.addEventListener("hashchange", () => {
      const next = (location.hash || "").replace(/^#\/?/, "");
      if (VIEW_COPY[next] && next !== state.view) switchView(next, { updateHash: false });
    });
  }

  // -------------------------------------------------------------------------
  // Config bindings
  // -------------------------------------------------------------------------
  const BINDINGS = [
    { id: "format", key: "format" },
    { id: "outputDir", key: "outputDir" },
    { id: "archiveDir", key: "archiveDir" },
    { id: "overwrite", key: "overwrite" },
    { id: "videoCodec", key: "videoCodec" },
    { id: "av1Encoder", key: "av1Encoder" },
    { id: "crf", key: "crf", type: "number" },
    { id: "preset", key: "preset" },
    { id: "audioCodec", key: "audioCodec" },
    { id: "audioBitrate", key: "audioBitrate" },
    { id: "audioTracks", key: "audioTracks" },
    { id: "audioLanguage", key: "audioLanguage" },
    { id: "subtitleCodec", key: "subtitleCodec" },
    { id: "trimStart", key: "trimStart" },
    { id: "trimEnd", key: "trimEnd" },
    { id: "workers", key: "workers", type: "number" },
    { id: "aspect", key: "aspect" },
    { id: "extensions", key: "extensions", type: "list" },
    { id: "faststart", key: "faststart", type: "bool" },
    { id: "genpts", key: "genPts", type: "bool" },
    { id: "verifyOutput", key: "verifyOutput", type: "bool" },
    { id: "hashFiles", key: "hashFiles", type: "bool" },
    { id: "deleteSource", key: "deleteSource", type: "bool" },
    { id: "dryRun", key: "dryRun", type: "bool" },
    { id: "logFile", key: "logFile" },
    { id: "recursive", key: "recursive", type: "bool" },
  ];

  function readBinding(binding) {
    if (!state.config) return;
    const element = document.getElementById(binding.id);
    if (!element) return;
    if (binding.type === "bool") state.config[binding.key] = element.checked;
    else if (binding.type === "number") state.config[binding.key] = Number(element.value) || 0;
    else if (binding.type === "list") state.config[binding.key] = splitList(element.value);
    else state.config[binding.key] = element.value;
  }

  function writeBinding(binding) {
    if (!state.config) return;
    const element = document.getElementById(binding.id);
    if (!element) return;
    const value = state.config[binding.key];
    if (binding.type === "bool") element.checked = Boolean(value);
    else if (binding.type === "list") element.value = Array.isArray(value) ? value.join(", ") : String(value || "");
    else element.value = value ?? "";
  }

  function writeConfigToForm() {
    if (!state.config) return;
    BINDINGS.forEach(writeBinding);
    setMode(state.config.mode || "remux", { silent: true });
    syncCrf();
    renderCommandPreview();
    updateModeUI();
    updateLaunchSummary();
  }

  function bindSettings() {
    BINDINGS.forEach((binding) => {
      const element = document.getElementById(binding.id);
      if (!element) return;
      const handler = () => {
        readBinding(binding);
        if (binding.key === "recursive" || binding.key === "extensions") invalidateScan();
        updateDerivedUI();
      };
      element.addEventListener("input", handler);
      element.addEventListener("change", handler);
    });

    $$("[data-mode]").forEach((button) => {
      button.addEventListener("click", () => setMode(button.dataset.mode));
    });

    $("#crf").addEventListener("input", () => {
      syncCrf();
      readBinding(BINDINGS.find((binding) => binding.id === "crf"));
      $("#runEstimate").textContent = launchEstimate();
    });

    $("#audioCodec").addEventListener("change", updateModeUI);
    $("#videoCodec").addEventListener("change", () => {
      readBinding(BINDINGS.find((binding) => binding.id === "videoCodec"));
      updateModeUI();
      renderCommandPreview();
      updateLaunchSummary();
      scheduleConfigSync();
    });

    $("#format").addEventListener("change", () => {
      readBinding(BINDINGS.find((binding) => binding.id === "format"));
      updateDerivedUI();
    });
  }

  function setMode(mode, { silent = false } = {}) {
    if (!state.config) return;
    const previous = state.config.mode;
    state.config.mode = mode;
    $$("[data-mode]").forEach((button) => {
      button.classList.toggle("is-active", button.dataset.mode === mode);
    });
    if (state.scan && (mode === "rename") !== (previous === "rename")) {
      state.scan = null;
      state.selected.clear();
      renderScanPreview();
    }
    updateModeUI();
    renderCommandPreview();
    updateLaunchSummary();
    updateStartState();
    if (!silent) {
      scheduleConfigSync();
      $$("[data-recipe]").forEach((chip) => chip.classList.remove("is-active"));
    }
  }

  function updateModeUI() {
    if (!state.config) return;
    const mode = state.config.mode;
    const notes = {
      remux: "Streams are copied untouched. Fast and lossless — ideal for TS recordings and archiving.",
      encode: "Video is rebuilt with x264 or x265. Slower, smaller files, plays on virtually anything.",
      rename: "Only the extension changes. No media data is read or rewritten.",
    };
    $("#modeNote").textContent = notes[mode] || "";
    $("#encodeOptions").classList.toggle("is-hidden", mode !== "encode");
    const av1 = state.config.videoCodec === "av1";
    $("#av1EncoderField").hidden = !(mode === "encode" && av1);
    const crf = $("#crf");
    if (crf) {
      crf.max = av1 ? "63" : "51";
      if (Number(crf.value) > Number(crf.max)) crf.value = crf.max;
    }
    syncCrf();
    const audio = state.config.audioCodec;
    $("#audioRateField").hidden = !(audio === "aac" || audio === "opus");
    $("#audioLanguageField").hidden = state.config.audioTracks !== "preferred";
  }

  function updateDerivedUI() {
    updateModeUI();
    renderCommandPreview();
    updateLaunchSummary();
    updateStartState();
    scheduleConfigSync();
  }

  function scheduleConfigSync() {
    if (!state.config) return;
    window.clearTimeout(state.configSyncTimer);
    state.configSyncTimer = window.setTimeout(async () => {
      try {
        await api("/api/config", { method: "PUT", body: { config: state.config, save: false } });
      } catch {
        // The start request validates again and reports real errors.
      }
    }, 500);
  }

  function qualityLabel(crf) {
    const value = Number(crf);
    if (value <= 17) return "visually lossless";
    if (value <= 21) return "high quality";
    if (value <= 25) return "balanced";
    if (value <= 30) return "smaller file";
    return "low quality";
  }

  function syncCrf() {
    const slider = $("#crf");
    const output = $("#crfLabel");
    if (!slider || !output) return;
    const value = Number(slider.value);
    slider.style.setProperty("--control-value", value);
    slider.style.setProperty("--attr-min", slider.min || 0);
    slider.style.setProperty("--attr-max", slider.max || 51);
    const label = `${value} · ${qualityLabel(value)}`;
    output.textContent = label;
    slider.setAttribute("aria-valuetext", label);
  }

  function commandGroups() {
    const cfg = state.config;
    if (!cfg) return [];
    let inputExt = (cfg.extensions && cfg.extensions[0]) || "ts";
    if (cfg.mode === "rename" && inputExt === cfg.format) {
      inputExt = cfg.format === "ts" ? "mp4" : "ts";
    }
    const output = `output.${cfg.format || "mp4"}`;
    if (cfg.mode === "rename") {
      return [`move "input.${inputExt}" "${output}"`];
    }

    const groups = ["ffmpeg"];
    if (cfg.genPts) groups.push("-fflags +genpts");
    groups.push(`-i "input.${inputExt}"`);
    if (cfg.trimStart) groups.push(`-ss ${cfg.trimStart}`);
    if (cfg.audioCodec !== "none") {
      if (cfg.audioTracks === "first") {
        groups.push("-map 0:v:0 -map 0:a:0?");
      } else if (cfg.audioTracks === "preferred" && cfg.audioLanguage) {
        groups.push(`-map 0:v:0 -map 0:a:m:language:${cfg.audioLanguage}?`);
      } else {
        groups.push("-map 0:v:0 -map 0:a?");
      }
    } else {
      groups.push("-map 0:v:0");
    }

    if (cfg.mode === "encode") {
      let encoder = "libx264";
      let speed = `-preset ${cfg.preset}`;
      if (cfg.videoCodec === "h265" || cfg.videoCodec === "hevc") {
        encoder = "libx265";
      } else if (cfg.videoCodec === "av1") {
        encoder = cfg.av1Encoder === "aom" ? "libaom-av1" : cfg.av1Encoder === "rav1e" ? "librav1e" : "libsvtav1";
        const svt = { ultrafast: 13, superfast: 12, veryfast: 11, faster: 10, fast: 9, medium: 8, slow: 6, slower: 4, veryslow: 2 };
        const aom = { ultrafast: 8, superfast: 8, veryfast: 7, faster: 6, fast: 5, medium: 4, slow: 3, slower: 2, veryslow: 0 };
        const rav1e = { ultrafast: 10, superfast: 10, veryfast: 9, faster: 8, fast: 7, medium: 6, slow: 4, slower: 2, veryslow: 1 };
        if (cfg.av1Encoder === "aom") speed = `-cpu-used ${aom[cfg.preset] ?? 4}`;
        else if (cfg.av1Encoder === "rav1e") speed = `-speed ${rav1e[cfg.preset] ?? 6}`;
        else speed = `-preset ${svt[cfg.preset] ?? 8}`;
      }
      groups.push(`-c:v ${encoder} ${speed} -crf ${cfg.crf} -pix_fmt yuv420p`);
    } else {
      groups.push("-c:v copy");
    }

    if (cfg.audioCodec === "copy") {
      groups.push("-c:a copy");
    } else if (cfg.audioCodec !== "none") {
      groups.push(`-c:a ${cfg.audioCodec === "opus" ? "libopus" : "aac"} -b:a ${cfg.audioBitrate || "192k"}`);
    }

    if (cfg.subtitleCodec !== "none") {
      let subtitle = cfg.subtitleCodec;
      if (subtitle === "auto") subtitle = ["mp4", "mov"].includes(cfg.format) ? "mov_text" : "copy";
      groups.push(`-c:s ${subtitle}`);
    }

    if (cfg.aspect) groups.push(`-aspect ${cfg.aspect}`);
    if (cfg.trimEnd) {
      const start = timecodeSeconds(cfg.trimStart);
      const end = timecodeSeconds(cfg.trimEnd);
      if (Number.isFinite(start) && Number.isFinite(end) && end > start) {
        groups.push(`-t ${(end - start).toFixed(3)}`);
      }
    }
    if (cfg.faststart && ["mp4", "mov"].includes(cfg.format)) groups.push("-movflags +faststart");
    groups.push(`"${output}"`);
    return groups;
  }

  function commandTemplate() {
    return commandGroups().join(" ");
  }

  function renderCommandPreview() {
    const target = $("#commandPreview");
    if (!target || !state.config) return;
    const groups = commandGroups();
    target.replaceChildren();
    groups.forEach((line, lineIndex) => {
      if (lineIndex > 0) target.append(document.createTextNode("  "));
      const tokens = line.split(" ");
      tokens.forEach((token, tokenIndex) => {
        const span = document.createElement("span");
        if (lineIndex === 0 && tokenIndex === 0) span.className = "cmd-bin";
        else if (token.startsWith("-")) span.className = "cmd-flag";
        else if (token.startsWith('"')) span.className = "cmd-quoted";
        else span.className = "cmd-value";
        span.textContent = token;
        target.append(span);
        if (tokenIndex < tokens.length - 1) target.append(document.createTextNode(" "));
      });
      if (lineIndex < groups.length - 1) target.append(document.createTextNode("\n"));
    });
  }

  async function copyCommand() {
    const command = commandTemplate();
    try {
      await navigator.clipboard.writeText(command);
      toast("ok", "Command copied", "Paste it in a terminal to run it once.");
    } catch {
      const area = document.createElement("textarea");
      area.value = command;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.append(area);
      area.select();
      try {
        document.execCommand("copy");
        toast("ok", "Command copied");
      } catch {
        toast("warn", "Clipboard unavailable", command);
      }
      area.remove();
    }
  }

  function applyRecipe(id) {
    const recipe = state.recipes.find((item) => item.id === id);
    if (!recipe || !state.config) {
      toast("warn", "Recipe unavailable", "Reload the dashboard and try again.");
      return;
    }
    state.config.mode = recipe.mode;
    state.config.format = recipe.format;
    state.config.overwrite = recipe.overwrite;
    state.config.faststart = recipe.faststart;
    state.config.aspect = recipe.aspect;
    state.config.genPts = recipe.genPts;
    if (recipe.videoCodec) {
      state.config.videoCodec = recipe.videoCodec;
      state.config.crf = recipe.crf;
      state.config.preset = recipe.preset;
      state.config.audioCodec = recipe.audioCodec;
    }
    writeConfigToForm();
    $$("[data-recipe]").forEach((chip) => chip.classList.toggle("is-active", chip.dataset.recipe === id));
    scheduleConfigSync();
    toast("info", recipe.name, recipe.description);
  }

  function bindRecipes() {
    $$("[data-recipe]").forEach((chip) => {
      chip.addEventListener("click", () => applyRecipe(chip.dataset.recipe));
    });
  }

  // -------------------------------------------------------------------------
  // Status
  // -------------------------------------------------------------------------
  function renderStatus(status) {
    if (!status) return;
    state.status = status;
    if (!state.configPath) state.configPath = status.configPath || "";
    $("#versionLabel").textContent = `v${String(status.version || "dev").split("+")[0]}`;
    $("#footerConfig").textContent = status.configPath || "";
    $("#footerConfig").title = status.configPath || "";

    const tool = status.tool || {};
    const dot = $("#toolDot");
    if (tool.ok) {
      dot.dataset.state = "ok";
      $("#toolTitle").textContent = "FFmpeg ready";
      const version = String(tool.version || "").replace(/^ffmpeg version\s*/i, "");
      $("#toolSub").textContent = version.split(" Copyright")[0] || tool.ffmpeg || "";
      $("#toolSub").title = tool.ffmpeg || "";
    } else {
      dot.dataset.state = "fail";
      $("#toolTitle").textContent = "FFmpeg missing";
      $("#toolSub").textContent = tool.error || "Install it from the Doctor tab";
      $("#toolSub").title = tool.error || "";
    }
    updateStartState();
  }

  // -------------------------------------------------------------------------
  // Source scanning
  // -------------------------------------------------------------------------
  function invalidateScan() {
    if (!state.scan) return;
    state.scan = null;
    state.selected.clear();
    renderScanPreview();
    updateStartState();
  }

  function setScanButtonBusy(busy) {
    const button = $("#scanBtn");
    if (busy) {
      button.dataset.html = button.innerHTML;
      button.disabled = true;
      button.textContent = "Scanning…";
    } else {
      button.disabled = false;
      if (button.dataset.html) {
        button.innerHTML = button.dataset.html;
        delete button.dataset.html;
      }
    }
  }

  async function scanFolder() {
    const dir = $("#sourcePath").value.trim();
    if (!dir) {
      toast("warn", "Choose a folder first", "Type a path or use Browse.");
      $("#sourcePath").focus();
      return;
    }
    readBinding(BINDINGS.find((binding) => binding.id === "recursive"));
    readBinding(BINDINGS.find((binding) => binding.id === "extensions"));
    setScanButtonBusy(true);
    try {
      const data = await api("/api/scan", {
        method: "POST",
        body: {
          dir,
          recursive: Boolean(state.config.recursive),
          extensions: state.config.extensions || [],
          mode: state.config.mode,
        },
      });
      state.scan = data;
      state.selected = new Set(data.files.map((file) => file.path));
      renderScanPreview();
      rememberFolder(data.root);
      updateStartState();
      if (data.count === 0) {
        toast("warn", "No matching files", `Nothing matched the configured extensions in ${data.root}`);
      } else {
        toast("ok", `${filesText(data.count)} found`, `${fmtBytes(data.totalBytes)} · ${data.recursive ? "including subfolders" : "this folder only"}`);
      }
    } catch (error) {
      toast("bad", "Scan failed", error.message);
    } finally {
      setScanButtonBusy(false);
    }
  }

  function scanSelection() {
    const files = state.scan ? state.scan.files : [];
    const selectedFiles = files.filter((file) => state.selected.has(file.path));
    const selectedBytes = selectedFiles.reduce((total, file) => total + (file.sizeBytes || 0), 0);
    return { files, selectedFiles, selectedBytes };
  }

  function refreshScanCounts() {
    if (!state.scan) return;
    const { files, selectedFiles, selectedBytes } = scanSelection();
    $("#sourcePill").textContent = `${numberFormat.format(selectedFiles.length)} / ${numberFormat.format(files.length)} ${files.length === 1 ? "file" : "files"}`;
    $("#scanCount").textContent = numberFormat.format(selectedFiles.length);
    $("#scanCountLabel").textContent = selectedFiles.length === 1 ? "file selected" : "files selected";
    $("#scanSize").textContent = fmtBytes(selectedBytes);
    updateStartState();
  }

  function renderScanPreview() {
    const summary = $("#scanSummary");
    const empty = $("#sourceEmpty");
    const wrap = $("#fileWrap");
    const list = $("#fileList");
    const pill = $("#sourcePill");
    const truncated = $("#scanTruncated");

    if (!state.scan) {
      summary.hidden = true;
      empty.hidden = false;
      wrap.hidden = true;
      truncated.hidden = true;
      pill.hidden = true;
      list.replaceChildren();
      $("#sourcePill").textContent = "0 files";
      return;
    }

    const { files, selectedFiles, selectedBytes } = scanSelection();

    summary.hidden = false;
    empty.hidden = files.length > 0;
    wrap.hidden = files.length === 0;
    pill.hidden = false;
    pill.textContent = `${numberFormat.format(selectedFiles.length)} / ${numberFormat.format(files.length)} ${files.length === 1 ? "file" : "files"}`;
    $("#scanCount").textContent = numberFormat.format(selectedFiles.length);
    $("#scanCountLabel").textContent = selectedFiles.length === 1 ? "file selected" : "files selected";
    $("#scanSize").textContent = fmtBytes(selectedBytes);
    $("#scanRootLabel").textContent = state.scan.root || "";
    $("#scanRootLabel").title = state.scan.root || "";
    truncated.hidden = !state.scan.truncated;

    if (files.length === 0) {
      empty.hidden = false;
      $("#sourceEmpty").querySelector("p strong").textContent = "No matching files.";
      $("#sourceEmpty").querySelector("p.muted").textContent = "Try enabling subfolders or adding an extension in Advanced.";
      list.replaceChildren();
      return;
    }
    $("#sourceEmpty").querySelector("p strong").textContent = "No folder scanned yet.";
    $("#sourceEmpty").querySelector("p.muted").textContent = "Choose a folder and hit Scan to preview every file before converting.";

    const fragment = document.createDocumentFragment();
    const limit = 800;
    files.slice(0, limit).forEach((file) => {
      const item = document.createElement("li");
      const label = document.createElement("label");
      label.className = "file-item";

      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.checked = state.selected.has(file.path);
      checkbox.setAttribute("aria-label", `Include ${file.name}`);
      checkbox.addEventListener("change", () => {
        if (checkbox.checked) state.selected.add(file.path);
        else state.selected.delete(file.path);
        refreshScanCounts();
      });

      const name = document.createElement("span");
      name.className = "file-name";
      name.textContent = file.name;
      name.title = file.path;

      const size = document.createElement("span");
      size.className = "file-size";
      size.textContent = fmtBytes(file.sizeBytes);

      const ext = document.createElement("span");
      ext.className = "file-ext";
      ext.textContent = String(file.ext || "").toUpperCase();

      label.append(checkbox, name, size, ext);
      item.append(label);
      fragment.append(item);
    });
    list.replaceChildren(fragment);
    if (files.length > limit) {
      const note = document.createElement("li");
      note.className = "file-item";
      note.style.color = "var(--faint)";
      note.textContent = `… and ${numberFormat.format(files.length - limit)} more (all are selected)`;
      list.append(note);
    }
  }

  function selectAllFiles(select) {
    if (!state.scan) return;
    if (select) state.selected = new Set(state.scan.files.map((file) => file.path));
    else state.selected.clear();
    renderScanPreview();
    updateStartState();
  }

  function rememberFolder(folder) {
    if (!folder) return;
    try {
      const current = JSON.parse(localStorage.getItem(RECENT_KEY) || "[]");
      const next = [folder, ...current.filter((item) => normalizePath(item) !== normalizePath(folder))].slice(0, 5);
      localStorage.setItem(RECENT_KEY, JSON.stringify(next));
      localStorage.setItem(SOURCE_KEY, folder);
    } catch { /* ignore */ }
    renderRecentFolders();
  }

  function renderRecentFolders() {
    const holder = $("#recentFolders");
    holder.replaceChildren();
    let folders = [];
    try { folders = JSON.parse(localStorage.getItem(RECENT_KEY) || "[]"); } catch { folders = []; }
    if (!Array.isArray(folders) || folders.length === 0) return;
    folders.slice(0, 5).forEach((folder) => {
      const chip = document.createElement("button");
      chip.type = "button";
      chip.className = "chip";
      chip.title = folder;
      chip.append(svgIcon("folder"));
      const text = document.createElement("span");
      text.className = "ellipsis";
      text.textContent = folder;
      chip.append(text);
      chip.addEventListener("click", () => {
        $("#sourcePath").value = folder;
        scanFolder();
      });
      holder.append(chip);
    });
  }

  function updateLaunchSummary() {
    if (!state.config) return;
    const summary = $("#runSummary");
    const estimate = $("#runEstimate");
    if (!summary || !estimate) return;

    if (state.scan) {
      const files = state.scan.files.filter((file) => state.selected.has(file.path));
      const bytes = files.reduce((total, file) => total + (file.sizeBytes || 0), 0);
      summary.textContent = `${numberFormat.format(files.length)} of ${filesText(state.scan.files.length)} · ${fmtBytes(bytes)}`;
    } else {
      const source = $("#sourcePath").value.trim();
      summary.textContent = source ? source.split(/[\\/]/).filter(Boolean).pop() || source : "Choose a folder to begin";
    }
    const workers = state.config.workers || 1;
    estimate.textContent = `${modeLabel(state.config.mode)} → ${String(state.config.format || "").toUpperCase()} · ${workers} ${workers === 1 ? "worker" : "workers"}`;
  }

  function updateStartState() {
    const button = $("#startBtn");
    const hint = $("#startHint");
    if (!button) return;
    const toolOK = !state.status || !state.status.tool || state.status.tool.ok !== false;
    const running = Boolean(state.activeRun && (state.activeRun.status === "running" || state.activeRun.status === "pending"));
    const hasSource = $("#sourcePath").value.trim().length > 0;
    let hasSelection = true;
    if (state.scan) hasSelection = state.selected.size > 0;

    button.disabled = !toolOK || running || !hasSource || !hasSelection;
    if (!toolOK) hint.textContent = "Install FFmpeg first — see the Doctor tab.";
    else if (running) hint.textContent = "A batch is running. Cancel it or wait for it to finish.";
    else if (!hasSource) hint.textContent = "Choose a source folder to begin.";
    else if (!hasSelection) hint.textContent = "Select at least one file, or scan again.";
    else hint.textContent = "Nothing runs until you start.";
    updateLaunchSummary();

    const runLink = $("#activeRunLink");
    if (runLink) {
      if (running && state.activeRun) {
        runLink.hidden = false;
        const percent = Math.round(Number(state.activeRun.percent) || 0);
        $("#activeRunText").textContent = `${modeLabel(state.activeRun.mode)} → ${String(state.activeRun.format || "").toUpperCase()} · ${percent}% · view progress`;
      } else {
        runLink.hidden = true;
      }
    }
  }

  function bindSource() {
    $("#browseSource").addEventListener("click", () => openBrowser("source"));
    $("#browseOutput").addEventListener("click", () => openBrowser("output"));
    $("#browseArchive").addEventListener("click", () => openBrowser("archive"));
    // Deleting and archiving are mutually exclusive; clear the other choice.
    $("#deleteSource").addEventListener("change", (event) => {
      if (!event.target.checked) return;
      const archive = $("#archiveDir");
      if (!archive.value) return;
      archive.value = "";
      readBinding(BINDINGS.find((binding) => binding.id === "archiveDir"));
      scheduleConfigSync();
    });
    $("#archiveDir").addEventListener("change", (event) => {
      if (!event.target.value.trim()) return;
      $("#deleteSource").checked = false;
      readBinding(BINDINGS.find((binding) => binding.id === "deleteSource"));
      scheduleConfigSync();
    });
    $("#scanBtn").addEventListener("click", scanFolder);
    $("#selectAll").addEventListener("click", () => selectAllFiles(true));
    $("#selectNone").addEventListener("click", () => selectAllFiles(false));

    $("#sourcePath").addEventListener("input", () => {
      if (state.scan && normalizePath($("#sourcePath").value) !== normalizePath(state.scan.root)) {
        state.scan = null;
        state.selected.clear();
        renderScanPreview();
      }
      updateStartState();
    });
    $("#sourcePath").addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        scanFolder();
      }
    });

    $("#recursive").addEventListener("change", () => {
      readBinding(BINDINGS.find((binding) => binding.id === "recursive"));
      invalidateScan();
    });

    $("#startBtn").addEventListener("click", startBatch);
    $("#newBatchBtn").addEventListener("click", () => switchView("convert"));
  }

  // -------------------------------------------------------------------------
  // Start a batch
  // -------------------------------------------------------------------------
  async function startBatch() {
    if (!state.config) return;
    if (state.activeRun && state.activeRun.status === "running") {
      toast("warn", "A batch is already running", "Open Runs to follow it or cancel it.");
      switchView("runs");
      return;
    }
    const dir = $("#sourcePath").value.trim();
    if (!dir) {
      toast("warn", "Choose a source folder", "Type a path or browse for one.");
      return;
    }
    let files = [];
    if (state.scan && state.selected.size > 0 && state.selected.size < state.scan.files.length) {
      files = [...state.selected];
    }

    const button = $("#startBtn");
    button.disabled = true;
    button.dataset.html = button.innerHTML;
    button.textContent = "Starting…";

    try {
      const snapshot = await api("/api/runs", {
        method: "POST",
        body: {
          dir: files.length > 0 ? "" : dir,
          files,
          config: state.config,
          saveConfig: $("#saveConfig").checked,
        },
      });
      rememberFolder(dir);
      state.currentRunID = snapshot.id;
      state.runDetail = snapshot;
      state.renderedRunID = null;
      state.activeRun = snapshot.status === "running" ? snapshot : null;
      upsertRunSummary(snapshot);
      renderRunList();
      selectRun(snapshot.id);
      switchView("runs");
      toast("ok", "Batch started", `${filesText(snapshot.total)} · ${modeLabel(snapshot.mode)} → ${String(snapshot.format).toUpperCase()}`);
      // Reconcile with the server after a beat: a tiny batch may finish before
      // the POST response is processed, and SSE events are authoritative.
      window.setTimeout(() => refreshRuns({ select: false }), 800);
    } catch (error) {
      toast("bad", "Could not start", error.message);
    } finally {
      button.disabled = false;
      if (button.dataset.html) {
        button.innerHTML = button.dataset.html;
        delete button.dataset.html;
      }
      updateStartState();
    }
  }

  // -------------------------------------------------------------------------
  // Runs: list, detail, live events
  // -------------------------------------------------------------------------
  function upsertRunSummary(summary) {
    if (!summary || !summary.id) return;
    const index = state.runs.findIndex((run) => run.id === summary.id);
    if (index >= 0) state.runs[index] = { ...state.runs[index], ...summary };
    else state.runs.unshift({ ...summary });
    state.runs.sort((a, b) => new Date(b.startedAt || 0) - new Date(a.startedAt || 0));
    updateNavBadge("runs", state.runs.length);
  }

  function updateNavBadge(view, count) {
    const badge = view === "runs" ? $("#navRunCount") : $("#navWatchCount");
    badge.textContent = String(count);
    badge.hidden = count === 0;
    const button = document.querySelector(`.nav-item[data-view="${view}"]`);
    if (button) {
      const label = view === "runs" ? "Runs" : "Watch";
      button.setAttribute("aria-label", count > 0 ? `${label} ${count}` : label);
    }
  }

  async function refreshRuns({ select = true } = {}) {
    try {
      const data = await api("/api/runs");
      state.runs = data.runs || [];
      let active = data.active || null;
      // A response that was generated before a run finished must not resurrect it.
      if (active && active.id === state.lastFinishedRunID) active = null;
      state.activeRun = active;
      updateNavBadge("runs", state.runs.length);
      $("#runsEmpty").hidden = state.runs.length > 0;
      renderRunList();
      if (select && !state.currentRunID && state.runs.length > 0) {
        selectRun(state.runs[0].id);
      }
      updateStartState();
    } catch (error) {
      // A transient failure should not disrupt the page.
      console.warn("refreshRuns", error);
    }
  }

  function renderRunList() {
    const list = $("#runList");
    list.replaceChildren();
    state.runs.forEach((run) => list.append(runListItem(run)));
    $("#runsEmpty").hidden = state.runs.length > 0;
  }

  function runListItem(run) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "run-item";
    button.dataset.status = run.status;
    button.dataset.runId = run.id;
    if (run.id === state.currentRunID) button.classList.add("is-active");

    const icon = document.createElement("span");
    icon.className = "run-item-icon";
    icon.append(svgIcon(run.status === "done" ? "check" : run.status === "failed" || run.status === "partial" ? "alert" : run.status === "canceled" ? "x" : "film"));

    const text = document.createElement("span");
    text.className = "run-item-text";
    const title = document.createElement("span");
    title.className = "run-item-title";
    title.textContent = `${modeLabel(run.mode)} → ${String(run.format || "").toUpperCase()}`;
    const meta = document.createElement("span");
    meta.className = "run-item-meta";
    meta.textContent = `${filesText(run.total || 0)} · ${fmtRelative(run.startedAt)}`;
    text.append(title, meta);

    const percent = document.createElement("span");
    percent.className = "run-item-percent";
    percent.textContent = run.status === "running" ? `${Math.round(run.percent || 0)}%` : statusLabel(run.status);

    button.append(icon, text, percent);
    button.addEventListener("click", () => {
      state.currentRunID = run.id;
      selectRun(run.id);
    });
    return button;
  }

  function updateRunItem(summary) {
    if (!summary || !summary.id) return;
    const item = Array.from(document.querySelectorAll(".run-item")).find((node) => node.dataset.runId === summary.id);
    if (!item) {
      renderRunList();
      return;
    }
    item.dataset.status = summary.status;
    const percent = item.querySelector(".run-item-percent");
    if (percent) {
      percent.textContent = summary.status === "running" ? `${Math.round(summary.percent || 0)}%` : statusLabel(summary.status);
    }
  }

  async function selectRun(id) {
    state.currentRunID = id;
    renderRunList();
    try {
      const snapshot = await api(`/api/runs/${id}`);
      state.runDetail = snapshot;
      renderRunDetail(snapshot);
      upsertRunSummary(snapshot);
      renderRunList();
      updateStartState();
    } catch (error) {
      toast("bad", "Could not load run", error.message);
    }
  }

  function renderRunDetail(snapshot) {
    if (!snapshot) return;
    state.renderedRunID = snapshot.id;
    $("#runPlaceholder").hidden = true;
    $("#runDetail").hidden = false;
    $(".run-detail-card").dataset.status = snapshot.status;

    $("#runEyebrow").textContent = `${snapshot.id} · ${statusLabel(snapshot.status)}`;
    $("#runTitle").textContent = `${modeLabel(snapshot.mode)} → ${String(snapshot.format || "").toUpperCase()}`;
    const root = snapshot.root || (snapshot.config && snapshot.config.outputDir) || "";
    $("#runMeta").textContent = `${filesText(snapshot.total || 0)} · ${fmtBytes(snapshot.bytesIn || 0)} · ${root}`;
    $("#runMeta").title = root;

    renderSummaryUI(snapshot);
    renderJobList(snapshot);
    renderLog(snapshot.log || []);

    const active = snapshot.status === "running" || snapshot.status === "pending";
    $("#cancelRun").hidden = !active;
    $("#cancelRun").disabled = Boolean(snapshot.canceling);
    $("#reportJson").disabled = false;
    $("#reportCsv").disabled = false;
  }

  function renderSummaryUI(snapshot) {
    const percent = Number(snapshot.percent) || 0;
    $("#overallProgress").value = Math.min(100, percent);
    $("#overallPercent").textContent = `${percent >= 100 ? 100 : percent.toFixed(percent < 10 ? 1 : 0)}%`;
    $("#statDone").textContent = numberFormat.format(snapshot.done || 0);
    $("#statSkipped").textContent = numberFormat.format(snapshot.skipped || 0);
    $("#statFailed").textContent = numberFormat.format(snapshot.failed || 0);
    $("#statData").textContent = snapshot.bytesOut ? fmtBytes(snapshot.bytesOut) : "—";

    const mediaSeconds = (snapshot.jobs || []).reduce((total, job) => {
      if (job.status === "done" && job.durationMs) return total + job.durationMs / 1000;
      return total;
    }, 0);
    const wallSeconds = elapsedOf(snapshot) / 1000;
    $("#statSpeed").textContent = wallSeconds > 0.5 && mediaSeconds > 0 ? `${(mediaSeconds / wallSeconds).toFixed(1)}×` : "—";
    $("#statElapsed").textContent = snapshot.startedAt ? fmtDurationMs(elapsedOf(snapshot)) : "—";

    const active = snapshot.status === "running" || snapshot.status === "pending";
    $("#cancelRun").hidden = !active;
    $("#cancelRun").disabled = Boolean(snapshot.canceling);
    const retry = $("#retryRun");
    const canRetry = snapshot.failed > 0 && !active;
    retry.hidden = !canRetry;
    if (canRetry) {
      $("#retryLabel").textContent = `Retry ${snapshot.failed} failed`;
    }
  }

  function updateElapsed() {
    const snapshot = state.runDetail;
    if (!snapshot) return;
    $("#statElapsed").textContent = fmtDurationMs(elapsedOf(snapshot));
  }

  function jobMeta(job) {
    const parts = [];
    if (job.sizeBytes) parts.push(fmtBytes(job.sizeBytes));
    if (job.durationMs) parts.push(fmtClock(job.durationMs));
    if (job.resolution) parts.push(job.resolution);
    if (job.videoCodec) parts.push(job.audioCodec ? `${job.videoCodec} / ${job.audioCodec}` : job.videoCodec);
    if (job.verified) parts.push("verified");
    if (job.sourceArchivedTo) parts.push("source archived");
    if (job.warnings && job.warnings.length) parts.push(`${job.warnings.length} warning${job.warnings.length === 1 ? "" : "s"}`);
    return parts.join(" · ");
  }

  function jobIcon(job) {
    if (job.status === "running") return spinner();
    const map = {
      done: "check",
      failed: "alert",
      skipped: "right",
      canceled: "x",
      planned: "spark",
      pending: "clock",
    };
    return svgIcon(map[job.status] || "clock");
  }

  function jobStage(job) {
    if (job.status === "running") {
      const stages = {
        starting: "starting",
        probing: "probing",
        "hashing source": "hashing source",
        converting: "converting",
        verifying: "verifying",
        "hashing output": "hashing output",
        finalizing: "finalising",
        "archiving source": "archiving source",
        renaming: "renaming",
      };
      return stages[job.stage] || "working";
    }
    if (job.status === "skipped") return job.reason || "skipped";
    if (job.status === "done") return job.sourceRemoved ? "done · source deleted" : "done";
    return statusLabel(job.status);
  }

  function createJobRow(job) {
    const item = document.createElement("li");
    item.className = "job-item";
    item.dataset.jobId = job.id;
    item.dataset.status = job.status;

    const icon = document.createElement("span");
    icon.className = "job-icon";
    icon.append(jobIcon(job));

    const main = document.createElement("div");
    main.className = "job-main";
    const name = document.createElement("span");
    name.className = "job-name";
    name.textContent = job.name;
    name.title = job.input || job.name;
    const meta = document.createElement("span");
    meta.className = "job-meta";
    meta.textContent = jobMeta(job);
    const error = document.createElement("span");
    error.className = "job-error";
    error.hidden = !job.error;
    error.textContent = job.error || "";
    main.append(name, meta, error);

    const progressWrap = document.createElement("div");
    progressWrap.className = "job-progress-wrap";
    const top = document.createElement("div");
    top.className = "job-progress-top";
    const stage = document.createElement("span");
    stage.className = "stage";
    const pct = document.createElement("span");
    pct.className = "pct";
    top.append(stage, pct);
    const bar = document.createElement("progress");
    bar.className = "tiny";
    bar.max = 100;
    bar.value = 0;
    bar.setAttribute("aria-label", `Progress for ${job.name}`);
    progressWrap.append(top, bar);

    const speed = document.createElement("div");
    speed.className = "job-speed";
    const speedMain = document.createElement("strong");
    const speedSub = document.createElement("span");
    speed.append(speedMain, speedSub);

    item.append(icon, main, progressWrap, speed);
    const refs = { li: item, icon, name, meta, error, stage, pct, bar, speedMain, speedSub };
    updateJobRow(job, refs);
    return refs;
  }

  function updateJobRow(job, refs = state.jobNodes.get(job.id)) {
    if (!refs || !job) return;
    refs.li.dataset.status = job.status;
    refs.icon.replaceChildren(jobIcon(job));
    refs.name.textContent = job.name;
    refs.name.title = job.input || job.name;
    const meta = jobMeta(job);
    if (refs.meta.textContent !== meta) refs.meta.textContent = meta;
    refs.error.hidden = !job.error || job.status === "canceled";
    refs.error.textContent = job.error && job.status !== "canceled" ? job.error : "";

    refs.stage.textContent = jobStage(job);
    const percent = Math.max(0, Math.min(100, Number(job.progress) || 0));
    const running = job.status === "running";
    if (running) {
      refs.pct.textContent = `${percent.toFixed(1)}%`;
      if (!job.durationMs) refs.bar.removeAttribute("value");
      else refs.bar.value = percent;
    } else {
      refs.pct.textContent = job.status === "done" || job.status === "planned" ? "100%" : "";
      refs.bar.value = job.status === "done" || job.status === "planned" ? 100 : 0;
      if (job.status === "failed" || job.status === "canceled") {
        refs.pct.textContent = `${percent.toFixed(1)}%`;
        refs.bar.value = percent;
      }
    }
    refs.bar.setAttribute("aria-label", `${job.name}: ${refs.pct.textContent || statusLabel(job.status)}`);

    if (job.speed > 0.01) refs.speedMain.textContent = `${job.speed.toFixed(1)}×`;
    else if (job.outputBytes) refs.speedMain.textContent = fmtBytes(job.outputBytes);
    else refs.speedMain.textContent = "—";
    refs.speedSub.textContent = job.etaSeconds > 0 ? `ETA ${fmtSeconds(job.etaSeconds)}` : job.status === "done" ? "written" : "";

    applyFilterToRow(refs.li, job);
  }

  function renderJobList(snapshot) {
    const list = $("#jobList");
    list.replaceChildren();
    state.jobNodes.clear();
    const jobs = snapshot.jobs || [];
    const limit = 800;
    jobs.slice(0, limit).forEach((job) => {
      const refs = createJobRow(job);
      state.jobNodes.set(job.id, refs);
      list.append(refs.li);
    });
    if (jobs.length > limit) {
      const note = document.createElement("li");
      note.className = "job-empty";
      note.textContent = `Showing the first ${numberFormat.format(limit)} of ${numberFormat.format(jobs.length)} jobs. Export the CSV for the full list.`;
      list.append(note);
    }
    if (jobs.length === 0) {
      const note = document.createElement("li");
      note.className = "job-empty";
      note.textContent = "No jobs in this run.";
      list.append(note);
    }
    applyFilter();
    updateJobCountLabel(jobs);
  }

  function updateJobCountLabel(jobs) {
    const shown = jobs.filter((job) => filterMatches(job)).length;
    const total = jobs.length;
    const label = $("#jobCountLabel");
    if (state.filter === "all") label.textContent = `${numberFormat.format(total)} job${total === 1 ? "" : "s"}`;
    else label.textContent = `${numberFormat.format(shown)} of ${numberFormat.format(total)} jobs`;
  }

  function filterMatches(job) {
    switch (state.filter) {
      case "all": return true;
      case "running": return job.status === "running" || job.status === "pending";
      case "done": return job.status === "done" || job.status === "planned";
      case "skipped": return job.status === "skipped";
      case "failed": return job.status === "failed" || job.status === "canceled";
      default: return true;
    }
  }

  function applyFilterToRow(row, job) {
    row.hidden = !filterMatches(job);
  }

  function applyFilter() {
    const jobs = (state.runDetail && state.runDetail.jobs) || [];
    jobs.forEach((job) => {
      const refs = state.jobNodes.get(job.id);
      if (refs) applyFilterToRow(refs.li, job);
    });
    updateJobCountLabel(jobs);
  }

  function renderLog(entries) {
    const list = $("#logList");
    list.replaceChildren();
    state.renderedLogCount = 0;
    const slice = entries.slice(-300);
    slice.forEach((entry) => appendLogEntry(entry, false));
    state.renderedLogCount = slice.length;
    $("#logCount").textContent = String(entries.length);
  }

  function appendLogEntry(entry, scroll = true) {
    if (!entry) return;
    const list = $("#logList");
    const item = document.createElement("li");
    const time = document.createElement("time");
    time.textContent = fmtTime(entry.at);
    const level = document.createElement("span");
    level.className = `lvl lvl-${entry.level || "info"}`;
    level.textContent = entry.level || "info";
    const message = document.createElement("span");
    message.className = "msg";
    message.textContent = entry.message || "";
    item.append(time, level, message);
    const nearBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 72;
    list.append(item);
    while (list.children.length > 300) list.firstElementChild.remove();
    state.renderedLogCount += 1;
    $("#logCount").textContent = String(state.renderedLogCount);
    if (scroll && nearBottom) list.scrollTop = list.scrollHeight;
  }

  function mergeSummaryIntoDetail(summary) {
    if (!state.runDetail || !summary || summary.id !== state.runDetail.id) return;
    Object.assign(state.runDetail, summary);
    renderSummaryUI(state.runDetail);
    $("#runEyebrow").textContent = `${summary.id} · ${statusLabel(summary.status)}`;
    if (summary.status === "running") {
      state.activeRun = summary;
    } else if (summary.status !== "pending") {
      if (state.activeRun && state.activeRun.id === summary.id) state.activeRun = null;
    }
    updateStartState();
  }

  function handleSSE(type, event) {
    if (!event) return;

    if (type === "snapshot") {
      const snapshot = event;
      upsertRunSummary(snapshot);
      renderRunList();
      if (!state.currentRunID) {
        state.currentRunID = snapshot.id;
      }
      if (state.currentRunID === snapshot.id) {
        state.runDetail = snapshot;
        renderRunDetail(snapshot);
      }
      if (snapshot.status === "running" || snapshot.status === "pending") state.activeRun = snapshot;
      else if (state.activeRun && state.activeRun.id === snapshot.id) state.activeRun = null;
      updateStartState();
      return;
    }

    if (event.run) {
      upsertRunSummary(event.run);
      if (type.startsWith("run.")) renderRunList();
      else updateRunItem(event.run);
      if (state.activeRun && event.run.id === state.activeRun.id && type !== "log") {
        state.activeRun = { ...state.activeRun, ...event.run };
        updateStartState();
      }
    }

    if (type === "log") {
      if (state.runDetail && event.run && event.run.id === state.runDetail.id) {
        appendLogEntry({ at: event.at, level: event.level, message: event.message });
      }
      return;
    }

    if (type === "run.started") {
      if (event.run) {
        if (state.lastFinishedRunID === event.run.id) state.lastFinishedRunID = "";
        state.activeRun = event.run;
        if (!state.currentRunID) state.currentRunID = event.run.id;
        if (state.currentRunID === event.run.id) selectRun(event.run.id);
      }
      if (state.view === "watch") refreshWatch({ silent: true });
      updateStartState();
      return;
    }

    if (type === "run.canceling") {
      if (state.runDetail && event.run && state.runDetail.id === event.run.id) {
        state.runDetail.canceling = true;
        renderSummaryUI(state.runDetail);
        $("#cancelRun").textContent = "Cancelling…";
      }
      return;
    }

    if (type === "run.finished") {
      if (event.run) {
        state.lastFinishedRunID = event.run.id;
        state.lastFinishedAt = Date.now();
      }
      if (state.activeRun && event.run && state.activeRun.id === event.run.id) state.activeRun = null;
      if (event.run && event.run.id === state.currentRunID) {
        finalizeRun(event.run.id, event);
      }
      if (state.view === "watch") refreshWatch({ silent: true });
      updateStartState();
      return;
    }

    if (!state.runDetail || !event.run || event.run.id !== state.runDetail.id) return;
    mergeSummaryIntoDetail(event.run);

    if (type === "job.started" && event.job) {
      ensureJobExists(event.job);
    }
    if (event.job) {
      const jobs = state.runDetail.jobs || [];
      const index = jobs.findIndex((job) => job.id === event.job.id);
      if (index >= 0) jobs[index] = { ...jobs[index], ...event.job };
      else jobs.push(event.job);
      if (state.renderedRunID !== state.runDetail.id) {
        renderJobList(state.runDetail);
        state.renderedRunID = state.runDetail.id;
      }
      updateJobRow(event.job);
    }
    if (type === "run.finished") {
      finalizeRun(event.run.id, event);
    }
  }

  function ensureJobExists(job) {
    if (!state.runDetail) return;
    const jobs = state.runDetail.jobs || [];
    if (!jobs.some((item) => item.id === job.id)) jobs.push(job);
  }

  async function finalizeRun(id, event) {
    try {
      const snapshot = await api(`/api/runs/${id}`);
      state.runDetail = snapshot;
      renderRunDetail(snapshot);
      upsertRunSummary(snapshot);
      renderRunList();
      state.renderedRunID = snapshot.id;
      updateStartState();
      const canceled = snapshot.status === "canceled" || snapshot.canceled > 0;
      const tone = snapshot.failed > 0 ? "bad" : canceled ? "warn" : "ok";
      const parts = [`${numberFormat.format(snapshot.done)} converted`];
      if (snapshot.failed) parts.push(`${numberFormat.format(snapshot.failed)} failed`);
      if (snapshot.skipped) parts.push(`${numberFormat.format(snapshot.skipped)} skipped`);
      if (canceled) parts.push(`${numberFormat.format(snapshot.canceled)} cancelled`);
      toast(tone, canceled ? "Batch cancelled" : "Batch finished", parts.join(" · "));
    } catch (error) {
      if (event && event.jobs) {
        state.runDetail = { ...(state.runDetail || {}), ...event.run, jobs: event.jobs };
        renderRunDetail(state.runDetail);
      }
      console.warn("finalizeRun", error);
    }
  }

  function connectEvents() {
    if (state.eventSource) state.eventSource.close();
    const source = new EventSource("/api/events");
    state.eventSource = source;

    source.onopen = () => setLive(true);
    source.onerror = () => setLive(false);

    source.addEventListener("snapshot", (message) => {
      try { handleSSE("snapshot", JSON.parse(message.data)); } catch (error) { console.warn(error); }
    });

    const types = [
      "run.started", "run.canceling", "run.finished",
      "job.started", "job.updated", "job.progress", "job.done",
      "job.skipped", "job.failed", "job.canceled", "job.planned", "log",
    ];
    types.forEach((type) => {
      source.addEventListener(type, (message) => {
        try { handleSSE(type, JSON.parse(message.data)); } catch (error) { console.warn(type, error); }
      });
    });
  }

  function setLive(live) {
    state.live = live;
    const pill = $("#livePill");
    pill.dataset.state = live ? "live" : "offline";
    $("#liveText").textContent = live ? "Live" : "Offline";
    if (!live) {
      // EventSource retries on its own; make the state visible.
      window.setTimeout(() => {
        if (!state.live) $("#liveText").textContent = "Reconnecting…";
      }, 1200);
    }
  }

  function bindRuns() {
    $("#refreshRuns").addEventListener("click", () => refreshRuns());
    $("#cancelRun").addEventListener("click", cancelCurrentRun);
    $("#retryRun").addEventListener("click", retryFailedRun);
    $("#reportJson").addEventListener("click", () => downloadReport("json"));
    $("#reportCsv").addEventListener("click", () => downloadReport("csv"));
    $("#revealRun").addEventListener("click", revealCurrentRun);
    $$(".filter").forEach((button) => {
      button.addEventListener("click", () => {
        state.filter = button.dataset.filter;
        $$(".filter").forEach((item) => item.classList.toggle("is-active", item === button));
        applyFilter();
      });
    });
    window.setInterval(updateElapsed, 1000);
  }

  async function cancelCurrentRun() {
    const snapshot = state.runDetail;
    if (!snapshot) return;
    const ok = await askConfirm({
      title: "Cancel this batch?",
      body: "FFmpeg will be stopped after the current packet, partial outputs are removed, and finished files are kept.",
      okLabel: "Cancel batch",
      cancelLabel: "Keep running",
      danger: true,
    });
    if (!ok) return;
    try {
      await api(`/api/runs/${snapshot.id}/cancel`, { method: "POST" });
      toast("warn", "Cancelling", "Waiting for FFmpeg to stop…");
      $("#cancelRun").disabled = true;
    } catch (error) {
      toast("bad", "Could not cancel", error.message);
    }
  }

  function downloadReport(format) {
    if (!state.runDetail) return;
    const anchor = document.createElement("a");
    anchor.href = `/api/runs/${state.runDetail.id}/report?format=${format}`;
    anchor.download = "";
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
  }

  async function revealCurrentRun() {
    const snapshot = state.runDetail;
    if (!snapshot) return;
    let path = snapshot.config && snapshot.config.outputDir ? snapshot.config.outputDir : "";
    if (!path || !isAbsolutePath(path)) path = snapshot.root;
    if (!path) {
      toast("warn", "No folder to open");
      return;
    }
    try {
      await api("/api/reveal", { method: "POST", body: { path } });
    } catch (error) {
      toast("bad", "Could not open folder", error.message);
    }
  }

  async function retryFailedRun() {
    const snapshot = state.runDetail;
    if (!snapshot || !snapshot.failed) return;
    const button = $("#retryRun");
    button.disabled = true;
    try {
      const next = await api(`/api/runs/${snapshot.id}/retry`, { method: "POST" });
      state.currentRunID = next.id;
      state.runDetail = next;
      state.renderedRunID = null;
      state.activeRun = next.status === "running" ? next : null;
      upsertRunSummary(next);
      renderRunList();
      selectRun(next.id);
      toast("ok", "Retrying failed jobs", filesText(next.total));
    } catch (error) {
      toast("bad", "Could not retry", error.message);
    } finally {
      button.disabled = false;
    }
  }

  // -------------------------------------------------------------------------
  // Watch folders
  // -------------------------------------------------------------------------
  function startWatchPolling() {
    if (state.watchTimer) return;
    state.watchTimer = window.setInterval(() => {
      if (state.view === "watch") refreshWatch({ silent: true });
    }, 3000);
  }

  function stopWatchPolling() {
    if (!state.watchTimer) return;
    window.clearInterval(state.watchTimer);
    state.watchTimer = 0;
  }

  async function refreshWatch({ silent = false } = {}) {
    try {
      const data = await api("/api/watch");
      state.watch = data;
      renderWatch(data);
    } catch (error) {
      if (!silent) toast("bad", "Watcher unavailable", error.message);
    }
  }

  function renderWatch(data) {
    const targets = data.targets || [];
    const list = $("#watchList");
    list.replaceChildren();
    targets.forEach((target) => list.append(watchRow(target)));
    $("#watchEmpty").hidden = targets.length > 0;

    const pill = $("#watchStatusPill");
    const enabled = targets.filter((target) => target.enabled).length;
    if (data.running) {
      pill.dataset.tone = "info";
      pill.textContent = "Converting";
    } else if (enabled > 0) {
      pill.dataset.tone = "ok";
      pill.textContent = `Watching ${enabled}`;
    } else if (targets.length > 0) {
      pill.dataset.tone = "warn";
      pill.textContent = "Paused";
    } else {
      pill.removeAttribute("data-tone");
      pill.textContent = "Idle";
    }

    renderWatchEvents(data.events || []);
    $("#watchTiming").textContent = `Scans every ${data.intervalSeconds || 5}s · waits ${data.settleSeconds || 0}s for files to settle.`;
    updateNavBadge("watch", targets.length);
  }

  function watchRow(target) {
    const item = document.createElement("li");
    item.className = "watch-item";
    item.dataset.id = target.id;
    if (!target.enabled) item.dataset.enabled = "false";

    const icon = document.createElement("span");
    icon.className = "watch-icon";
    icon.append(svgIcon("folder"));

    const body = document.createElement("div");
    body.className = "watch-text";
    const path = document.createElement("span");
    path.className = "watch-path";
    path.textContent = target.dir;
    path.title = target.dir;
    const meta = document.createElement("span");
    meta.className = "watch-meta";
    const parts = [
      target.recursive ? "recursive" : "top level",
      `${numberFormat.format(target.tracked || 0)} tracked`,
    ];
    if (target.pending) parts.push(`${numberFormat.format(target.pending)} pending`);
    if (target.lastScanAt) parts.push(`scanned ${fmtRelative(target.lastScanAt)}`);
    meta.textContent = parts.join(" · ");
    body.append(path, meta);
    if (target.lastError) {
      const error = document.createElement("span");
      error.className = "watch-error";
      error.textContent = target.lastError;
      body.append(error);
    }

    const toggleLabel = document.createElement("label");
    toggleLabel.className = "switch watch-toggle";
    const toggle = document.createElement("input");
    toggle.type = "checkbox";
    toggle.checked = Boolean(target.enabled);
    toggle.setAttribute("aria-label", `Watch ${target.dir}`);
    toggle.addEventListener("change", async () => {
      const desired = toggle.checked;
      toggle.disabled = true;
      try {
        await api(`/api/watch/${encodeURIComponent(target.id)}`, {
          method: "PATCH",
          body: { enabled: desired },
        });
        toast(desired ? "ok" : "warn", desired ? "Watching resumed" : "Watching paused", target.dir);
      } catch (error) {
        toggle.checked = !desired;
        toast("bad", "Could not update watcher", error.message);
      } finally {
        toggle.disabled = false;
        refreshWatch({ silent: true });
      }
    });
    const track = document.createElement("span");
    track.className = "switch-track";
    track.setAttribute("aria-hidden", "true");
    toggleLabel.append(toggle, track);

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "icon-btn watch-remove";
    remove.setAttribute("aria-label", `Stop watching ${target.dir}`);
    remove.title = "Stop watching";
    remove.append(svgIcon("x"));
    remove.addEventListener("click", async () => {
      const ok = await askConfirm({
        title: "Stop watching this folder?",
        body: `${target.dir} will no longer be scanned. Existing files, outputs and runs are left untouched.`,
        okLabel: "Stop watching",
        cancelLabel: "Keep watching",
        danger: true,
      });
      if (!ok) return;
      try {
        await api(`/api/watch/${encodeURIComponent(target.id)}`, { method: "DELETE" });
        toast("warn", "Stopped watching", target.dir);
        refreshWatch({ silent: true });
      } catch (error) {
        toast("bad", "Could not stop watcher", error.message);
      }
    });

    item.append(icon, body, toggleLabel, remove);
    return item;
  }

  function renderWatchEvents(events) {
    const list = $("#watchEvents");
    list.replaceChildren();
    const slice = events.slice(0, 120);
    slice.forEach((event) => {
      const item = document.createElement("li");
      item.dataset.level = event.level || "info";
      const time = document.createElement("time");
      time.textContent = fmtTime(event.at);
      const level = document.createElement("span");
      level.className = "watch-level";
      level.textContent = event.level || "info";
      const message = document.createElement("span");
      message.className = "watch-message";
      message.textContent = event.message || "";
      item.append(time, level, message);
      list.append(item);
    });
    $("#watchEventsEmpty").hidden = slice.length > 0;
  }

  async function addWatchTarget(dir, recursive, convertExisting) {
    const target = await api("/api/watch", {
      method: "POST",
      body: { dir, recursive, convertExisting, config: state.config },
    });
    toast("ok", "Watching folder", target.dir);
    await refreshWatch({ silent: true });
    switchView("watch");
    return target;
  }

  function bindWatch() {
    $("#addWatchBtn").addEventListener("click", () => openBrowser("watch"));
    $("#watchEmptyAdd").addEventListener("click", () => openBrowser("watch"));
    $("#watchSettingsBtn").addEventListener("click", () => switchView("convert"));
  }

  // -------------------------------------------------------------------------
  // Confirm dialog
  // -------------------------------------------------------------------------
  let confirmResolver = null;

  function askConfirm({ title, body, okLabel = "Confirm", cancelLabel = "Cancel", danger = true }) {
    $("#confirmTitle").textContent = title;
    $("#confirmBody").textContent = body;
    const ok = $("#confirmOk");
    ok.textContent = okLabel;
    ok.className = danger ? "btn btn-danger" : "btn btn-primary";
    $("#confirmCancel").textContent = cancelLabel;
    $("#confirmDialog").showModal();
    return new Promise((resolve) => { confirmResolver = resolve; });
  }

  function closeConfirm(value) {
    $("#confirmDialog").close();
    if (confirmResolver) {
      confirmResolver(value);
      confirmResolver = null;
    }
  }

  function bindConfirm() {
    $("#confirmOk").addEventListener("click", () => closeConfirm(true));
    $("#confirmCancel").addEventListener("click", () => closeConfirm(false));
    $("#confirmDialog").addEventListener("cancel", (event) => {
      event.preventDefault();
      closeConfirm(false);
    });
  }

  // -------------------------------------------------------------------------
  // Folder browser
  // -------------------------------------------------------------------------
  function openBrowser(mode) {
    state.browser.mode = mode;
    const isWatch = mode === "watch";
    $("#browserTitle").textContent = mode === "source"
      ? "Choose a source folder"
      : isWatch
        ? "Choose a folder to watch"
        : mode === "archive"
          ? "Choose an archive folder"
          : "Choose an output folder";
    $("#browserSub").textContent = mode === "source"
      ? "FFBatch scans this folder for the configured extensions."
      : isWatch
        ? "New files will be converted using the current Convert settings."
        : mode === "archive"
          ? "Verified originals will be moved here after conversion."
          : "Converted files will be written here.";
    $("#watchDialogOptions").hidden = !isWatch;
    if (isWatch) {
      $("#watchRecursive").checked = Boolean(state.config && state.config.recursive);
      $("#watchExisting").checked = false;
    }
    const input = mode === "source"
      ? $("#sourcePath")
      : isWatch
        ? null
        : mode === "archive"
          ? $("#archiveDir")
          : $("#outputDir");
    const start = input ? input.value.trim() : $("#sourcePath").value.trim();
    $("#browserPath").value = start || state.browser.path || "";
    $("#browserDialog").showModal();
    loadDirectory(start || state.browser.path || "");
  }

  async function loadDirectory(path) {
    const list = $("#browserList");
    list.innerHTML = '<div class="browser-loading">Loading folders…</div>';
    try {
      const data = await api(`/api/fs?path=${encodeURIComponent(path || "")}`);
      state.browser.path = data.path;
      $("#browserPath").value = data.path;
      const folders = data.entries.length;
      $("#browserStatus").textContent = `${numberFormat.format(folders)} ${folders === 1 ? "folder" : "folders"}`;
      $("#browserUp").disabled = !data.parent;
      $("#browserUp").dataset.parent = data.parent || "";
      renderPlaces([...(data.roots || []), ...(data.places || [])], data.path);
      renderBrowserEntries(data.entries || []);
    } catch (error) {
      list.replaceChildren();
      const note = document.createElement("div");
      note.className = "browser-empty";
      note.textContent = error.message;
      list.append(note);
      $("#browserStatus").textContent = "Folder unavailable";
    }
  }

  function renderPlaces(places, current) {
    const holder = $("#browserPlaces");
    holder.replaceChildren();
    const seen = new Set();
    places.forEach((place) => {
      const key = normalizePath(place.path);
      if (seen.has(key)) return;
      seen.add(key);
      const button = document.createElement("button");
      button.type = "button";
      button.className = "place-btn";
      if (normalizePath(place.path) === normalizePath(current)) button.classList.add("is-selected");
      button.append(svgIcon("folder"));
      const label = document.createElement("span");
      label.textContent = place.name || place.path;
      button.append(label);
      button.title = place.path;
      button.addEventListener("click", () => loadDirectory(place.path));
      holder.append(button);
    });
  }

  function renderBrowserEntries(entries) {
    const list = $("#browserList");
    list.replaceChildren();
    if (entries.length === 0) {
      const note = document.createElement("div");
      note.className = "browser-empty";
      note.textContent = "No subfolders here. Use this folder, or type a path above.";
      list.append(note);
      return;
    }
    entries.forEach((entry) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "browser-dir";
      button.append(svgIcon("folder"));
      const label = document.createElement("span");
      label.textContent = entry.name;
      button.append(label);
      button.title = entry.path;
      button.addEventListener("click", () => loadDirectory(entry.path));
      list.append(button);
    });
  }

  function bindBrowser() {
    $$("#browserDialog [data-close]").forEach((button) => {
      button.addEventListener("click", () => $("#browserDialog").close());
    });
    $("#browserDialog").addEventListener("click", (event) => {
      if (event.target === $("#browserDialog")) $("#browserDialog").close();
    });
    $("#browserUp").addEventListener("click", () => {
      const parent = $("#browserUp").dataset.parent;
      if (parent) loadDirectory(parent);
    });
    $("#browserGo").addEventListener("click", () => loadDirectory($("#browserPath").value.trim()));
    $("#browserPath").addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        loadDirectory($("#browserPath").value.trim());
      }
    });
    $("#browserUse").addEventListener("click", async () => {
      const typed = $("#browserPath").value.trim();
      const path = typed || state.browser.path;
      if (!path) return;
      const useButton = $("#browserUse");
      if (state.browser.mode === "watch") {
        useButton.disabled = true;
        try {
          await addWatchTarget(path, $("#watchRecursive").checked, $("#watchExisting").checked);
          $("#browserDialog").close();
        } catch (error) {
          toast("bad", "Could not watch folder", error.message);
        } finally {
          useButton.disabled = false;
        }
        return;
      }
      if (state.browser.mode === "source") {
        $("#sourcePath").value = path;
        state.scan = null;
        state.selected.clear();
        renderScanPreview();
        updateStartState();
        rememberFolder(path);
        $("#browserDialog").close();
        scanFolder();
      } else {
        const isArchive = state.browser.mode === "archive";
        const target = isArchive ? $("#archiveDir") : $("#outputDir");
        const bindingID = isArchive ? "archiveDir" : "outputDir";
        target.value = path;
        readBinding(BINDINGS.find((binding) => binding.id === bindingID));
        if (isArchive) {
          $("#deleteSource").checked = false;
          readBinding(BINDINGS.find((binding) => binding.id === "deleteSource"));
        }
        $("#browserDialog").close();
        updateLaunchSummary();
        scheduleConfigSync();
      }
    });
  }

  // -------------------------------------------------------------------------
  // Doctor
  // -------------------------------------------------------------------------
  async function loadDoctor() {
    const list = $("#checkList");
    list.innerHTML = '<li class="check-row"><div></div><div><div class="check-label">Running diagnostics…</div><div class="check-detail">Probing FFmpeg and the file system.</div></div></li>';
    try {
      const report = await api("/api/doctor");
      state.doctor = report;
      renderDoctor(report);
    } catch (error) {
      list.replaceChildren();
      const row = document.createElement("li");
      row.className = "check-row";
      row.dataset.status = "fail";
      const badge = document.createElement("span");
      badge.className = "check-badge";
      badge.textContent = "!";
      const text = document.createElement("div");
      const label = document.createElement("div");
      label.className = "check-label";
      label.textContent = "Diagnostics failed";
      const detail = document.createElement("div");
      detail.className = "check-detail";
      detail.textContent = error.message;
      text.append(label, detail);
      row.append(badge, text);
      list.append(row);
    }
  }

  function renderDoctor(report) {
    const env = $("#doctorEnv");
    env.replaceChildren();
    const rows = [
      ["version", report.version],
      ["platform", report.platform],
      ["runtime", report.goVersion],
      ["CPUs", String(report.cpus)],
      ["config", report.configPath],
      ["ffmpeg", report.health && report.health.ffmpeg],
      ["ffprobe", report.health && report.health.ffprobe],
      ["output", (state.config && state.config.outputDir) || "next to source"],
    ];
    rows.forEach(([key, value]) => {
      const row = document.createElement("div");
      const dt = document.createElement("dt");
      dt.textContent = key;
      const dd = document.createElement("dd");
      dd.textContent = value || "—";
      dd.title = value || "";
      row.append(dt, dd);
      env.append(row);
    });

    const list = $("#checkList");
    list.replaceChildren();
    let failures = 0;
    let warnings = 0;
    (report.checks || []).forEach((check) => {
      if (check.status === "fail") failures += 1;
      if (check.status === "warn") warnings += 1;
      const row = document.createElement("li");
      row.className = "check-row";
      row.dataset.status = check.status;
      const badge = document.createElement("span");
      badge.className = "check-badge";
      badge.append(check.status === "pass" ? svgIcon("check") : svgIcon("alert"));
      const text = document.createElement("div");
      const label = document.createElement("div");
      label.className = "check-label";
      label.textContent = check.label;
      const detail = document.createElement("div");
      detail.className = "check-detail";
      detail.textContent = check.detail || "";
      text.append(label, detail);
      if (check.hint) {
        const hint = document.createElement("div");
        hint.className = "check-hint";
        hint.textContent = check.hint;
        text.append(hint);
      }
      row.append(badge, text);
      list.append(row);
    });

    const summary = $("#doctorSummary");
    if (failures > 0) {
      summary.dataset.tone = "bad";
      summary.textContent = `${failures} failed`;
    } else if (warnings > 0) {
      summary.dataset.tone = "warn";
      summary.textContent = `${warnings} warning${warnings === 1 ? "" : "s"}`;
    } else {
      summary.dataset.tone = "ok";
      summary.textContent = "All checks pass";
    }

    const badges = $("#encoderBadges");
    badges.replaceChildren();
    const encoders = Object.keys(report.encoders || {});
    const priority = ["libx264", "libx265", "aac", "libopus", "libvpx-vp9", "libsvtav1", "prores_ks"];
    const priorityShown = priority.filter((name) => encoders.includes(name));
    const extra = encoders
      .filter((name) => /x264|x265|hevc|aac|opus|vp9|av1|prores/i.test(name) && !priorityShown.includes(name))
      .sort();
    const shown = [...priorityShown, ...extra].slice(0, 18);
    if (shown.length === 0) {
      const pill = document.createElement("span");
      pill.className = "pill";
      pill.textContent = "none detected";
      badges.append(pill);
    } else {
      shown.forEach((name) => {
        const pill = document.createElement("span");
        pill.className = "pill";
        pill.textContent = name;
        badges.append(pill);
      });
      if (encoders.length > shown.length) {
        const pill = document.createElement("span");
        pill.className = "pill";
        pill.textContent = `+${encoders.length - shown.length} more`;
        badges.append(pill);
      }
    }
  }

  async function copyDiagnostics() {
    if (!state.doctor) await loadDoctor();
    if (!state.doctor) return;
    const text = JSON.stringify(state.doctor, null, 2);
    try {
      await navigator.clipboard.writeText(text);
      toast("ok", "Diagnostics copied", "Paste it into your bug report.");
    } catch {
      const area = document.createElement("textarea");
      area.value = text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.append(area);
      area.select();
      try { document.execCommand("copy"); toast("ok", "Diagnostics copied"); }
      catch { toast("bad", "Clipboard unavailable", "Select the JSON from the report instead."); }
      area.remove();
    }
  }

  function bindDoctor() {
    $("#doctorRefresh").addEventListener("click", loadDoctor);
    $("#copyDiagnostics").addEventListener("click", copyDiagnostics);
  }

  // -------------------------------------------------------------------------
  // Init
  // -------------------------------------------------------------------------
  async function init() {
    initTheme();
    bindChrome();
    bindSettings();
    bindRecipes();
    bindSource();
    bindRuns();
    bindWatch();
    bindConfirm();
    bindBrowser();
    bindDoctor();
    renderRecentFolders();

    try {
      const status = await api("/api/status");
      let payload = null;
      try { payload = await api("/api/config"); } catch { payload = null; }
      state.config = payload && payload.config ? payload.config : status.config;
      state.configPath = (payload && payload.configPath) || status.configPath || "";
      state.recipes = (payload && payload.recipes) || [];
      state.capabilities = payload;
      writeConfigToForm();
      renderStatus(status);
      $("#footerConfig").textContent = state.configPath;

      try {
        const source = localStorage.getItem(SOURCE_KEY);
        if (source && !$("#sourcePath").value) $("#sourcePath").value = source;
      } catch { /* ignore */ }
      updateStartState();

      await refreshRuns();
      await refreshWatch({ silent: true });
      connectEvents();
    } catch (error) {
      toast("bad", "Dashboard cannot reach the server", error.message);
      setLive(false);
    }
  }

  document.addEventListener("DOMContentLoaded", init);
})();
