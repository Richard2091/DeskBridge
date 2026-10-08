// 原型核心（初版范围）：状态、路由、渲染循环、事件分发、外壳（精简标题栏、侧栏、手机框架）、浮层、原型控制台。

const S = {
  // 原型控制台
  theme: "clean", mode: "light", size: "desktop", scenario: "normal",
  // 桌面控制权：设备 ID → 状态（none、ai、local、human、paused），没有记录的设备为空闲。原型控制台只调 win-pc
  ctl: { "win-pc": "ai" },
  // 路由
  route: { page: "devices", dev: null, tab: "overview", sec: "appearance" },
  // 浮层
  modal: null, remote: null, menu: null, toasts: [], drawer: null, pop: null,
  exp: {},
  conn: "lan", // 设备首页当前显示的连接方式
  ts: "none", // Tailscale：none 未添加（默认只有局域网）、login 已添加未登录、on 已登录
  lanNotice: true, // 默认开启局域网发现时提示一次“其他设备能看到本机名称”
  // 各页面状态
  filter: "all", search: "",
  // 终端和文件按设备分开保存：设备 ID → 状态，第一次打开时创建（见 devices.js 的 cmdOf、fsOf）
  cmds: {}, fss: {},
  act: { who: "all", dev: "all", open: "0" },
  // 已打开的远程窗口：设备 ID → 窗口对象（截图用的假会话为 null）
  sessions: {}, win: null,
  set: {
    autostart: true, tray: true, lang: "简体中文", density: "标准", fontsize: "14",
    release: "3", lease: "15", idle: "60", onlyAway: false, remoteMode: "独立窗口", lanOn: true,
    hkPause: "Ctrl + Alt + P", hkStop: "Ctrl + Alt + End", server: "默认（Tailscale 官方）", serverUrl: "",
  },
  netTry: 3, tsLogging: false, tsErr: false, tsFailNext: false,
  stopped: {}, // 被紧急停止的 AI：设备 ID → { who, t, kill }
  connecting: {}, elevated: false, // 远程桌面：正在连接的条目；演示“前台是管理员窗口”
  remoteOpts: { quality: "自动（推荐）", refresh: "每 5 秒", monitor: "显示器 1（主）" },
};

const A = {}; // 动作表：data-a="名称"

// ---------- 工具函数 ----------
const $ = (sel, root = document) => root.querySelector(sel);
const esc = s => String(s ?? "").replace(/[&<>"]/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
// 设备条目：同一台设备在每种连接方式里各有一条，用“设备 ID@连接方式”区分
const devById = (id, via) => DEVICES.find(d => d.id === id && (!via || d.via === via));
const entryKey = d => `${d.id}@${d.via}`;
const devByKey = k => { const [id, via] = String(k).split("@"); return devById(id, via); };
const curDev = () => devById(S.route.dev, S.route.via);
const devUrl = (d, tab = "overview") => `#/devices/${d.via}/${d.id}/${tab}`;
const isPhone = () => S.size === "phone";
const netDown = () => S.scenario === "netdown";
const tsAdded = () => S.ts !== "none"; // 已添加 Tailscale 连接方式
const tsOn = () => S.ts === "on"; // 已登录 Tailscale
// 无法连接的原因：每种连接方式互相独立，Tailscale 断开不影响局域网里的同一台设备
function unreachWhy(d) {
  if (d.self) return "";
  if (d.via === "tailscale") return !tsOn() ? "tsoff" : netDown() ? "netdown" : "";
  if (d.via === "lan") return S.set.lanOn ? "" : "lanoff";
  return "";
}
const unreach = d => !!unreachWhy(d);
const kindIcon = d => d.kind;
const isWin = d => d.os.startsWith("Windows");

// 桌面控制权的 5 种状态：设备行、设备页、概览和远程桌面都从这里取文字和颜色，保证一致
const CTL = {
  none: { label: "空闲", cls: "off", stat: "空闲" },
  ai: { label: "AI 操作中", cls: "ai", live: true, stat: "Claude Code", sub: "AI 正在使用鼠标键盘" },
  local: { label: "本地用户在用", cls: "info", stat: "本地用户", sub: "AI 在排队" },
  human: { label: "我在远程操作", cls: "info", stat: "你", sub: "正在远程操作" },
  paused: { label: "被控端已暂停", cls: "bad", stat: "已暂停", sub: "本地用户暂停了远程操作" },
};
const ctlOf = d => (d && S.ctl[d.id]) || "none";
function setCtl(d, v) { if (v === "none") delete S.ctl[d.id]; else S.ctl[d.id] = v; }

function latBars(ms) {
  const q = ms < 30 ? 3 : ms < 100 ? 2 : 1;
  return `<span class="bars q${q}"><i></i><i></i><i></i></span>`;
}

function devState(d) {
  // 返回 [圆点类, 是否动态, 状态文字, 标签类]
  if (unreach(d)) return ["off", false, "状态未知", "off"];
  if (d.via === "lan" && !d.self && !d.paired) return ["warn", false, "未配对", "warn"];
  if (!d.online) return ["off", false, "离线", "off"];
  if (d.noNode) return ["off", false, "未安装 DeskBridge", "off"];
  const c = ctlOf(d);
  if (c !== "none") return [CTL[c].cls, !!CTL[c].live, CTL[c].label, CTL[c].cls];
  if (d.latency >= 100) return ["warn", false, "延迟高", "warn"];
  return ["", false, "在线", "ok"];
}

function dot(d) {
  const [c, live] = devState(d);
  return `<span class="pulse ${c} ${live ? "live" : ""}"></span>`;
}

function pathText(d) {
  if (d.path === "relay") return `中继（${d.relay}）`;
  return PATH_LABEL[d.path] || "";
}

function avatar(key, cls = "") {
  const a = ACTORS[key];
  return `<span class="avatar ${a.avc} ${cls}">${a.av}</span>`;
}

// 打开某台设备时选哪一条：优先当前连接方式，其次局域网，再次已添加的 Tailscale
function pickEntry(id) {
  const all = DEVICES.filter(d => d.id === id && (d.via !== "tailscale" || tsAdded()));
  return all.find(d => d.via === S.conn) || all.find(d => d.via === "lan") || all[0] || null;
}

function fmtSec(ms) {
  return (ms / 1000).toFixed(1) + " 秒";
}

function toast(kind, title, desc = "") {
  const id = Math.random().toString(36).slice(2);
  S.toasts.push({ id, kind, title, desc });
  renderToasts();
  setTimeout(() => { S.toasts = S.toasts.filter(t => t.id !== id); renderToasts(); }, 3800);
}

function go(hash) {
  if (location.hash === hash) render(); else location.hash = hash;
}

// ---------- 路由 ----------
function parseRoute() {
  const p = location.hash.replace(/^#\/?/, "").split("/").filter(Boolean);
  const r = { page: p[0] || "devices", dev: null, via: null, tab: "overview", sec: null };
  // 设备页地址：#/devices/<连接方式>/<设备>/<标签>
  if (r.page === "devices" && p[1]) {
    if (VIA_LABEL[p[1]]) { r.via = p[1]; r.dev = p[2] || null; r.tab = p[3] || "overview"; }
    else { r.dev = p[1]; r.tab = p[2] || "overview"; const e = pickEntry(r.dev); r.via = e && e.via; } // 不带连接方式的旧地址
  }
  const routeDev = r.dev && devById(r.dev, r.via);
  if (r.dev && (!routeDev || (routeDev.via === "tailscale" && !tsAdded()))) {
    // 这种连接方式里没有这台设备（或连接方式已移除）：回到设备列表并说明原因
    const name = r.dev;
    r.dev = null; r.via = null;
    location.replace("#/devices"); // 不用 history.replaceState：直接打开 file:// 时会报安全错误
    setTimeout(() => toast("info", `当前的连接方式里没有 ${name}`, tsAdded() ? "" : "它可能在 Tailscale 里，添加 Tailscale 后可以打开"), 0);
  }
  else if (routeDev) S.conn = routeDev.via; // 从设备页返回时回到它所在的连接方式
  if (r.page === "settings") r.sec = p[1] || null;
  if (r.tab === "cmd") r.tab = "term";
  if (["desk", "shot", "uia"].includes(r.tab)) r.tab = "overview"; // “桌面”标签已并入远程桌面
  if (r.page === "settings" && !r.sec && !isPhone()) r.sec = "appearance";
  S.route = r;
}

// ---------- 渲染 ----------
function render() {
  const app = $("#app");
  const mode = S.mode === "system" ? (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light") : S.mode;
  app.dataset.theme = S.theme;
  app.dataset.mode = mode;
  app.dataset.size = S.size;

  // 保留滚动位置和输入焦点
  const scrolls = {};
  app.querySelectorAll("[data-sk]").forEach(el => { scrolls[el.dataset.sk] = el.scrollTop; });
  const act = document.activeElement;
  const focusId = act && app.contains(act) ? act.id : null;
  const selStart = focusId && act.selectionStart;

  app.innerHTML = frame();

  app.querySelectorAll("[data-sk]").forEach(el => { if (scrolls[el.dataset.sk] != null) el.scrollTop = scrolls[el.dataset.sk]; });
  if (focusId) {
    const el = document.getElementById(focusId);
    if (el) { el.focus(); try { el.setSelectionRange(selStart, selStart); } catch (e) { /* 非文本输入 */ } }
  }
  const out = $("#out");
  if (out && S.route.dev && cmdOf(S.route.dev).status === "run") out.scrollTop = out.scrollHeight;
  renderToasts();
  if (S.win !== "remote") renderProto();
}

function frame() {
  if (S.win === "remote") return remoteWindow(devByKey(S.remote)) + overlays();
  const top = isPhone()
    ? `<div class="phonebar"><span>9:41</span><span class="notch"></span><span class="row" style="gap:5px">${ic("wifi", "sm")}<b style="font-size:12px">87%</b></span></div>`
    : titlebar();
  const body = isPhone() ? phoneShell() : desktopShell();
  return top + body + overlays();
}

// 标题栏：品牌、按需出现的“远程窗口 N”、窗口按钮。整条可以拖动窗口。
function titlebar() {
  const sess = Object.keys(S.sessions);
  return `<div class="titlebar">
    <div class="tb-brand"><span class="logo">${ic("bridge")}</span><b>DeskBridge</b></div>
    <span class="spacer"></span>
    ${sess.length ? `<div class="tb-right"><button class="tb-chip" data-a="popSessions" title="远程窗口">${ic("cast", "sm")}<span class="label">远程窗口</span><span class="n">${sess.length}</span></button></div>` : ""}
    <div class="ctl"><span>—</span><span>☐</span><span>✕</span></div>
  </div>`;
}

function desktopShell() {
  const p = S.route.page;
  const nav = (page, icon, label, on = p === page) =>
    `<a class="${on ? "on" : ""}" data-go="#/${page}" title="${label}">${ic(icon)}<span class="label">${label}</span></a>`;
  return `<div class="shell">
    <aside class="nav">
      ${nav("devices", "monitor", "设备")}
      ${nav("activity", "history", "操作记录")}
      <div class="grow"></div>
      ${nav("settings", "sliders", "设置")}
    </aside>
    <div class="content"><section class="page">${pageBody()}</section></div>
  </div>`;
}

function pageBody() {
  switch (S.route.page) {
    case "devices": return viewDevices();
    case "activity": return viewActivity();
    case "settings": return viewSettings();
    default: return viewDevices();
  }
}

function phoneShell() {
  const p = S.route.page;
  const tab = (page, icon, label, extra = "") =>
    `<a class="${p === page ? "on" : ""}" data-go="#/${page}">${ic(icon)}${label}${extra}</a>`;
  return `<div class="mshell">
    <div class="mbody" data-sk="mbody">${pageBody()}</div>
    <nav class="mtabbar">
      ${tab("devices", "monitor", "设备")}
      ${tab("activity", "history", "记录")}
      ${tab("settings", "sliders", "设置")}
    </nav>
  </div>`;
}

function overlays() {
  let h = "";
  if (S.remote && S.win !== "remote") h += viewRemote();
  if (S.modal) h += `<div class="modal-wrap" data-a="closeModalBg">${MODALS[S.modal.type]()}</div>`;
  if (S.drawer) h += `<div class="drawer-wrap" data-a="closeDrawerBg">${DRAWERS[S.drawer.type]()}</div>`;
  if (S.pop) h += S.pop.html();
  if (S.menu) h += S.menu.html;
  h += `<div class="toasts" id="toasts"></div>`;
  return h;
}

function renderToasts() {
  const el = $("#toasts");
  if (!el) return;
  el.innerHTML = S.toasts.map(t => `<div class="toast"><span class="ic ${t.kind}">${ic({ ok: "check", warn: "alert", bad: "x", info: "info", ai: "sparkles" }[t.kind], "sm")}</span><div><b>${t.title}</b>${t.desc ? `<div class="d">${t.desc}</div>` : ""}</div></div>`).join("");
}

// ---------- 菜单 ----------
function openMenu(anchor, items) {
  const app = $("#app").getBoundingClientRect();
  const r = anchor.getBoundingClientRect();
  const right = app.right - r.right;
  const top = r.bottom - app.top + 4;
  const html = `<div class="menu" style="top:${top}px;right:${right}px">${items.map(i => i === "-" ? `<div class="sep"></div>` :
    `<a class="${i.bad ? "bad" : ""}" data-a="menuPick" data-msg="${esc(i.msg || "")}" data-act="${i.act || ""}" data-id="${i.id || ""}" data-key="${i.key || ""}">${ic(i.icon)}${i.t}</a>`).join("")}</div>`;
  S.menu = { html };
  render();
}
A.menuPick = el => { S.menu = null; if (el.dataset.msg) toast("ok", el.dataset.msg); if (el.dataset.act) A[el.dataset.act](el); render(); };

// ---------- 抽屉与弹出面板 ----------
const DRAWERS = {};
const POPS = {};
function drawer({ title, sub = "", body, foot = "" }) {
  return `<aside class="drawer"><div class="drawer-h"><div style="min-width:0"><h3>${title}</h3>${sub ? `<div class="sub">${sub}</div>` : ""}</div><button class="btn ghost icon sm x" data-a="closeDrawer">${ic("x")}</button></div>
    <div class="drawer-b">${body}</div>${foot ? `<div class="drawer-f">${foot}</div>` : ""}</aside>`;
}
A.closeDrawer = () => { S.drawer = null; render(); };
A.closeDrawerBg = (el, ev) => { if (ev.target === el) A.closeDrawer(); };
function openPop(anchor, type) {
  if (S.pop && S.pop.type === type) { S.pop = null; render(); return; }
  const app = $("#app").getBoundingClientRect();
  const r = anchor.getBoundingClientRect();
  const right = Math.max(8, app.right - r.right);
  const below = r.bottom - app.top < app.height * .6;
  const pos = below ? `top:${r.bottom - app.top + 6}px` : `bottom:${app.bottom - r.top + 6}px`;
  S.pop = { type, html: () => `<div class="pop" style="${pos};right:${right}px">${POPS[type]()}</div>` };
  render();
}

// ---------- 对话框 ----------
const MODALS = {};
function modal({ title, sub = "", body, foot, wide = false, icon = "" }) {
  return `<div class="modal ${wide ? "wide" : ""}">
    <div class="modal-h">${icon}<div><h3>${title}</h3>${sub ? `<div class="sub">${sub}</div>` : ""}</div><button class="btn ghost icon sm x" data-a="closeModal">${ic("x")}</button></div>
    <div class="modal-b">${body}</div>
    ${foot ? `<div class="modal-f">${foot}</div>` : ""}
  </div>`;
}
A.closeModal = () => { S.modal = null; render(); };
A.closeModalBg = (el, ev) => { if (ev.target === el) A.closeModal(); };
A.modalOpt = el => { S.modal[el.dataset.k] = el.dataset.v; render(); };
// 通用确认框：危险或难以撤销的操作先确认
function confirmBox({ title, body, ok, act, danger = false }) { S.modal = { type: "confirm", title, body, ok, act, danger }; render(); }
MODALS.confirm = () => modal({
  title: S.modal.title, body: `<p class="muted" style="line-height:1.7">${S.modal.body}</p>`,
  foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn ${S.modal.danger ? "danger" : "primary"}" data-a="confirmOk">${S.modal.ok}</button>`,
});
A.confirmOk = () => { const act = S.modal.act; S.modal = null; A[act](); };
A.modalToggle = el => { const k = el.dataset.k; S.modal[k] = !S.modal[k]; render(); };

// ---------- 原型控制台 ----------
function renderProto() {
  const cur = key => key === "control" ? ctlOf(devById("win-pc")) : S[key];
  const seg = (key, opts) => `<div class="seg">${opts.map(([v, l]) =>
    `<button class="${cur(key) === v ? "on" : ""}" data-p="${key}" data-v="${v}">${l}</button>`).join("")}</div>`;
  $("#proto").innerHTML = `
    <div class="proto-head"><span>原型控制台</span><button data-p="toggleProto" title="收起或展开">⇆</button></div>
    <div><h4>主题</h4>${seg("theme", [["clean", "清爽"], ["mono", "黑白"], ["soft", "柔光"]])}</div>
    <div><h4>明暗</h4>${seg("mode", [["light", "浅色"], ["dark", "深色"], ["system", "跟随系统"]])}</div>
    <div><h4>窗口</h4>${seg("size", [["desktop", "桌面"], ["narrow", "窄窗口"], ["phone", "安卓手机"]])}</div>
    <div><h4>场景</h4>${seg("scenario", [["normal", "正常"], ["loading", "加载中"], ["firstrun", "首次启动"], ["error", "出错"], ["netdown", "组网断开"]])}</div>
    <div><h4>win-pc 的桌面控制权</h4>${seg("control", Object.entries(CTL).map(([v, c]) => [v, c.label]))}</div>
    <div><h4>流程与页面</h4><div class="links">
      <a data-p="flow" data-v="welcome">① 首次启动（只有局域网）→ 添加 Tailscale</a>
      <a data-p="flow" data-v="add">② 添加设备</a>
      <a data-p="flow" data-v="cmd">③ 执行并取消命令</a>
      <a data-p="flow" data-v="takeover">④ AI 操作中：提示与抢回</a>
      <a data-p="flow" data-v="preempt">⑤ 模拟本地用户动鼠标</a>
      <a data-p="flow" data-v="netdown">⑥ 断线与重连</a>
      <a data-p="flow" data-v="remote">⑦ 远程桌面（独立窗口）</a>
      <a data-p="flow" data-v="relay">⑧ 高延迟设备（中继，截图模式）</a>
      <a data-p="flow" data-v="offline">⑨ 离线设备</a>
      <a data-p="flow" data-v="nonode">⑩ 未安装 DeskBridge 的设备</a>
      <a data-p="flow" data-v="lan">⑪ 局域网设备配对（含输错）</a>
      <a data-p="flow" data-v="mycode">⑫ 显示本机配对码</a>
      <a data-p="flow" data-v="tsfail">⑬ Tailscale 登录失败</a>
      <a data-p="flow" data-v="cmderr">⑭ 命令失败和超时</a>
      <a data-p="flow" data-v="elevated">⑮ 远程桌面：前台是管理员窗口</a>
    </div></div>
    <div class="note">这是不接后端的原型，所有数据都是示例。点击界面中的按钮、标签页和列表都有反应。</div>`;
}

function onProto(el) {
  const k = el.dataset.p, v = el.dataset.v;
  if (k === "toggleProto") { $("#proto").classList.toggle("closed"); return; }
  if (k === "flow") {
    if (!["welcome", "netdown"].includes(v)) S.scenario = "normal";
    return FLOWS[v]();
  }
  if (k === "control") { setCtl(devById("win-pc"), v); render(); return; }
  S[k] = v;
  if (k === "scenario" && v === "netdown") S.netTry = 1;
  if (k === "scenario") applyScenario();
  if (k === "size") { if (v === "phone") S.search = ""; parseRoute(); }
  if (k === "scenario") parseRoute();
  render();
}

// 场景只负责修正演示前提：默认和首次启动只有局域网；列表加载、错误和断线流程先临时启用 Tailscale
function applyScenario() {
  if (S.scenario === "firstrun") { S.ts = "none"; S.conn = "lan"; S.lanNotice = true; S.tsLogging = false; }
  else if (["loading", "error", "netdown"].includes(S.scenario) && S.ts === "none") S.ts = "on";
}

const FLOWS = {
  welcome: () => { S.scenario = "firstrun"; applyScenario(); go("#/devices"); },
  add: () => { go("#/devices"); setTimeout(A.addDevice, 0); },
  cmd: () => { S.ts = "on"; S.conn = "tailscale"; Object.assign(cmdOf("win-pc"), { status: "idle", lines: [] }); go("#/devices/tailscale/win-pc/term"); setTimeout(() => { A.runCmd(); toast("info", "命令开始执行", "运行几秒后点“取消”，看看取消后的状态"); }, 200); },
  takeover: () => { S.ts = "on"; S.conn = "tailscale"; S.ctl["win-pc"] = "ai"; go("#/devices/tailscale/win-pc/overview"); setTimeout(() => toast("ai", "Claude Code 正在操作 win-pc 的桌面", "点提示条上的“停止”或“接管”"), 100); },
  preempt: () => { S.ts = "on"; S.conn = "tailscale"; S.ctl["win-pc"] = "ai"; go("#/devices/tailscale/win-pc/overview"); setTimeout(A.simLocal, 600); },
  netdown: () => { S.scenario = "netdown"; applyScenario(); S.netTry = 1; S.conn = "tailscale"; go("#/devices"); setTimeout(() => { if (netDown()) { S.netTry = 2; render(); } }, 1500); },
  remote: () => { S.ts = "on"; S.conn = "tailscale"; go("#/devices/tailscale/win-pc/overview"); setTimeout(A.openRemote, 0); },
  relay: () => { S.ts = "on"; S.conn = "tailscale"; go("#/devices/tailscale/old-laptop/overview"); setTimeout(A.openRemote, 0); },
  offline: () => { S.ts = "on"; S.conn = "tailscale"; go("#/devices/tailscale/redmi-phone/overview"); },
  nonode: () => { S.ts = "on"; S.conn = "tailscale"; go("#/devices/tailscale/nas/overview"); },
  lan: () => { S.scenario = "normal"; devById("study-pc", "lan").paired = false; S.conn = "lan"; go("#/devices"); setTimeout(() => A.pairDevice({ dataset: { id: "study-pc" } }), 0); },
  mycode: () => { S.scenario = "normal"; S.conn = "lan"; devById("media-pc", "lan").paired = false; go("#/devices"); setTimeout(A.showPairCode, 0); },
  tsfail: () => { S.scenario = "normal"; if (S.ts === "on") S.ts = "login"; S.tsLogging = false; S.tsFailNext = true; S.conn = "tailscale"; go("#/devices"); setTimeout(A.tsLogin, 0); },
  cmderr: () => {
    S.scenario = "normal"; S.ts = "on"; S.conn = "tailscale";
    Object.assign(cmdOf("win-pc"), { status: "idle", lines: [], text: "git -C D:\\data\\projects\\deskbridge pull --ff-only" });
    go("#/devices/tailscale/win-pc/term");
    setTimeout(() => { A.runCmd(); toast("info", "这条命令会失败", "再试试输入 winget upgrade --all，看超时的样子"); }, 200);
  },
  elevated: () => { S.scenario = "normal"; S.elevated = true; S.conn = "lan"; go("#/devices/lan/win-pc/overview"); setTimeout(() => { S.remote = "win-pc@lan"; render(); }, 0); },
};

A.simLocal = () => {
  if (S.ctl["win-pc"] !== "ai") return;
  S.ctl["win-pc"] = "local";
  toast("warn", "本地用户开始使用 win-pc", "AI 的前台输入已被中断（preempted），3 秒无操作后自动继续");
  render();
  setTimeout(() => {
    if (S.ctl["win-pc"] === "local") { S.ctl["win-pc"] = "ai"; toast("ai", "本地用户已空闲 3 秒", "Claude Code 继续操作桌面"); render(); }
  }, 6000);
};

A.netRetry = () => {
  S.netTry += 1; render();
  setTimeout(() => { S.scenario = "normal"; toast("ok", "Tailscale 已重新连接", "设备列表已刷新"); render(); }, 1200);
};

// ---------- 事件 ----------
document.addEventListener("click", ev => {
  const p = ev.target.closest("[data-p]");
  if (p) { ev.preventDefault(); onProto(p); return; }
  if (S.menu && !ev.target.closest(".menu")) { S.menu = null; render(); if (!ev.target.closest("[data-a]")) return; }
  if (S.pop && !ev.target.closest(".pop") && !ev.target.closest("[data-a^=pop]")) { S.pop = null; render(); if (!ev.target.closest("[data-a],[data-go]")) return; }
  const g = ev.target.closest("[data-go]");
  const a = ev.target.closest("[data-a]");
  // 最内层优先
  if (a && (!g || g.contains(a))) { ev.preventDefault(); A[a.dataset.a]?.(a, ev); return; }
  if (g) { ev.preventDefault(); go(g.dataset.go); }
});

document.addEventListener("input", ev => {
  const el = ev.target.closest("[data-in]");
  if (!el) return;
  const k = el.dataset.in;
  const path = k.split(".");
  let o = S;
  for (let i = 0; i < path.length - 1; i++) o = o[path[i]];
  o[path.at(-1)] = el.value;
  if (el.dataset.rerender) render();
});

let longPressTimer = 0, longPressed = false;
document.addEventListener("pointerdown", ev => {
  if (!isPhone() || ev.button !== 0) return;
  const row = ev.target.closest(".drow[data-id]");
  if (!row || ev.target.closest("button, a")) return;
  longPressed = false;
  clearTimeout(longPressTimer);
  longPressTimer = setTimeout(() => {
    longPressed = true;
    openMenu({ getBoundingClientRect: () => row.getBoundingClientRect() }, [
      { icon: "terminal", t: "终端", act: "rowTerm", key: `${row.dataset.id}@${row.dataset.via}` },
      { icon: "folder", t: "文件", act: "rowFiles", key: `${row.dataset.id}@${row.dataset.via}` },
      { icon: "cast", t: "远程桌面", act: "openRemote", key: `${row.dataset.id}@${row.dataset.via}` },
    ]);
  }, 450);
});
["pointerup", "pointercancel", "pointermove"].forEach(type => document.addEventListener(type, () => clearTimeout(longPressTimer)));
document.addEventListener("click", ev => {
  if (!longPressed) return;
  ev.preventDefault();
  ev.stopPropagation();
  longPressed = false;
}, true);

document.addEventListener("keydown", ev => {
  if (ev.key === "Escape") {
    if (S.menu) S.menu = null;
    else if (S.pop) S.pop = null;
    else if (S.drawer) S.drawer = null;
    else if (S.modal) S.modal = null;
    else if (S.remote) {
      // 正在控制远程桌面时，Esc 发给远端，不关闭远程桌面；独立远程窗口里 Esc 也不关闭窗口
      const d = devByKey(S.remote);
      if (ctlOf(d) === "human") { toast("info", `Esc 已发送到 ${d.name}`, "要断开远程桌面，请点工具栏上的“断开”"); return; }
      if (S.win !== "remote") S.remote = null;
    }
    render();
    return;
  }
  if (ev.key === "Enter") {
    if (ev.target.id === "cmdin") { ev.preventDefault(); A.runCmd(); }
    if (ev.target.id === "paircode") { ev.preventDefault(); A.pairConfirm(); }
  }
});

window.addEventListener("hashchange", () => { S.menu = null; S.pop = null; S.drawer = null; parseRoute(); render(); });
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => { if (S.mode === "system") render(); });

// 检查远程窗口是否已被关闭
setInterval(() => {
  const closed = Object.entries(S.sessions).filter(([, w]) => w && w.closed).map(([id]) => id);
  if (closed.length) { closed.forEach(id => delete S.sessions[id]); render(); }
  const rt = $("#runtime");
  const rc = S.route.dev && S.cmds[S.route.dev];
  if (rt && rc && rc.status === "run") rt.textContent = fmtSec(Date.now() - rc.started);
}, 1000);

window.addEventListener("DOMContentLoaded", () => {
  const q = new URLSearchParams(location.search);
  ["theme", "mode", "size", "scenario", "conn"].forEach(k => { if (q.get(k)) S[k] = q.get(k); });
  if (isPhone()) S.search = "";
  applyScenario();
  if (q.get("conn")) S.conn = q.get("conn");
  if (q.get("proto") === "closed") $("#proto").classList.add("closed");
  // 远程窗口：?win=remote&dev=win-pc&via=lan，由主窗口用 window.open 打开
  if (q.get("win") === "remote") { S.win = "remote"; S.remote = `${q.get("dev") || "win-pc"}@${q.get("via") || "lan"}`; S.size = "remote"; document.body.classList.add("rwin"); document.title = `${q.get("dev") || "win-pc"} - 远程桌面`; }
  if (S.win === "remote") startConnecting(S.remote);
  if (q.get("control")) { S.ctl = {}; setCtl(S.remote ? devByKey(S.remote) : devById("win-pc"), q.get("control")); }
  // 截图用：?sess=win-pc@tailscale,old-laptop 假装已打开这些远程窗口（不写连接方式时按 Tailscale）
  if (q.get("sess")) q.get("sess").split(",").forEach(k => { S.sessions[k.includes("@") ? k : k + "@tailscale"] = null; });
  parseRoute();
  render();
  if (q.get("modal") && A[q.get("modal")]) A[q.get("modal")]({ dataset: { id: q.get("id") || "" } });
});

// ---------- 远程窗口列表（标题栏） ----------
POPS.sessions = () => `<div class="ph2">远程窗口</div>${Object.keys(S.sessions).map(k => {
  const d = devByKey(k);
  return `<div class="pi" style="cursor:default">${ic(kindIcon(d))}<div style="flex:1;min-width:0"><div>${d.name}</div><div class="small muted">${VIA_LABEL[d.via]} · ${pathText(d)} · ${d.latency} 毫秒</div></div>
    <button class="btn sm" data-a="focusRemote" data-key="${k}">切换</button><button class="btn ghost sm danger-soft" data-a="closeSession" data-key="${k}">断开</button></div>`;
}).join("")}<div class="small muted" style="padding:6px 8px 4px;line-height:1.6">每台设备的远程桌面在独立窗口中打开，可以同时操作多台。</div>`;
A.popSessions = el => openPop(el, "sessions");
