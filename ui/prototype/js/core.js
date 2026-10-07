// 原型核心：状态、路由、渲染循环、事件分发、外壳（窗口、侧栏、手机框架）、浮层。

const S = {
  // 原型控制台
  theme: "clean", mode: "light", size: "desktop", scenario: "normal", control: "ai",
  // 路由
  route: { page: "devices", dev: "win-pc", tab: "overview", sec: "appearance" },
  // 浮层
  modal: null, palette: false, paletteQ: "", remote: null, menu: null, toasts: [], drawer: null, pop: null,
  exp: {}, deskMode: "shot",
  // 各页面状态
  filter: "all", search: "",
  cmd: { shell: "PowerShell 7", text: "Get-ChildItem D:\\data -Recurse | Measure-Object -Sum Length", status: "run", lines: CMD_OUTPUTS.default.slice(0, 12), started: Date.now() - 8400, timeout: true, utf8: true },
  fs: { path: "D:\\data\\projects", sel: "readme.md", upload: null },
  shot: { mon: "1", sel: 0, taking: false, list: [
    { t: "14:03:22", by: "示例用户", mon: "显示器 1" },
    { t: "14:01:05", by: "Claude Code", mon: "显示器 1", ai: true },
    { t: "13:20:41", by: "Claude Code", mon: "全部", ai: true },
  ] },
  uia: { win: "np", sel: "doc", open: { w: true, mb: true, tb: false, sb: false } },
  agent: { who: "Claude Code", running: false, steps: [], prompt: "在 D:\\data\\projects\\deskbridge 下编译，修复所有报错，最后汇报改了哪些文件。" },
  act: { who: "all", dev: "all", open: "0" },
  appr: { tab: "pending", items: APPROVALS.map(a => ({ ...a })), done: [] },
  rules: RULES.map(r => ({ ...r })),
  set: {
    autostart: true, tray: true, lang: "简体中文", density: "标准", fontsize: "14",
    allowCmd: true, allowDesktop: true, allowInput: true, allowAgent: false,
    release: "3", lease: "15", idle: "60", onlyAway: false, askBeforeInput: false,
    hkPause: "Ctrl + Alt + P", hkStop: "Ctrl + Alt + End", server: "默认（Tailscale）",
  },
  onb: { step: 0, adv: false, name: "my-laptop", allowCmd: true, allowDesk: true, allowInput: false, autostart: true },
  netTry: 3,
};

const A = {}; // 动作表：data-a="名称"

// ---------- 工具函数 ----------
const $ = (sel, root = document) => root.querySelector(sel);
const esc = s => String(s ?? "").replace(/[&<>"]/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const devById = id => DEVICES.find(d => d.id === id);
const isPhone = () => S.size === "phone";
const netDown = () => S.scenario === "netdown";
const kindIcon = d => d.kind;

function latBars(ms) {
  const q = ms < 30 ? 3 : ms < 100 ? 2 : 1;
  return `<span class="bars q${q}"><i></i><i></i><i></i></span>`;
}

function devState(d) {
  // 返回 [圆点类, 是否动态, 状态文字, 标签类]
  if (netDown() && !d.self) return ["off", false, "状态未知", "off"];
  if (!d.online) return ["off", false, "离线", "off"];
  if (d.noNode) return ["off", false, "未安装 DeskBridge", "off"];
  if (d.id === "win-pc" && S.control === "ai") return ["ai", true, "AI 操作中", "ai"];
  if (d.id === "win-pc" && S.control === "local") return ["info", false, "本地用户使用中", "info"];
  if (d.id === "win-pc" && S.control === "paused") return ["bad", false, "已暂停远程操作", "bad"];
  if (d.latency >= 150) return ["warn", false, "延迟高", "warn"];
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
  const r = { page: p[0] || "devices", dev: null, tab: "overview", sec: null };
  if (r.page === "devices") { r.dev = p[1] || null; r.tab = p[2] || "overview"; }
  if (r.page === "settings") r.sec = p[1] || null;
  if (r.tab === "cmd") r.tab = "term";
  if (["shot", "uia"].includes(r.tab)) { S.deskMode = r.tab; r.tab = "desk"; }
  if (r.tab === "agent") r.tab = "ai";
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
  if (out && S.cmd.status === "run") out.scrollTop = out.scrollHeight;
  renderToasts();
  renderProto();
}

function frame() {
  const top = isPhone()
    ? `<div class="phonebar"><span>9:41</span><span class="notch"></span><span class="row" style="gap:5px">${ic("wifi", "sm")}<b style="font-size:12px">87%</b></span></div>`
    : `<div class="titlebar">DeskBridge${S.route.page === "welcome" ? "" : ` <span class="faint" style="margin-left:8px">— ${pageTitle()}</span>`}<div class="ctl"><span>—</span><span>☐</span><span>✕</span></div></div>`;
  let body;
  if (S.route.page === "welcome") body = viewWelcome();
  else body = isPhone() ? phoneShell() : desktopShell();
  return top + body + overlays();
}

function pageTitle() {
  return { devices: "设备", activity: "操作记录", approvals: "审批", shared: "外部设备", settings: "设置" }[S.route.page] || "";
}

function pendingCount() { return S.appr.items.length; }

function desktopShell() {
  const p = S.route.page;
  const nav = (page, icon, label, extra = "") =>
    `<a class="${p === page ? "on" : ""}" data-go="#/${page}" title="${label}">${ic(icon)}<span class="label">${label}</span>${extra}</a>`;
  return `<div class="shell">
    <aside class="nav">
      <div class="brand"><span class="logo">${ic("bridge")}</span><span class="label">DeskBridge</span></div>
      <div class="quick" data-a="openPalette" title="搜索或执行（Ctrl K）">${ic("search")}<span class="label">搜索或执行…</span><span class="kbd">Ctrl K</span></div>
      ${nav("devices", "monitor", "设备", "")}
      ${nav("activity", "history", "操作记录")}
      ${nav("approvals", "shield", "审批", pendingCount() ? `<span class="badge">${pendingCount()}</span>` : "")}
      ${nav("settings", "sliders", "设置")}
      <div class="grow"></div>
      <div class="foot" data-go="#/settings/network" style="cursor:pointer" title="${netDown() ? "组网已断开" : "组网正常"}"><span class="avatar">示</span><div style="min-width:0"><div class="n" style="font-weight:600">示例用户</div>
        <div class="s">${netDown() ? `<span class="pulse bad"></span>组网已断开` : `<span class="pulse"></span>组网正常`}</div></div></div>
    </aside>
    <div class="content">${netBanner()}<section class="page">${pageBody()}</section></div>
  </div>`;
}

function netBanner() {
  if (!netDown()) return "";
  return `<div class="netbar banner bad">${ic("wifiOff")}<div class="t"><b>组网已断开</b> <span class="d">正在重连（第 ${S.netTry} 次）</span></div>
    <div class="btns"><button class="btn sm" data-a="netRetry">${ic("refresh")}立即重试</button></div></div>`;
}

function pageBody() {
  switch (S.route.page) {
    case "devices": return viewDevices();
    case "activity": return viewActivity();
    case "approvals": return viewApprovals();
    case "shared": return viewShared();
    case "settings": return viewSettings();
    default: return viewDevices();
  }
}

function phoneShell() {
  const p = S.route.page;
  const tab = (page, icon, label, extra = "") =>
    `<a class="${p === page || (page === "devices" && p === "shared") ? "on" : ""}" data-go="#/${page}">${ic(icon)}${label}${extra}</a>`;
  return `<div class="mshell">
    ${netBanner()}
    <div class="mbody" data-sk="mbody">${pageBody()}</div>
    <nav class="mtabbar">
      ${tab("devices", "monitor", "设备")}
      ${tab("activity", "history", "记录")}
      ${tab("approvals", "shield", "审批", pendingCount() ? `<span class="badge">${pendingCount()}</span>` : "")}
      ${tab("settings", "sliders", "设置")}
    </nav>
  </div>`;
}

function overlays() {
  let h = "";
  if (S.remote) h += viewRemote();
  if (S.modal) h += `<div class="modal-wrap" data-a="closeModalBg">${MODALS[S.modal.type]()}</div>`;
  if (S.drawer) h += `<div class="drawer-wrap" data-a="closeDrawerBg">${DRAWERS[S.drawer.type]()}</div>`;
  if (S.pop) h += S.pop.html();
  if (S.palette) h += viewPalette();
  if (S.menu) h += S.menu.html;
  h += `<div class="toasts" id="toasts"></div>`;
  return h;
}

function renderToasts() {
  const el = $("#toasts");
  if (!el) return;
  el.innerHTML = S.toasts.map(t => `<div class="toast"><span class="ic ${t.kind}">${ic({ ok: "check", warn: "alert", bad: "x", info: "info", ai: "sparkles" }[t.kind], "sm")}</span><div><b>${t.title}</b>${t.desc ? `<div class="d">${t.desc}</div>` : ""}</div></div>`).join("");
}

// ---------- 命令面板 ----------
function paletteItems() {
  const q = S.paletteQ.trim().toLowerCase();
  const items = [];
  DEVICES.filter(d => !d.self).forEach(d => items.push({ sec: "设备", icon: kindIcon(d), t: d.name, r: d.os, go: `#/devices/${d.id}/overview` }));
  items.push(
    { sec: "操作", icon: "terminal", t: "在 win-pc 上执行命令", r: "终端", go: "#/devices/win-pc/term" },
    { sec: "操作", icon: "image", t: "给 win-pc 截图", r: "截图", go: "#/devices/win-pc/desk" },
    { sec: "操作", icon: "cast", t: "打开 win-pc 的远程画面", r: "远程画面", a: "openRemote" },
    { sec: "操作", icon: "plus", t: "添加设备", r: "", a: "addDevice" },
    { sec: "操作", icon: "moon", t: "切换深色和浅色", r: "外观", a: "toggleMode" },
    { sec: "页面", icon: "history", t: "操作记录", r: "", go: "#/activity" },
    { sec: "页面", icon: "users", t: "外部设备与共享", r: "", go: "#/shared" },
    { sec: "页面", icon: "shield", t: "审批", r: `${pendingCount()} 项待处理`, go: "#/approvals" },
    { sec: "页面", icon: "plug", t: "AI 接入（MCP 配置）", r: "设置", go: "#/settings/ai" },
  );
  return items.filter(i => !q || (i.t + i.r).toLowerCase().includes(q));
}

function viewPalette() {
  const items = paletteItems();
  let sec = "";
  const rows = items.map((it, i) => {
    const head = it.sec !== sec ? `<div class="sec">${(sec = it.sec)}</div>` : "";
    return head + `<div class="it ${i === 0 ? "on" : ""}" data-a="paletteGo" data-i="${i}">${ic(it.icon)}<span>${esc(it.t)}</span><span class="r">${esc(it.r)}</span></div>`;
  }).join("");
  return `<div class="palette" data-a="closePaletteBg"><div class="box">
    <div class="in">${ic("search", "lg")}<input id="pq" placeholder="搜索设备、操作或页面…" value="${esc(S.paletteQ)}" data-in="paletteQ" autocomplete="off"><span class="kbd">Esc</span></div>
    <div class="list">${rows || `<div class="empty" style="padding:24px">没有匹配的结果</div>`}</div>
  </div></div>`;
}

A.openPalette = () => { S.palette = true; S.paletteQ = ""; render(); setTimeout(() => $("#pq")?.focus(), 0); };
A.closePaletteBg = (el, ev) => { if (ev.target === el) { S.palette = false; render(); } };
A.paletteGo = el => {
  const it = paletteItems()[+el.dataset.i];
  S.palette = false;
  if (it.go) go(it.go); else A[it.a]();
  if (it.a === "toggleMode") render();
};
A.toggleMode = () => { S.mode = S.mode === "dark" ? "light" : "dark"; render(); };

// ---------- 菜单 ----------
function openMenu(anchor, items) {
  const app = $("#app").getBoundingClientRect();
  const r = anchor.getBoundingClientRect();
  const right = app.right - r.right;
  const top = r.bottom - app.top + 4;
  const html = `<div class="menu" style="top:${top}px;right:${right}px">${items.map(i => i === "-" ? `<div class="sep"></div>` :
    `<a class="${i.bad ? "bad" : ""}" data-a="menuPick" data-msg="${esc(i.msg || "")}">${ic(i.icon)}${i.t}</a>`).join("")}</div>`;
  S.menu = { html };
  render();
}
A.menuPick = el => { S.menu = null; if (el.dataset.msg) toast("ok", el.dataset.msg); render(); };

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
A.modalToggle = el => { const k = el.dataset.k; S.modal[k] = !S.modal[k]; render(); };

// ---------- 原型控制台 ----------
function renderProto() {
  const seg = (key, opts) => `<div class="seg">${opts.map(([v, l]) =>
    `<button class="${S[key] === v ? "on" : ""}" data-p="${key}" data-v="${v}">${l}</button>`).join("")}</div>`;
  $("#proto").innerHTML = `
    <div class="proto-head"><span>原型控制台</span><button data-p="toggleProto" title="收起或展开">⇆</button></div>
    <div><h4>主题</h4>${seg("theme", [["clean", "清爽"], ["mono", "黑白"], ["soft", "柔光"]])}</div>
    <div><h4>明暗</h4>${seg("mode", [["light", "浅色"], ["dark", "深色"], ["system", "跟随系统"]])}</div>
    <div><h4>窗口</h4>${seg("size", [["desktop", "桌面"], ["narrow", "窄窗口"], ["phone", "安卓手机"]])}</div>
    <div><h4>场景</h4>${seg("scenario", [["normal", "正常"], ["loading", "加载中"], ["empty", "没有设备"], ["error", "出错"], ["netdown", "组网断开"]])}</div>
    <div><h4>win-pc 的桌面控制权</h4>${seg("control", [["none", "空闲"], ["ai", "AI 操作中"], ["local", "本地用户在用"], ["human", "我在远程操作"], ["paused", "被控端已暂停"]])}</div>
    <div><h4>流程与页面</h4><div class="links">
      <a data-p="flow" data-v="welcome">① 首次登录</a>
      <a data-p="flow" data-v="add">② 添加设备</a>
      <a data-p="flow" data-v="cmd">③ 执行并取消命令</a>
      <a data-p="flow" data-v="takeover">④ AI 操作中：提示与抢回</a>
      <a data-p="flow" data-v="preempt">⑤ 模拟本地用户动鼠标</a>
      <a data-p="flow" data-v="netdown">⑥ 断线与重连</a>
      <a data-p="flow" data-v="remote">⑦ 远程画面</a>
      <a data-p="flow" data-v="relay">⑧ 高延迟设备（中继）</a>
      <a data-p="flow" data-v="offline">⑨ 离线设备</a>
      <a data-p="flow" data-v="nonode">⑩ 未安装 DeskBridge 的设备</a>
      <a data-p="flow" data-v="external">⑪ 外部设备（权限受限）</a>
      <a data-p="flow" data-v="approve">⑫ 审批危险操作</a>
      <a data-p="flow" data-v="invite">⑬ 共享设备给别人</a>
      <a data-p="flow" data-v="agent">⑭ 调用被控端的 Agent</a>
      <a data-p="flow" data-v="palette">⑮ 命令面板（Ctrl K）</a>
    </div></div>
    <div class="note">这是不接后端的原型，所有数据都是示例。点击界面中的按钮、标签页和列表都有反应。</div>`;
}

function onProto(el) {
  const k = el.dataset.p, v = el.dataset.v;
  if (k === "toggleProto") { $("#proto").classList.toggle("closed"); return; }
  if (k === "flow") return FLOWS[v]();
  S[k] = v;
  if (k === "scenario" && v === "netdown") S.netTry = 1;
  if (k === "size") parseRoute();
  if (k === "scenario") parseRoute();
  render();
}

const FLOWS = {
  welcome: () => { S.onb.step = 0; S.scenario = "normal"; go("#/welcome"); },
  add: () => { go("#/devices"); setTimeout(A.addDevice, 0); },
  cmd: () => { S.cmd.status = "idle"; S.cmd.lines = []; go("#/devices/win-pc/term"); setTimeout(() => { A.runCmd(); toast("info", "命令开始执行", "运行几秒后点“取消”，看看取消后的状态"); }, 200); },
  takeover: () => { S.control = "ai"; go("#/devices/win-pc/overview"); setTimeout(() => toast("ai", "Claude Code 正在操作 win-pc 的桌面", "点提示条上的“停止”或“接管”"), 100); },
  preempt: () => { S.control = "ai"; go("#/devices/win-pc/overview"); setTimeout(A.simLocal, 600); },
  netdown: () => { S.scenario = "netdown"; S.netTry = 1; go("#/devices"); setTimeout(() => { if (netDown()) { S.netTry = 2; render(); } }, 1500); },
  remote: () => { go("#/devices/win-pc/overview"); setTimeout(A.openRemote, 0); },
  relay: () => go("#/devices/old-laptop/overview"),
  offline: () => go("#/devices/redmi-phone/overview"),
  nonode: () => go("#/devices/nas/overview"),
  external: () => go("#/devices/office-pc/overview"),
  approve: () => { S.appr.tab = "pending"; go("#/approvals"); },
  invite: () => { go("#/shared"); setTimeout(A.invite, 0); },
  agent: () => go("#/devices/win-pc/ai"),
  palette: () => A.openPalette(),
};

A.simLocal = () => {
  if (S.control !== "ai") return;
  S.control = "local";
  toast("warn", "本地用户开始使用 win-pc", "AI 的前台输入已被中断（preempted），3 秒无操作后自动继续");
  render();
  setTimeout(() => {
    if (S.control === "local") { S.control = "ai"; toast("ai", "本地用户已空闲 3 秒", "Claude Code 继续操作桌面"); render(); }
  }, 6000);
};

A.netRetry = () => {
  S.netTry += 1; render();
  setTimeout(() => { S.scenario = "normal"; toast("ok", "组网已恢复", "已重新连接，设备列表已刷新"); render(); }, 1200);
};

// ---------- 事件 ----------
document.addEventListener("click", ev => {
  const p = ev.target.closest("[data-p]");
  if (p) { onProto(p); return; }
  if (S.menu && !ev.target.closest(".menu")) { S.menu = null; render(); if (!ev.target.closest("[data-a]")) return; }
  if (S.pop && !ev.target.closest(".pop") && !ev.target.closest("[data-a^=pop]")) { S.pop = null; render(); if (!ev.target.closest("[data-a],[data-go]")) return; }
  const g = ev.target.closest("[data-go]");
  const a = ev.target.closest("[data-a]");
  // 最内层优先
  if (a && (!g || g.contains(a))) { ev.preventDefault(); A[a.dataset.a]?.(a, ev); return; }
  if (g) { ev.preventDefault(); S.palette = false; go(g.dataset.go); }
});

document.addEventListener("input", ev => {
  const el = ev.target.closest("[data-in]");
  if (!el) return;
  const k = el.dataset.in;
  if (k === "paletteQ") { S.paletteQ = el.value; render(); return; }
  const path = k.split(".");
  let o = S;
  for (let i = 0; i < path.length - 1; i++) o = o[path[i]];
  o[path.at(-1)] = el.value;
  if (el.dataset.rerender) render();
});

document.addEventListener("keydown", ev => {
  if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === "k") { ev.preventDefault(); A.openPalette(); return; }
  if (ev.key === "Escape") {
    if (S.palette) S.palette = false;
    else if (S.menu) S.menu = null;
    else if (S.pop) S.pop = null;
    else if (S.drawer) S.drawer = null;
    else if (S.modal) S.modal = null;
    else if (S.remote) S.remote = null;
    render();
    return;
  }
  if (ev.key === "Enter") {
    if (S.palette) { const it = $(".palette .it"); if (it) A.paletteGo(it); return; }
    if (ev.target.id === "cmdin") { ev.preventDefault(); A.runCmd(); }
  }
});

window.addEventListener("hashchange", () => { S.menu = null; S.pop = null; S.drawer = null; parseRoute(); render(); });
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => { if (S.mode === "system") render(); });

// 审批倒计时
setInterval(() => {
  S.appr.items.forEach(a => { if (a.wait > 0) a.wait -= 1; });
  document.querySelectorAll("[data-tmr]").forEach(el => {
    const a = S.appr.items.find(x => x.id === el.dataset.tmr);
    if (a) el.textContent = `${Math.floor(a.wait / 60)}:${String(a.wait % 60).padStart(2, "0")} 后自动拒绝`;
  });
  const rt = $("#runtime");
  if (rt && S.cmd.status === "run") rt.textContent = fmtSec(Date.now() - S.cmd.started);
}, 1000);

window.addEventListener("DOMContentLoaded", () => {
  const q = new URLSearchParams(location.search);
  ["theme", "mode", "size", "scenario", "control"].forEach(k => { if (q.get(k)) S[k] = q.get(k); });
  if (q.get("proto") === "closed") $("#proto").classList.add("closed");
  parseRoute();
  render();
  if (q.get("modal") && A[q.get("modal")]) A[q.get("modal")]();
});
