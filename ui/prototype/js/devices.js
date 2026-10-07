// 设备：首页卡片网格 → 设备页（概览、终端、文件、桌面、AI）。次要信息放进展开器、抽屉和弹出面板。

const TABS = [
  ["overview", "info", "概览"],
  ["term", "terminal", "终端"],
  ["files", "folder", "文件"],
  ["desk", "monitor", "桌面"],
  ["ai", "bot", "AI"],
];

// 设备对某个标签页的可用性：null 表示可用，否则返回 [原因类型, 说明]
function tabBlock(d, tab) {
  if (tab === "desk" && d.kind === "server") return ["nodesk", "这台设备没有图形桌面"];
  if (tab === "ai" && d.outdated) return ["old", "需要节点 0.3.0 以上"];
  if (d.group === "external") {
    const map = { term: "命令", files: "读文件", desk: "截图", ai: "Agent" };
    if (map[tab] && !d.perms.includes(map[tab])) return ["perm", `alice 没有授予“${map[tab]}”权限`];
  }
  return null;
}

function viewDevices() {
  return S.route.dev ? devicePage(devById(S.route.dev)) : devicesHome();
}

// =========================================================
// 首页
// =========================================================
function isActive(d) { return d.online && !d.noNode; }

function statusPill(d) {
  const [, , label, pc] = devState(d);
  if (d.self) return `<span class="pill line">本机</span>`;
  if (pc === "ok") return "";
  return `<span class="pill ${pc}">${pc === "ai" ? `<span class="pulse ai live"></span>` : ""}${label}</span>`;
}

function dcard(d) {
  const dim = !isActive(d) || (netDown() && !d.self) ? "dim" : "";
  let foot;
  if (!d.online) foot = `${ic("clock", "sm")}${d.lastSeen}在线`;
  else if (d.noNode) foot = `${ic("info", "sm")}在内网中，没有运行 DeskBridge`;
  else if (d.self) foot = `${ic("check", "sm")}可被其他设备和 AI 访问`;
  else if (d.group === "external") foot = `${ic("users", "sm")}来自 ${d.owner}`;
  else foot = `${ic("route", "sm")}${pathText(d)}`;
  const lat = isActive(d) && !d.self && !netDown() ? `<span class="lat">${latBars(d.latency)}${d.latency} 毫秒</span>` : "";
  const quick = isActive(d) && !d.self && !netDown() ? `<div class="quick">
      <button class="btn ghost icon sm" title="终端" data-go="#/devices/${d.id}/term">${ic("terminal")}</button>
      <button class="btn ghost icon sm" title="文件" data-go="#/devices/${d.id}/files">${ic("folder")}</button>
      ${d.kind !== "server" ? `<button class="btn ghost icon sm" title="桌面" data-go="#/devices/${d.id}/desk">${ic("monitor")}</button>` : ""}
    </div>` : "";
  return `<div class="dcard ${dim} ${d.self ? "self" : ""}" data-go="#/devices/${d.id}/overview">
    <div class="top"><div class="dic">${ic(kindIcon(d))}${dot(d)}</div>
      <div style="min-width:0"><div class="nm">${d.name}</div><div class="os">${d.os}</div></div>
      <span class="badge-r">${statusPill(d)}</span></div>
    <div class="foot">${foot}${lat}</div>${quick}
  </div>`;
}

function devicesHome() {
  const online = DEVICES.filter(d => isActive(d) && !d.self).length;
  const head = isPhone()
    ? `<div class="mtop"><h1>设备</h1><button class="btn ghost icon" data-a="openPalette">${ic("search")}</button><button class="btn primary icon" data-a="addDevice">${ic("plus")}</button></div>`
    : `<div class="v2-head"><div><h1>设备</h1><div class="sub">${netDown() ? "组网断开，状态可能不是最新的" : S.scenario === "empty" ? "只有本机" : `${online} 台在线 · 你和 AI 都可以操作它们`}</div></div>
        <div class="actions"><div class="search" style="width:220px;background:var(--surface)">${ic("search", "sm")}<input id="dsearch" placeholder="搜索设备" value="${esc(S.search)}" data-in="search" data-rerender="1"></div>
        <button class="btn primary" data-a="addDevice">${ic("plus")}添加设备</button></div></div>`;
  let body;
  if (S.scenario === "loading") {
    body = `<div class="dgrid">${Array.from({ length: 6 }, () => `<div class="dcard" style="cursor:default"><div class="top"><div class="dic sk" style="border:0"></div><div style="flex:1"><div class="sk" style="height:13px;width:60%"></div><div class="sk" style="height:10px;width:80%;margin-top:8px"></div></div></div><div class="sk" style="height:10px;width:50%"></div></div>`).join("")}</div>`;
  } else if (S.scenario === "error") {
    body = `<div class="empty"><div class="art bad">${ic("alert", "xl")}</div><h3>无法读取设备列表</h3><p>后台服务没有响应，正在重启它。</p><p><span class="errcode">ui_core_timeout</span></p><div class="btns"><button class="btn" data-a="noop" data-msg="已打开日志目录">打开日志</button><button class="btn primary" data-a="retryList">${ic("refresh")}重试</button></div></div>`;
  } else if (S.scenario === "empty") {
    body = `<div class="dgrid">${dcard(DEVICES[0])}</div>
      <div class="empty" style="padding:48px 24px"><div class="art accent">${ic("plus", "xl")}</div><h3>添加你的第一台设备</h3>
        <p>在另一台电脑、服务器或手机上安装 DeskBridge，用同一账号登录，它会自动出现在这里。</p>
        <div class="btns"><button class="btn primary lg" data-a="addDevice">${ic("plus")}添加设备</button></div></div>`;
  } else {
    const q = S.search.trim().toLowerCase();
    const match = d => !q || (d.name + d.ip + d.os).toLowerCase().includes(q);
    const mine = DEVICES.filter(d => d.group === "mine" && match(d));
    const active = mine.filter(isActive);
    const inactive = mine.filter(d => !isActive(d));
    const ext = DEVICES.filter(d => d.group === "external" && match(d));
    const open = S.exp.inactive || q;
    body = `<section>
        <div class="sec-h"><h2>我的设备</h2><span class="count">${active.length}</span></div>
        <div class="dgrid">${active.map(dcard).join("")}</div>
        ${inactive.length ? `<div style="margin-top:14px">${open
          ? `<div class="dgrid">${inactive.map(dcard).join("")}</div>${q ? "" : `<div class="collapsed-row" style="margin-top:14px" data-a="toggleExp" data-k="inactive">${ic("chevDown", "sm")}收起</div>`}`
          : `<div class="collapsed-row" data-a="toggleExp" data-k="inactive">${ic("chevRight", "sm")}还有 ${inactive.length} 台离线或未安装 DeskBridge 的设备</div>`}</div>` : ""}
      </section>
      ${ext.length ? `<section><div class="sec-h"><h2>外部设备</h2><span class="count">${ext.length}</span><span class="right"><button class="btn ghost sm" data-go="#/shared">管理共享</button></span></div>
        <div class="dgrid">${ext.map(dcard).join("")}</div></section>` : ""}
      ${!mine.length && !ext.length ? `<div class="empty"><div class="art">${ic("search", "xl")}</div><h3>没有匹配的设备</h3></div>` : ""}`;
  }
  return `<div class="v2-page" data-sk="home"><div class="v2-in">${head}${body}</div></div>`;
}

A.toggleExp = el => { const k = el.dataset.k; S.exp[k] = !S.exp[k]; render(); };
A.retryList = () => { S.scenario = "loading"; render(); setTimeout(() => { S.scenario = "normal"; parseRoute(); render(); toast("ok", "设备列表已刷新"); }, 1200); };

// =========================================================
// 设备页
// =========================================================
function canRemote(d) { return !netDown() && isActive(d) && !["server", "phone"].includes(d.kind) && !d.self && d.group !== "external"; }

function devicePage(d) {
  const [, , label, pc] = devState(d);
  const line = [d.os];
  if (isActive(d) && !netDown()) line.push(`<span class="mono">${d.ip}</span>`, d.self ? "本机" : `${pathText(d)} · ${d.latency} 毫秒`);
  if (!d.online) line.push(`${d.lastSeen}在线`);
  const head = `<div class="dhead">
      <div class="big">${ic(kindIcon(d))}</div>
      <div style="min-width:0"><h1>${d.name}${d.self ? `<span class="pill line">本机</span>` : `<span class="pill ${pc}">${pc === "ai" ? `<span class="pulse ai live"></span>` : pc === "ok" ? `<span class="pulse"></span>` : ""}${label}</span>`}</h1>
        <div class="line">${line.join(`<span class="faint">·</span>`)}</div></div>
      <div class="actions">
        ${canRemote(d) ? `<button class="btn primary" data-a="openRemote">${ic("cast")}远程画面</button>` : ""}
        ${isActive(d) && !netDown() ? `<button class="btn icon" data-a="devMenu" title="更多">${ic("more")}</button>` : ""}
      </div></div>`;
  const tabs = isActive(d) ? `<nav class="tabs2">${TABS.map(([k, icon, l]) => {
    const blk = tabBlock(d, k);
    return `<a class="${S.route.tab === k ? "on" : ""} ${blk ? "dim" : ""}" data-go="#/devices/${d.id}/${k}">${ic(blk && blk[0] === "perm" ? "lock" : icon)}${l}${k === "ai" ? `<span class="soon">规划中</span>` : ""}</a>`;
  }).join("")}</nav>` : "";
  const top = isPhone()
    ? `<div class="mtop"><button class="btn ghost icon" data-go="#/devices">${ic("chevLeft", "lg")}</button><h2 class="muted" style="font-size:14px;font-weight:500">设备</h2>
        ${canRemote(d) ? `<button class="btn ghost icon" data-a="openRemote">${ic("cast")}</button>` : ""}${isActive(d) && !netDown() ? `<button class="btn ghost icon" data-a="devMenu">${ic("more")}</button>` : ""}</div>`
    : "";
  return `${top}<div class="work">
    <div class="dtop">${isPhone() ? "" : `<a class="back" data-go="#/devices">${ic("chevLeft", "sm")}设备</a>`}${head}${statusBar(d)}${tabs}</div>
    <div class="work-in" data-sk="work-${S.route.tab}">${devicePane(d)}</div>
  </div>`;
}

// 只显示一条最重要的状态，点击展开说明
function statusBar(d) {
  if (netDown() || !isActive(d)) return "";
  let b = null;
  if (d.id === "win-pc") {
    b = {
      ai: { k: "ai", lead: `<span class="pulse ai live"></span>${avatar("claude", "sm")}`, t: `<b>Claude Code 正在操作桌面</b> · 2 分 14 秒`,
        btns: `<button class="btn sm" data-a="takeover">${ic("hand")}接管</button><button class="btn sm danger-soft" data-a="stopAI">${ic("stop")}停止</button>`,
        x: "来自 linux-server。本地用户动一下鼠标键盘，AI 会立即停下；也可以在这里接管或紧急停止。命令和文件操作不受控制权影响。" },
      local: { k: "info", lead: ic("mouse"), t: `<b>本地用户正在使用</b> · Claude Code 在排队`, btns: "",
        x: "AI 的鼠标键盘操作已暂停，本地停止操作 3 秒后自动继续。可以在“设置 → 桌面控制权”中修改这个时间。" },
      human: { k: "info", lead: ic("hand"), t: `<b>你正在远程操作桌面</b>`, btns: `<button class="btn sm" data-a="release">释放</button>`,
        x: "Claude Code 的鼠标键盘操作在排队。你 60 秒不操作会自动释放。" },
      paused: { k: "bad", lead: ic("pause"), t: `<b>被控端已暂停所有远程操作</b>`, btns: "",
        x: "本地用户在 13:58 按下了暂停。新的远程操作会被拒绝，正在运行的后台命令继续。只能在被控端解除。" },
    }[S.control] || null;
  }
  if (!b && d.path === "relay") b = { k: "warn", lead: ic("route"), t: `<b>经中继连接，延迟较高</b> · 远程画面不可用`, btns: "",
    x: `当前经${d.relay}的中继连接，延迟 ${d.latency} 毫秒。命令和文件可以正常使用；远程画面暂时只支持局域网直连。<a data-a="noop" data-msg="正在重新探测直连路径…" style="text-decoration:underline;cursor:pointer">重新探测</a>` };
  if (!b && d.group === "external") b = { k: "info", lead: ic("users"), t: `<b>alice 共享给你的设备</b> · ${d.perms.join("、")}`, btns: "",
    x: "你只能使用 alice 授予的权限。操作会同时记录在你和 alice 的操作记录中。" };
  if (!b) return "";
  const open = S.exp["bar-" + d.id];
  return `<div class="statusbar ${b.k}" data-a="toggleExp" data-k="bar-${d.id}">${b.lead}<span class="t">${b.t}</span>
      <span class="more">${open ? "收起" : "详情"}${ic(open ? "chevDown" : "chevRight", "sm")}</span>${b.btns ? `<span class="btns">${b.btns}</span>` : ""}</div>
    ${open ? `<div class="statusbar-x">${b.x}</div>` : ""}`;
}

function devicePane(d) {
  if (netDown() && !d.self) {
    if (S.route.tab === "overview" && isActive(d)) return `<div class="banner warn">${ic("clock")}<div class="t"><b>显示的是 2 分钟前的信息</b> <span class="d">组网恢复后自动刷新。</span></div></div>` + paneOverview(d);
    return `<div class="empty"><div class="art warn">${ic("wifiOff", "xl")}</div><h3>暂时无法连接 ${d.name}</h3><p>组网断开了，恢复后会自动重新连接。</p><div class="btns"><button class="btn" data-a="netRetry">${ic("refresh")}立即重试</button></div></div>`;
  }
  if (!d.online) return paneOffline(d);
  if (d.noNode) return paneNoNode(d);
  const blk = tabBlock(d, S.route.tab);
  if (blk) {
    return `<div class="empty"><div class="art">${ic(blk[0] === "perm" ? "lock" : "monitor", "xl")}</div><h3>${blk[0] === "perm" ? "没有权限" : "不可用"}</h3><p>${blk[1]}。</p>${blk[0] === "perm" ? `<div class="btns"><button class="btn" data-a="noop" data-msg="已向 alice 发送权限申请">向 alice 申请</button></div>` : ""}</div>`;
  }
  switch (S.route.tab) {
    case "term": return paneTerm(d);
    case "files": return paneFiles(d);
    case "desk": return paneDesk(d);
    case "ai": return paneAI(d);
    default: return paneOverview(d);
  }
}

function paneOffline(d) {
  return `<div class="empty"><div class="art">${ic(kindIcon(d), "xl")}</div><h3>${d.name} 离线</h3>
    <p>最后在线：${d.lastSeen}。${d.controllerOnly ? "这台手机目前只作为控制端；安卓被控端还在规划中。" : "设备开机并联网后会自动重新连接。"}</p>
    <div class="btns"><button class="btn" data-go="#/activity">查看操作记录</button><button class="btn ghost" data-a="noop" data-msg="已从列表中隐藏 ${d.name}">从列表隐藏</button></div></div>`;
}

function paneNoNode(d) {
  return `<div class="empty" style="padding-bottom:12px"><div class="art">${ic("hdd", "xl")}</div><h3>${d.name} 没有运行 DeskBridge</h3>
      <p>它在你的内网中（${d.ip}），但没有安装或没有启动 DeskBridge，所以还不能操作。</p></div>
    <div style="max-width:560px;width:100%;margin:0 auto" class="col">
      <div class="snippet"><div class="codebox">curl -fsSL https://example.com/deskbridge/install.sh | sh\ndeskbridge login</div><button class="btn sm" data-a="noop" data-msg="已复制安装命令">${ic("copy")}复制</button></div>
      <div class="small muted" style="text-align:center">在 ${d.name} 上运行，然后用同一账号登录。<a data-a="noop" data-msg="已隐藏" style="cursor:pointer;text-decoration:underline">不再显示这台设备</a></div>
    </div>`;
}

// ---------- 概览 ----------
function sparkSvg(arr) {
  const max = Math.max(...arr) * 1.15, min = Math.min(...arr) * .85;
  const pts = arr.map((v, i) => `${(i / (arr.length - 1)) * 300},${44 - ((v - min) / (max - min)) * 40}`).join(" ");
  return `<svg class="spark" viewBox="0 0 300 46" preserveAspectRatio="none"><polyline points="${pts}" fill="none" stroke="var(--accent)" stroke-width="1.8" vector-effect="non-scaling-stroke"/><polyline points="0,46 ${pts} 300,46" fill="var(--accent-soft)" stroke="none"/></svg>`;
}

function actVerb(a) {
  return { "proc.exec": "执行命令", "input.click": "点击", "control.preempt": "抢回桌面", "fs.write": "上传文件", "agent.run": "调用 Agent", "screen.capture": "截图", "ui.tree": "读取界面元素", "fs.read": "读取文件" }[a.tool] || a.tool;
}

function actItem(a) {
  const i = ACTIVITY.indexOf(a);
  const [stl, stc] = ST_LABEL[a.st];
  return `<div class="li" data-a="actDrawer" data-i="${i}">${avatar(a.actor)}
    <div class="b"><div class="t"><b>${ACTORS[a.actor].name}</b> <span class="muted">在 ${a.dev} 上${actVerb(a)}</span></div><div class="d">${esc(a.sum)}</div></div>
    <div class="r">${a.st === "ok" ? "" : `<span class="pill ${stc}">${stl}</span>`}<span>${a.t}</span></div></div>`;
}

function paneOverview(d) {
  const tile = (go, icon, t, desc, off) => `<div class="tile ${off ? "off" : ""}" ${go.startsWith("#") ? `data-go="${go}"` : `data-a="${go}"`}><span class="ti">${ic(icon)}</span><div><div class="tt">${t}</div><div class="td">${desc}</div></div></div>`;
  const ctl = { none: ["空闲", "本地用户空闲 2 分钟"], ai: ["Claude Code", "AI 正在使用鼠标键盘"], local: ["本地用户", "AI 在排队"], human: ["你", "正在远程操作"], paused: ["已暂停", "本地用户暂停了远程操作"] }[S.control];
  const recent = ACTIVITY.filter(a => a.dev === d.id).slice(0, 3);
  const caps = [["terminal", "命令", true], ["folder", "文件", true], ["image", "截图", d.kind !== "server"], ["layers", "界面元素", d.kind !== "server" && !d.outdated], ["mouse", "鼠标键盘", d.kind !== "server"], ["bot", "Agent", !d.outdated && d.kind !== "server"]];
  return `
    <div class="tiles">
      ${tile(`#/devices/${d.id}/term`, "terminal", "终端", "执行命令", tabBlock(d, "term"))}
      ${tile(`#/devices/${d.id}/files`, "folder", "文件", "浏览和传输", tabBlock(d, "files"))}
      ${tile(`#/devices/${d.id}/desk`, "image", "截图", "查看屏幕", tabBlock(d, "desk"))}
      ${tile("openRemote", "cast", "远程画面", d.path === "lan" ? "局域网可用" : "仅局域网", !canRemote(d))}
    </div>
    <div class="stats">
      <div class="stat"><div class="l">${ic("route", "sm")}连接</div><div class="v">${d.self ? "本机" : `${d.latency} 毫秒`}</div><div class="s">${pathText(d)}${d.latency >= 150 ? " · 波动较大" : " · 稳定"}</div></div>
      <div class="stat"><div class="l">${ic("hand", "sm")}桌面</div><div class="v">${d.id === "win-pc" ? ctl[0] : d.kind === "server" ? "无桌面" : "空闲"}</div><div class="s">${d.id === "win-pc" ? ctl[1] : d.session || "—"}</div></div>
      <div class="stat"><div class="l">${ic("cpu", "sm")}系统</div><div class="v">${d.os}</div><div class="s">节点 ${d.version}${d.outdated ? " · 可更新" : ""}</div></div>
    </div>
    <section><div class="sec-h"><h2>最近操作</h2><span class="right"><button class="btn ghost sm" data-a="actForDev" data-dev="${d.id}">全部${ic("chevRight", "sm")}</button></span></div>
      ${recent.length ? `<div class="list2">${recent.map(actItem).join("")}</div>` : `<div class="muted small">暂无操作</div>`}</section>
    ${expander("info-" + d.id, "info", "设备详情", "系统、连接、能力和节点信息", `
      ${d.merged ? `<div class="banner info">${ic("info")}<div class="t">${d.merged}。</div><div class="btns"><button class="btn sm" data-a="noop" data-msg="已拆分为两个设备">拆分显示</button></div></div>` : ""}
      ${d.outdated ? `<div class="banner info">${ic("info")}<div class="t">节点版本 ${d.version} 较旧，界面元素和 Agent 需要 0.3.0 以上。</div><div class="btns"><button class="btn sm" data-a="noop" data-msg="已发送更新请求">远程更新</button></div></div>` : ""}
      <div class="ov">
        <div><div class="small muted" style="margin-bottom:8px">延迟（最近 10 分钟）</div>${d.spark ? sparkSvg(d.spark) : ""}</div>
        <dl class="kv"><dt>内网地址</dt><dd class="mono">${d.ip}</dd><dt>桌面会话</dt><dd>${d.session || "—"}</dd><dt>显示器</dt><dd>${d.displays ? d.displays + " 个" : "无"}</dd><dt>运行方式</dt><dd>${d.runMode || "—"}</dd></dl>
      </div>
      <div><div class="small muted" style="margin-bottom:8px">可用能力（由节点的能力报告决定）</div><div class="chips">${caps.map(([i, l, ok]) => `<span class="tag" style="${ok ? "" : "opacity:.5;text-decoration:line-through"}">${ic(i, "sm")}${l}</span>`).join("")}</div></div>
    `)}`;
}

function expander(key, icon, title, desc, body) {
  const open = S.exp[key];
  return `<div class="expander ${open ? "open" : ""}"><div class="eh" data-a="toggleExp" data-k="${key}">${ic(icon)}<div><div class="t">${title}</div><div class="d">${desc}</div></div><span class="chev">${ic("chevDown")}</span></div>${open ? `<div class="eb">${body}</div>` : ""}</div>`;
}

A.actForDev = el => { S.act.dev = el.dataset.dev; go("#/activity"); };

// ---------- 终端 ----------
function termHTML() {
  const c = S.cmd;
  const st = {
    idle: `<span class="st faint" style="font-weight:500">就绪</span>`,
    run: `<span class="st"><span class="spin"></span>运行中</span><span id="runtime">${fmtSec(Date.now() - c.started)}</span>`,
    ok: `<span class="st" style="color:var(--ok)">${ic("check", "sm")}完成</span><span>退出码 0 · ${c.dur}</span>`,
    cancel: `<span class="st" style="color:var(--muted)">${ic("stop", "sm")}已取消</span><span>${c.dur}</span>`,
  }[c.status];
  const lines = c.lines.map(([k, t]) => {
    const html = esc(t).replace(/&lt;hl&gt;(.*?)&lt;\/hl&gt;/g, '<span class="hl">$1</span>');
    return k ? `<span class="${k}">${html}</span>` : html;
  }).join("\n");
  const head = c.status === "idle" ? "" : `<span class="dim">$ ${esc(c.ran || c.text)}</span>\n`;
  const tail = c.status === "run" ? ` <span class="cursor"></span>` : c.status === "cancel" ? `\n<span class="warn">^C 已取消，进程树已结束</span>` : "";
  return `<div class="out" id="out">${c.status === "idle" ? `<span class="dim">输入命令后按回车执行。输出会实时显示，Windows 上的中文会自动转成 UTF-8。</span>` : head + lines + tail}</div>`;
}

function termBar() {
  const c = S.cmd;
  return `<div class="bar" id="tbar">${{ idle: `<span class="st faint" style="font-weight:500">就绪</span>`, run: `<span class="st"><span class="spin"></span>运行中</span><span id="runtime">${fmtSec(Date.now() - c.started)}</span>`, ok: `<span class="st" style="color:var(--ok)">${ic("check", "sm")}完成</span><span>退出码 0 · ${c.dur}</span>`, cancel: `<span class="st" style="color:var(--muted)">${ic("stop", "sm")}已取消</span><span>${c.dur}</span>` }[c.status]}
    <span class="tools">
      <button class="btn ghost sm" data-a="popHistory">${ic("history")}历史</button>
      <button class="btn ghost sm" data-a="popOptions">${ic("sliders")}选项</button>
      <button class="btn ghost icon sm" data-a="noop" data-msg="已复制输出" title="复制输出">${ic("copy")}</button>
      ${c.status !== "idle" && c.status !== "run" ? `<button class="btn ghost icon sm" data-a="clearCmd" title="清空">${ic("trash")}</button>` : ""}
    </span></div>`;
}

function paneTerm(d) {
  const c = S.cmd;
  const shells = d.os.startsWith("Windows") ? ["PowerShell 7", "命令提示符"] : ["bash", "sh"];
  if (!shells.includes(c.shell)) c.shell = shells[0];
  return `<div class="term2" id="term">
    ${termBar()}
    ${termHTML()}
    <div class="inrow">
      <span class="sh" data-a="cycleShell" data-v="${shells.join("|")}" title="切换命令行">${c.shell}${ic("chevDown", "sm")}</span>
      <span class="prompt">›</span>
      <input id="cmdin" value="${esc(c.text)}" data-in="cmd.text" placeholder="输入命令，回车执行" autocomplete="off" spellcheck="false">
      ${c.status === "run" ? `<button class="btn danger-soft" data-a="cancelCmd">${ic("stop")}取消</button>` : `<button class="btn primary" data-a="runCmd">${ic("play")}执行</button>`}
    </div>
  </div>`;
}

let cmdTimer = null;
function updateTerm() {
  const o = $("#out"); if (o) { o.outerHTML = termHTML(); const n = $("#out"); n.scrollTop = n.scrollHeight; }
}
A.runCmd = () => {
  if (netDown()) return toast("bad", "无法执行", "组网已断开");
  if (S.control === "paused") return toast("bad", "被拒绝", "被控端已暂停远程操作");
  const c = S.cmd;
  if (!c.text.trim()) return;
  clearInterval(cmdTimer);
  const src = /ping/i.test(c.text) ? CMD_OUTPUTS.ping : CMD_OUTPUTS.default;
  Object.assign(c, { status: "run", lines: [], started: Date.now(), ran: c.text });
  render();
  let i = 0;
  cmdTimer = setInterval(() => {
    if (c.status !== "run") return clearInterval(cmdTimer);
    c.lines.push(src[i++]);
    if (i >= src.length) { clearInterval(cmdTimer); c.status = "ok"; c.dur = fmtSec(Date.now() - c.started); render(); return; }
    updateTerm();
  }, 450);
};
A.cancelCmd = () => {
  clearInterval(cmdTimer);
  const c = S.cmd;
  c.status = "cancel"; c.dur = fmtSec(Date.now() - c.started);
  toast("ok", "已取消", "命令及其子进程已结束");
  render();
};
A.clearCmd = () => { S.cmd.status = "idle"; S.cmd.lines = []; render(); };
A.fillCmd = el => { S.cmd.text = el.dataset.c; S.pop = null; render(); $("#cmdin")?.focus(); };
A.cycleShell = el => { const s = el.dataset.v.split("|"); S.cmd.shell = s[(s.indexOf(S.cmd.shell) + 1) % s.length]; render(); };
A.optToggle = el => { S.cmd[el.dataset.k] = !S.cmd[el.dataset.k]; render(); };
A.popHistory = el => openPop(el, "history");
A.popOptions = el => openPop(el, "options");

POPS.history = () => `<div class="ph2">最近执行</div>${RECENT_RUNS.map(r => `<div class="pi" data-a="fillCmd" data-c="${esc(r.c)}"><span class="pill ${r.st === "ok" ? "ok" : r.st === "err" ? "bad" : "off"}">${r.st === "ok" ? "成功" : r.st === "err" ? `退出码 ${r.code}` : "已取消"}</span><span class="c">${esc(r.c)}</span><span class="w">${r.w}</span></div>`).join("")}`;
POPS.options = () => {
  const c = S.cmd;
  return `<div class="ph2">执行选项</div>
    <div class="pi" data-a="optToggle" data-k="timeout"><span class="check ${c.timeout ? "on" : ""}"></span>超时 120 秒后结束</div>
    <div class="pi" data-a="optToggle" data-k="admin"><span class="check ${c.admin ? "on" : ""}"></span>以管理员身份（需要被控端确认）</div>
    <div class="divider"></div>
    <div class="pi" style="cursor:default">${ic("folder", "sm")}<span class="muted">工作目录</span><span class="c" style="text-align:right">D:\\data\\projects</span></div>`;
};

// ---------- 文件 ----------
function paneFiles(d) {
  const f = S.fs;
  const parts = f.path.split("\\").filter(Boolean);
  const crumbs = parts.map((p, i) => {
    const path = parts.slice(0, i + 1).join("\\") + (i === 0 ? "\\" : "");
    return `<a data-a="cd" data-p="${esc(path)}">${esc(p)}</a>`;
  }).join(`<span class="sep">${ic("chevRight", "sm")}</span>`);
  const list = FILES[f.path];
  let body;
  if (f.path === "D:\\data\\private") {
    body = `<div class="empty"><div class="art bad">${ic("lock", "xl")}</div><h3>拒绝访问</h3><p>节点以普通用户身份运行，没有读取这个文件夹的权限。</p><p><span class="errcode">permission_denied</span></p><div class="btns"><button class="btn" data-a="cd" data-p="D:\\data">返回上一级</button></div></div>`;
  } else if (!list) {
    body = `<div class="empty"><div class="art">${ic("folder", "xl")}</div><h3>空文件夹</h3><p>把文件拖到这里即可上传。</p></div>`;
  } else {
    const rows = list.concat(f.path === "D:\\data" ? [{ n: "private", d: true, m: "—" }] : []);
    body = `<div class="list2">${rows.map(x => `<div class="li" data-a="${x.d ? "cd" : "fileDrawer"}" data-p="${esc(f.path + "\\" + x.n)}" data-n="${esc(x.n)}">
        ${x.d ? `<span class="fname"><span class="folder">${ic("folder", "lg")}</span></span>` : `<span class="fname">${ic(x.text ? "fileText" : "file", "lg")}</span>`}
        <div class="b"><div class="t">${esc(x.n)}</div></div>
        <div class="r"><span class="hide-m">${x.m}</span><span style="width:64px;text-align:right">${x.d ? "" : x.s}</span>${x.d ? ic("chevRight", "sm") : ""}</div></div>`).join("")}</div>`;
  }
  const up = f.upload;
  return `<div class="row" style="flex-wrap:wrap">
      <button class="btn ghost icon sm" data-a="cdUp" title="上一级">${ic("chevLeft")}</button>
      <div class="crumbs">${crumbs}</div><span class="spacer"></span>
      <button class="btn ghost icon sm" data-a="noop" data-msg="已刷新" title="刷新">${ic("refresh")}</button>
      <button class="btn" data-a="upload">${ic("upload")}上传</button>
    </div>
    ${up ? `<div class="card" style="padding:12px 16px"><div class="row small"><span>${ic("upload", "sm")}上传 <b>${up.n}</b></span><span class="spacer"></span><span class="muted">${up.p}%</span><button class="btn ghost sm" data-a="cancelUpload">取消</button></div><div class="progress" style="margin-top:8px"><i style="width:${up.p}%"></i></div></div>` : ""}
    ${body}`;
}
A.cd = el => { S.fs.path = el.dataset.p; render(); };
A.cdUp = () => { const p = S.fs.path.split("\\").filter(Boolean); if (p.length > 1) { p.pop(); S.fs.path = p.join("\\") + (p.length === 1 ? "\\" : ""); render(); } };
A.fileDrawer = el => { S.drawer = { type: "file", n: el.dataset.n }; render(); };
let upTimer = null;
A.upload = () => {
  S.fs.upload = { n: "报告-2026Q3.xlsx", p: 0 }; render();
  clearInterval(upTimer);
  upTimer = setInterval(() => {
    if (!S.fs.upload) return clearInterval(upTimer);
    S.fs.upload.p = Math.min(100, S.fs.upload.p + 17);
    if (S.fs.upload.p >= 100) { clearInterval(upTimer); S.fs.upload = null; toast("ok", "上传完成", "报告-2026Q3.xlsx → " + S.fs.path); }
    render();
  }, 400);
};
A.cancelUpload = () => { clearInterval(upTimer); S.fs.upload = null; toast("ok", "已取消上传"); render(); };

DRAWERS.file = () => {
  const x = (FILES[S.fs.path] || []).find(f => f.n === S.drawer.n) || {};
  return drawer({
    title: esc(x.n), sub: `${x.s || ""} · 修改于 ${x.m || ""}`,
    body: x.text ? `<div class="codebox" style="font-size:12.5px;line-height:1.7">${esc(x.text)}</div>` : `<div class="empty"><div class="art">${ic("file", "xl")}</div><p>这种文件不支持预览。</p></div>`,
    foot: `<button class="btn primary" data-a="noop" data-msg="已保存到 下载 文件夹">${ic("download")}下载</button><button class="btn" data-a="noop" data-msg="已复制路径">${ic("copy")}复制路径</button><span class="spacer"></span><button class="btn ghost danger-soft icon" data-a="noop" data-msg="删除需要确认（原型未实现）">${ic("trash")}</button>`,
  });
};

// ---------- 桌面（截图 + 界面元素） ----------
function mouseSvg(x, y) {
  return `<svg class="mouse" viewBox="0 0 12 18" style="left:${x}%;top:${y}%"><path d="M1 1v14.5l3.6-3.4 2.6 5.6 2.3-1.1-2.6-5.4H11.5z" fill="#fff" stroke="#111" stroke-width="1" stroke-linejoin="round"/></svg>`;
}

function desktopArt({ ai = false, small = false } = {}) {
  return `<div class="desk-wrap"><div class="desk ${small ? "small" : ""}">
    <div class="win" style="left:6%;top:8%;width:48%;height:58%"><div class="tb">无标题 - 记事本</div><div class="ln" style="width:70%"></div><div class="ln" style="width:55%"></div><div class="ln" style="width:62%"></div><div class="ln" style="width:30%"></div></div>
    <div class="win dark" style="left:40%;top:30%;width:52%;height:56%"><div class="tb">PowerShell</div><div class="ln b" style="width:40%"></div><div class="ln" style="width:75%"></div><div class="ln" style="width:68%"></div><div class="ln g" style="width:35%"></div><div class="ln" style="width:58%"></div></div>
    ${ai ? `<div class="aiframe"></div><div class="aitip">AI 正在操作（来自 linux-server）</div>${mouseSvg(30, 40)}` : mouseSvg(70, 62)}
    <div class="taskbar"><i></i><i></i><i></i><i></i><i></i></div>
  </div></div>`;
}

function paneDesk(d) {
  const mode = S.deskMode;
  const seg = `<div class="segmented"><button class="${mode === "shot" ? "on" : ""}" data-a="deskMode" data-v="shot">${ic("image", "sm")}截图</button><button class="${mode === "uia" ? "on" : ""}" data-a="deskMode" data-v="uia">${ic("layers", "sm")}界面元素</button></div>`;
  if (mode === "uia") return paneUia(d, seg);
  const s = S.shot, cur = s.list[s.sel];
  return `<div class="row" style="flex-wrap:wrap">${seg}<span class="spacer"></span>
      <select class="select" style="width:130px;height:30px" data-in="shot.mon"><option value="1">显示器 1</option><option value="2">显示器 2</option><option value="all">全部显示器</option></select>
      <button class="btn primary" data-a="takeShot">${ic("image")}截图</button></div>
    <div class="card" style="padding:16px;flex:1;min-height:260px;display:flex;flex-direction:column;gap:12px">
      <div class="shot-canvas" style="background:transparent;padding:0">${s.taking ? `<div class="col" style="align-items:center;color:var(--muted)"><span class="spin lg"></span>正在截图…</div>` : desktopArt({ ai: S.control === "ai" && cur.ai })}</div>
      <div class="row small muted">${cur.t} · ${cur.mon} · 由 ${cur.by} 截取<span class="spacer"></span>
        <button class="btn ghost sm" data-a="noop" data-msg="已复制到剪贴板">${ic("copy")}复制</button><button class="btn ghost sm" data-a="noop" data-msg="已保存">${ic("download")}保存</button>
        <button class="btn ghost sm" data-a="toggleExp" data-k="shotHist">${ic("history")}历史 ${s.list.length}${ic(S.exp.shotHist ? "chevDown" : "chevRight", "sm")}</button></div>
      ${S.exp.shotHist ? `<div class="row" style="gap:10px;overflow-x:auto">${s.list.map((x, i) => `<div class="thumb ${i === s.sel ? "on" : ""}" style="width:150px;flex:none" data-a="selShot" data-i="${i}">${desktopArt({ small: true, ai: x.ai })}<div class="cap2">${x.t} · ${x.by}</div></div>`).join("")}</div>` : ""}
    </div>`;
}
A.deskMode = el => { S.deskMode = el.dataset.v; render(); };
A.selShot = el => { S.shot.sel = +el.dataset.i; render(); };
A.takeShot = () => {
  S.shot.taking = true; render();
  setTimeout(() => {
    S.shot.taking = false;
    S.shot.list.unshift({ t: new Date().toTimeString().slice(0, 8), by: "示例用户", mon: S.shot.mon === "all" ? "全部显示器" : "显示器 " + S.shot.mon });
    S.shot.sel = 0; render();
  }, 900);
};

function treeHTML(n, depth = 0) {
  const has = n.kids && n.kids.length;
  const open = S.uia.open[n.id] ?? n.open;
  return `<div class="node ${S.uia.sel === n.id ? "on" : ""}" style="padding-left:${8 + depth * 18}px" data-a="uiaSel" data-id="${n.id}">
      <span class="tw" ${has ? `data-a="uiaToggle" data-id="${n.id}"` : ""}>${has ? ic(open ? "chevDown" : "chevRight", "sm") : ""}</span>
      <span class="ty">${n.ty}</span><span class="nm">${n.nm ? `“${esc(n.nm)}”` : ""}</span></div>` +
    (has && open ? n.kids.map(k => treeHTML(k, depth + 1)).join("") : "");
}

function paneUia(d, seg) {
  const u = S.uia;
  const win = UI_WINDOWS.find(w => w.id === u.win);
  let center;
  if (win.elevated) center = `<div class="empty"><div class="art warn">${ic("shieldAlert", "xl")}</div><h3>窗口权限高于节点</h3><p>“${win.n}”以管理员身份运行，普通权限的节点不能读取它。</p><p><span class="errcode">elevated_target</span></p></div>`;
  else if (u.win !== "np") center = `<div class="empty"><div class="art">${ic("layers", "xl")}</div><p>原型只准备了“记事本”的示例数据。</p></div>`;
  else center = `<div class="tree" style="padding:8px">${treeHTML(UI_TREE)}</div>`;
  return `<div class="row" style="flex-wrap:wrap">${seg}<span class="spacer"></span>
      <select class="select" style="width:220px;height:30px" data-in="uia.win" data-rerender="1">${UI_WINDOWS.map(w => `<option value="${w.id}" ${w.id === u.win ? "selected" : ""}>${w.elevated ? "🛡 " : ""}${esc(w.n)}</option>`).join("")}</select>
      <button class="btn ghost icon" data-a="noop" data-msg="已刷新" title="刷新">${ic("refresh")}</button></div>
    <div class="card" style="flex:1;min-height:260px;overflow:auto" data-sk="tree">${center}</div>
    <div class="small muted">点击元素查看属性和可执行的操作。后台操作不抢鼠标键盘，不需要控制权。</div>`;
}
A.uiaSel = (el, ev) => {
  const tw = ev.target.closest("[data-a=uiaToggle]");
  if (tw) return A.uiaToggle(tw);
  S.uia.sel = el.dataset.id; S.drawer = { type: "uia" }; render();
};
A.uiaToggle = el => { const id = el.dataset.id; const n = findNode(UI_TREE, id); S.uia.open[id] = !(S.uia.open[id] ?? n.open); render(); };
function findNode(n, id) { if (n.id === id) return n; for (const k of n.kids || []) { const r = findNode(k, id); if (r) return r; } return null; }

DRAWERS.uia = () => {
  const n = findNode(UI_TREE, S.uia.sel);
  const p = UI_PROPS[S.uia.sel] || { ty: n.ty, nm: n.nm || "—", aid: "—", cls: n.ty, rect: "—", pat: [], en: true };
  return drawer({
    title: `${p.ty} ${p.nm && p.nm !== "—" ? `“${esc(p.nm)}”` : ""}`, sub: "无标题 - 记事本",
    body: `<dl class="kv" style="grid-template-columns:90px 1fr;gap:12px"><dt>自动化 ID</dt><dd class="mono">${p.aid}</dd><dt>类名</dt><dd class="mono">${p.cls}</dd><dt>位置</dt><dd class="mono">${p.rect}</dd><dt>可用</dt><dd>${p.en ? "是" : "否"}</dd></dl>
      <div><div class="small muted" style="margin-bottom:8px">支持的操作</div><div class="chips">${p.pat.map(x => `<span class="tag">${x}</span>`).join("") || `<span class="faint small">无</span>`}</div></div>
      <div class="col"><button class="btn" data-a="noop" data-msg="已在被控端屏幕上高亮 2 秒">${ic("target")}在屏幕上高亮</button>
        <button class="btn" data-a="noop" data-msg="已执行（后台操作，不移动鼠标）">${ic("play")}点击（后台）</button>
        <button class="btn" data-a="noop" data-msg="已复制选择器">${ic("copy")}复制选择器</button></div>`,
  });
};

// ---------- AI（规划中） ----------
const AGENT_STEPS = [
  ["ai", "sparkles", "理解任务", "读取项目结构，准备编译"],
  ["run", "terminal", "执行命令", "<code>go build ./...</code>"],
  ["run", "alert", "发现 2 个编译错误", "internal/proto/codec.go:42、cmd/node/main.go:88"],
  ["run", "edit", "修改文件", "<code>internal/proto/codec.go</code>（+6 −2）"],
  ["run", "edit", "修改文件", "<code>cmd/node/main.go</code>（+1 −1）"],
  ["run", "terminal", "重新编译", "<code>go build ./...</code> 通过"],
  ["ok", "check", "完成", "修改了 2 个文件，编译通过。用时 1 分 48 秒"],
];

function paneAI(d) {
  const g = S.agent;
  const steps = g.steps.map((s, i) => {
    const last = i === g.steps.length - 1 && g.running;
    return `<div class="step"><span class="si ${last ? "run" : s[0] === "ai" ? "ai" : "ok"}">${last ? `<span class="spin" style="width:10px;height:10px"></span>` : ic(s[1], "sm")}</span><div class="sb"><div>${s[2]}</div><div class="sd">${s[3]}</div></div></div>`;
  }).join("");
  return `<div class="small muted row">${ic("info", "sm")}规划中的功能（任务 22）：把任务交给这台设备上已有的 Agent，使用它的 MCP 服务器和 Skill。</div>
    <div class="card"><div class="card-b col" style="gap:14px;padding:var(--cp)">
      <div class="row"><span class="muted small">交给</span><div class="segmented">${AGENTS.filter(a => a.ok).map(a => `<button class="${g.who === a.n ? "on" : ""}" data-a="agentWho" data-v="${a.n}">${a.n}</button>`).join("")}</div></div>
      <textarea class="textarea" rows="3" id="agentin" data-in="agent.prompt">${esc(g.prompt)}</textarea>
      <div class="row" style="flex-wrap:wrap"><span class="tag mono">${ic("folder", "sm")}D:\\data\\projects\\deskbridge</span><span class="tag">${ic("shield", "sm")}写入前询问我</span><span class="spacer"></span>
        ${g.running ? `<button class="btn danger-soft" data-a="agentStop">${ic("stop")}取消任务</button>` : `<button class="btn primary" data-a="agentRun">${ic("send")}交给 ${g.who}</button>`}</div>
    </div></div>
    ${g.steps.length ? `<section><div class="sec-h"><h2>任务过程</h2>${g.running ? `<span class="pill info">进行中</span>` : `<span class="pill ok">已完成</span>`}<span class="right small faint">断线后可以回来继续查看</span></div><div class="card"><div class="steps" data-sk="steps">${steps}</div></div></section>` : ""}
    ${expander("aicap", "box", "这台设备上的 AI 能力", `${AGENTS.filter(a => a.ok).length} 个 Agent · ${MCP_SERVERS.length} 个 MCP 服务器 · ${SKILLS.length} 个 Skill`, `
      <div class="list2">${AGENTS.map(a => `<div class="li" style="cursor:default"><span class="avatar sm ${a.avc}">${a.av}</span><div class="b"><div class="t">${a.n}</div><div class="d">${a.v || "—"}</div></div><span class="pill ${a.ok ? "ok" : "off"}">${a.st}</span></div>`).join("")}</div>
      <div class="list2">${MCP_SERVERS.map(m => `<div class="li" style="cursor:default">${ic("plug")}<div class="b"><div class="t mono">${m.n}</div><div class="d">${m.tools} 个工具</div></div><span class="pill ${m.ok ? "ok" : "off"}">${m.st}</span></div>`).join("")}</div>
      <div class="chips">${SKILLS.map(s => `<span class="tag mono">${s}</span>`).join("")}</div>`)}`;
}
let agentTimer = null;
A.agentWho = el => { S.agent.who = el.dataset.v; render(); };
A.agentRun = () => {
  const g = S.agent; g.running = true; g.steps = [AGENT_STEPS[0]]; render();
  let i = 1;
  clearInterval(agentTimer);
  agentTimer = setInterval(() => {
    if (!g.running) return clearInterval(agentTimer);
    g.steps.push(AGENT_STEPS[i++]);
    if (i >= AGENT_STEPS.length) { g.running = false; clearInterval(agentTimer); toast("ok", `${g.who} 完成了任务`, "修改了 2 个文件，编译通过"); }
    render();
  }, 900);
};
A.agentStop = () => { S.agent.running = false; S.agent.steps.push(["ok", "stop", "已取消", "已通知被控端结束 Agent 会话"]); render(); };

// =========================================================
// 远程画面
// =========================================================
function viewRemote() {
  const d = devById(S.remote);
  const ctl = S.control;
  const ctlText = { ai: `<span class="pill ai"><span class="pulse ai live"></span>AI 正在操作 · 你在观看</span>`, human: `<span class="pill info">${ic("hand", "sm")}你正在控制</span>`, local: `<span class="pill info">本地用户正在使用</span>`, none: `<span class="pill off">只观看</span>`, paused: `<span class="pill bad">被控端已暂停</span>` }[ctl];
  const ctlBtn = ctl === "human" ? `<button class="btn sm" data-a="release">释放控制权</button>` : ctl === "paused" || ctl === "local" ? "" : `<button class="btn sm" data-a="takeover">${ic("hand")}${ctl === "ai" ? "接管" : "开始控制"}</button>`;
  return `<div class="remote">
    <div class="remote-bar">
      <button class="btn sm" data-a="closeRemote">${ic("chevLeft")}返回</button>
      <b>${d.name}</b><span class="stat">${d.latency} 毫秒 · 58 帧</span>
      <span class="sep"></span>${ctlText}${ctlBtn}
      <span class="spacer"></span>
      <button class="btn sm" data-a="popRemote">${ic("sliders")}画质与显示器</button>
      <button class="btn sm icon" title="发送 Ctrl+Alt+Del" data-a="noop" data-msg="已发送 Ctrl+Alt+Del">${ic("ctrlAltDel")}</button>
      <button class="btn sm icon" title="全屏" data-a="noop" data-msg="全屏（原型未实现）">${ic("maximize")}</button>
      <button class="btn sm danger-soft" data-a="closeRemote">${ic("x")}断开</button>
    </div>
    <div class="remote-body">${desktopArt({ ai: ctl === "ai" })}
      <div class="remote-hint">${ctl === "human" ? `${ic("keyboard", "sm")}键盘和鼠标已发送到 ${d.name} · 按右 Ctrl 释放键盘捕获` : `${ic("eye", "sm")}正在观看 · 点击“${ctl === "ai" ? "接管" : "开始控制"}”后才会发送鼠标键盘`}</div>
    </div>
  </div>`;
}
A.popRemote = el => openPop(el, "remote");
POPS.remote = () => `<div class="ph2">画质</div>${["自动（推荐）", "清晰优先", "流畅优先"].map((q, i) => `<div class="pi"><span class="radio ${i === 0 ? "on" : ""}"></span>${q}</div>`).join("")}
  <div class="divider"></div><div class="ph2">显示器</div>${["显示器 1（主）", "显示器 2"].map((q, i) => `<div class="pi"><span class="radio ${i === 0 ? "on" : ""}"></span>${q}</div>`).join("")}
  <div class="divider"></div><div class="pi" style="cursor:default"><span class="muted">局域网直连 · 7.6 Mbps</span></div>`;

A.openRemote = () => {
  const d = devById(S.route.dev || "win-pc");
  if (netDown()) return toast("bad", "无法打开远程画面", "组网已断开");
  if (d.path !== "lan") { S.modal = { type: "noRemote", dev: d.id }; render(); return; }
  S.remote = d.id; render();
};
A.closeRemote = () => { S.remote = null; S.pop = null; if (S.control === "human") S.control = "none"; render(); };
A.takeover = () => { if (S.control === "ai") { S.modal = { type: "takeover" }; render(); return; } S.control = "human"; toast("info", "你已获得 win-pc 的桌面控制权"); render(); };
A.release = () => { S.control = "none"; toast("ok", "已释放控制权", "Claude Code 可以继续排队获取"); render(); };
A.stopAI = () => { S.modal = { type: "stopAI", opt: "stop" }; render(); };
A.devMenu = el => openMenu(el, [
  { icon: "edit", t: "重命名", msg: "重命名（原型未实现）" },
  { icon: "copy", t: "复制内网地址", msg: "已复制 100.64.0.2" },
  { icon: "terminal", t: "复制命令行用法", msg: "已复制：deskbridge exec win-pc \"…\"" },
  "-",
  { icon: "fileText", t: "查看节点日志", msg: "已打开节点日志" },
  { icon: "refresh", t: "重启节点", msg: "已向 win-pc 发送重启请求" },
  { icon: "share", t: "共享给别人…", msg: "请在“外部设备”中邀请" },
  "-",
  { icon: "trash", t: "从列表隐藏", msg: "已隐藏", bad: true },
]);
A.noop = el => { if (el && el.dataset.msg) toast("ok", el.dataset.msg); };

// =========================================================
// 对话框
// =========================================================
MODALS.noRemote = () => {
  const d = devById(S.modal.dev);
  return modal({
    title: "远程画面暂时不可用", sub: `${d.name} 当前经${d.path === "relay" ? `${d.relay}的中继` : pathText(d)}连接`,
    body: `<p class="muted" style="line-height:1.7">远程画面需要较高的带宽和稳定的延迟，目前只在局域网直连时开放。命令、文件和截图在任何连接方式下都可以使用。</p>`,
    foot: `<button class="btn" data-a="closeModal">知道了</button><button class="btn primary" data-a="goShot">${ic("image")}改用截图</button>`,
  });
};
A.goShot = () => { const id = S.modal.dev; S.modal = null; S.deskMode = "shot"; go(`#/devices/${id}/desk`); };

MODALS.takeover = () => modal({
  title: "从 AI 手中接管桌面？", sub: "Claude Code 正在操作 win-pc 的鼠标键盘",
  body: `<p class="muted" style="line-height:1.7">接管后，AI 当前的鼠标键盘操作会立即中断，它会排队等你释放控制权。它在后台运行的命令不受影响。</p>`,
  foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn primary" data-a="doTakeover">${ic("hand")}接管</button>`,
});
A.doTakeover = () => { S.modal = null; S.control = "human"; toast("info", "你已接管 win-pc", "Claude Code 在排队"); render(); };

MODALS.stopAI = () => {
  const o = S.modal.opt;
  const opt = (v, t, d) => `<div class="opt ${o === v ? "on" : ""}" data-a="modalOpt" data-k="opt" data-v="${v}"><span class="radio ${o === v ? "on" : ""}"></span><div><div class="t">${t}</div><div class="d">${d}</div></div></div>`;
  return modal({
    title: "停止 Claude Code", sub: "它正在操作 win-pc",
    body: opt("stop", "紧急停止", "取消它在这台设备上的全部请求，并拒绝后续请求，直到你恢复。") +
      opt("kill", "紧急停止并结束它启动的进程", "另外结束它启动的 2 个进程。做了一半的操作可能留下不完整的文件。") +
      opt("take", "只接管鼠标键盘", "由你远程控制；它的后台命令继续运行。"),
    foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn ${o === "take" ? "primary" : "danger"}" data-a="doStopAI">${o === "take" ? "接管" : o === "kill" ? "停止并结束进程" : "紧急停止"}</button>`,
  });
};
A.doStopAI = () => {
  const o = S.modal.opt; S.modal = null;
  if (o === "take") { S.control = "human"; toast("info", "你已接管 win-pc"); }
  else { S.control = "none"; toast("ok", o === "kill" ? "已停止 Claude Code 并结束 2 个进程" : "已紧急停止 Claude Code", "可以在“设置 → 桌面控制权”中恢复"); }
  render();
};

A.addDevice = () => { S.modal = { type: "addDevice", kind: "pc" }; render(); };
MODALS.addDevice = () => {
  const k = S.modal.kind;
  const tab = (v, icon, l) => `<button class="${k === v ? "on" : ""}" data-a="modalOpt" data-k="kind" data-v="${v}">${ic(icon, "sm")}${l}</button>`;
  const body = {
    pc: `<div class="col" style="gap:16px">
        <div class="feature"><div class="fi">1</div><div><div class="t">在另一台电脑上下载 DeskBridge</div><div class="d">Windows 10/11（macOS 版规划中）· <a style="cursor:pointer;text-decoration:underline" data-a="noop" data-msg="已复制下载链接">复制下载链接</a></div></div></div>
        <div class="feature"><div class="fi">2</div><div><div class="t">用同一账号登录</div><div class="d">user@example.com</div></div></div>
        <div class="feature"><div class="fi">3</div><div><div class="t">它会自动出现在设备列表</div><div class="d">不需要输入地址或配对码</div></div></div></div>`,
    server: `<div class="col" style="gap:12px"><div class="muted">在 Linux 服务器上运行：</div>
        <div class="snippet"><div class="codebox">curl -fsSL https://example.com/deskbridge/install.sh | sh\ndeskbridge login</div><button class="btn sm" data-a="noop" data-msg="已复制">${ic("copy")}复制</button></div>
        <div class="muted small">没有浏览器的服务器可以用授权密钥登录，密钥在“设置 → 组网”中生成。</div></div>`,
    phone: `<div class="row" style="gap:18px;align-items:flex-start"><div style="width:112px;height:112px;flex:none;border-radius:10px;background:repeating-conic-gradient(var(--text) 0 25%, var(--surface) 0 50%) 0 0/16px 16px;border:6px solid var(--surface);box-shadow:0 0 0 1px var(--border)"></div>
        <div class="col" style="gap:8px"><div style="font-weight:600">用安卓手机扫码安装</div><div class="muted small" style="line-height:1.6">手机目前作为控制端使用。让电脑控制手机还在调研中。</div></div></div>`,
  }[k];
  return modal({
    title: "添加设备", sub: "新设备用同一账号登录后会自动出现",
    body: `<div class="segmented">${tab("pc", "monitor", "电脑")}${tab("server", "server", "服务器")}${tab("phone", "phone", "手机")}<button disabled style="opacity:.5">WireGuard<span class="soon">规划中</span></button></div>${body}
      <div class="row small muted" style="gap:10px"><span class="spin"></span>正在等待新设备上线…<span class="spacer"></span><a style="cursor:pointer;text-decoration:underline" data-a="simNewDevice">模拟新设备上线</a></div>`,
    foot: `<button class="btn" data-a="closeModal">完成</button>`,
  });
};
A.simNewDevice = () => { S.modal = null; toast("ok", "发现新设备 new-pc", "Windows 11 · 局域网直连 · 3 毫秒"); render(); };
