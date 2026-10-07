// 设备页：设备列表、设备详情（概览、命令、文件、截图、界面元素、Agent、记录）、远程画面。

const TABS = [
  ["overview", "info", "概览"],
  ["cmd", "terminal", "命令"],
  ["files", "folder", "文件"],
  ["shot", "image", "截图"],
  ["uia", "layers", "界面元素"],
  ["agent", "bot", "Agent"],
  ["log", "history", "记录"],
];

// 设备对某个标签页的可用性：null 表示可用，否则返回 [原因类型, 说明]
function tabBlock(d, tab) {
  const desk = ["shot", "uia"].includes(tab);
  if (desk && d.kind === "server") return ["nodesk", "这台设备没有图形桌面"];
  if (d.group === "external") {
    const map = { cmd: "命令", files: "读文件", shot: "截图", uia: "界面元素", agent: "Agent" };
    if (map[tab] && !d.perms.includes(map[tab])) return ["perm", `alice 没有授予“${map[tab]}”权限`];
  }
  return null;
}

function viewDevices() {
  if (isPhone()) return S.route.dev ? phoneDetail() : phoneList();
  return `<div class="devices">${deviceList()}<div class="detail">${deviceDetail()}</div></div>`;
}

// ---------- 设备列表 ----------
function visibleDevices() {
  let list = DEVICES;
  if (S.scenario === "empty") list = list.filter(d => d.self);
  const f = S.filter, q = S.search.trim().toLowerCase();
  if (f === "online") list = list.filter(d => d.online && !d.noNode);
  if (f === "windows") list = list.filter(d => d.os.startsWith("Windows"));
  if (f === "linux") list = list.filter(d => /Linux|Ubuntu/.test(d.os));
  if (f === "android") list = list.filter(d => d.os.startsWith("Android"));
  if (q) list = list.filter(d => (d.name + d.ip + d.os).toLowerCase().includes(q));
  return list;
}

function devRow(d) {
  const [, , label, pc] = devState(d);
  const on = S.route.dev === d.id ? "on" : "";
  const dim = !d.online || d.noNode || (netDown() && !d.self) ? "dim" : "";
  let meta = d.os + (d.online && !d.noNode && !d.self ? " · " + pathText(d) : "");
  if (!d.online) meta = `${d.os} · ${d.lastSeen}在线`;
  if (d.group === "external") meta = `来自 ${d.owner} · ${d.perms.join("、")}`;
  if (d.self) meta = `${d.os}`;
  let right = "";
  if (d.self) right = `<span class="pill line">本机</span>`;
  else if (!netDown() && d.online && !d.noNode) {
    right = (pc !== "ok" ? `<span class="pill ${pc}">${label}</span>` : "") + `<span class="lat">${latBars(d.latency)} ${d.latency} 毫秒</span>`;
  } else right = `<span class="pill off">${label}</span>`;
  return `<div class="dev ${on} ${dim}" data-go="#/devices/${d.id}/${S.route.tab && S.route.dev ? S.route.tab : "overview"}">
    <div class="dic">${ic(kindIcon(d))}${dot(d)}</div>
    <div class="main"><div class="n">${d.name}</div><div class="m">${meta}</div></div>
    <div class="r">${right}</div>
  </div>`;
}

function listBody() {
  if (S.scenario === "loading") {
    return Array.from({ length: 5 }, () => `<div class="dev"><div class="dic sk" style="border:0"></div><div class="main"><div class="sk" style="height:12px;width:60%"></div><div class="sk" style="height:10px;width:85%;margin-top:7px"></div></div></div>`).join("");
  }
  if (S.scenario === "error") {
    return `<div class="empty"><div class="art bad">${ic("alert", "xl")}</div><h3>无法读取设备列表</h3><p>后台服务没有响应。<br><span class="errcode">ui_core_timeout</span></p><div class="btns"><button class="btn sm" data-a="retryList">${ic("refresh")}重试</button></div></div>`;
  }
  const list = visibleDevices();
  const mine = list.filter(d => d.group === "mine");
  const ext = list.filter(d => d.group === "external");
  let h = "";
  if (mine.length) h += `<div class="gtitle"><span>我的设备</span><span>${mine.length}</span></div>` + mine.map(devRow).join("");
  if (S.scenario === "empty") {
    h += `<div class="empty" style="padding:28px 12px"><div class="art accent">${ic("plus", "xl")}</div><h3>还没有其他设备</h3><p>在另一台设备上安装 DeskBridge，并用同一账号登录，它会自动出现在这里。</p><div class="btns"><button class="btn primary sm" data-a="addDevice">${ic("plus")}添加设备</button></div></div>`;
    return h;
  }
  h += `<div class="gtitle"><span>WireGuard 设备</span><span class="soon">规划中</span></div>`;
  if (ext.length) h += `<div class="gtitle"><span>外部设备</span><span>${ext.length}</span></div>` + ext.map(devRow).join("");
  if (!list.length) h = `<div class="empty"><div class="art">${ic("search", "xl")}</div><h3>没有匹配的设备</h3><p>换个关键词或筛选条件试试。</p></div>`;
  return h;
}

function deviceList() {
  const online = DEVICES.filter(d => d.online && !d.noNode && !d.self).length;
  const chip = (v, l) => `<button class="${S.filter === v ? "on" : ""}" data-a="setFilter" data-v="${v}">${l}</button>`;
  return `<section class="dlist">
    <div class="dlist-h"><h2>设备</h2><span class="sub">${S.scenario === "empty" ? "" : netDown() ? "状态未知" : `${online} 台在线`}</span>
      <button class="btn primary icon sm" data-a="addDevice" title="添加设备">${ic("plus")}</button></div>
    <div class="search">${ic("search", "sm")}<input id="dsearch" placeholder="搜索设备名、地址…" value="${esc(S.search)}" data-in="search" data-rerender="1"></div>
    <div class="chips">${chip("all", "全部")}${chip("online", "在线")}${chip("windows", "Windows")}${chip("linux", "Linux")}${chip("android", "安卓")}</div>
    <div class="groups" data-sk="dlist">${listBody()}</div>
  </section>`;
}

A.setFilter = el => { S.filter = el.dataset.v; render(); };
A.retryList = () => { S.scenario = "loading"; render(); setTimeout(() => { S.scenario = "normal"; parseRoute(); render(); toast("ok", "设备列表已刷新"); }, 1200); };

// ---------- 设备详情 ----------
function deviceDetail() {
  const r = S.route;
  if (S.scenario === "loading") {
    return `<div class="dh"><div class="big sk"></div><div style="flex:1"><div class="sk" style="height:18px;width:160px"></div><div class="sk" style="height:12px;width:300px;margin-top:8px"></div></div></div>
      <div class="tabs">${TABS.map(t => `<a>${t[2]}</a>`).join("")}</div>
      <div class="pane"><div class="pane-in"><div class="ov">${[1, 2, 3, 4].map(() => `<div class="card" style="height:150px;padding:14px"><div class="sk" style="height:12px;width:40%"></div><div class="sk" style="height:10px;width:80%;margin-top:14px"></div><div class="sk" style="height:10px;width:70%;margin-top:10px"></div></div>`).join("")}</div></div></div>`;
  }
  if (S.scenario === "error") {
    return `<div class="empty"><div class="art bad">${ic("alert", "xl")}</div><h3>出错了</h3><p>界面和后台服务（ui-core）的连接中断了，正在尝试重启后台服务。如果一直失败，可以查看日志。</p><p><span class="errcode">ui_core_exited · 退出码 2</span></p><div class="btns"><button class="btn" data-a="noop" data-msg="已打开日志目录">打开日志</button><button class="btn primary" data-a="retryList">${ic("refresh")}重试</button></div></div>`;
  }
  if (!r.dev || S.scenario === "empty") {
    return `<div class="empty"><div class="art accent">${ic("monitor", "xl")}</div><h3>添加你的第一台设备</h3>
      <p>DeskBridge 让你和 AI 都能操作自己的所有设备。在另一台电脑、服务器或手机上安装 DeskBridge，用同一账号登录即可。</p>
      <div class="btns"><button class="btn primary lg" data-a="addDevice">${ic("plus")}添加设备</button></div></div>`;
  }
  const d = devById(r.dev);
  return detailHead(d) + detailBanner(d) + detailTabs(d) + `<div class="pane">${detailPane(d)}</div>`;
}

function detailHead(d, phone = false) {
  const [, , label, pc] = devState(d);
  const meta = [];
  meta.push(`<span>${ic(kindIcon(d))}${d.os}</span>`);
  if (d.online && !netDown()) meta.push(`<span>${ic("route")}${d.ip}${d.self ? "" : ` · ${pathText(d)} ${d.latency} 毫秒`}</span>`);
  if (d.session && d.online && !netDown()) meta.push(`<span>${ic("user")}${d.session}</span>`);
  if (!d.online) meta.push(`<span>${ic("clock")}${d.lastSeen}在线</span>`);
  const canRemote = d.online && !d.noNode && d.kind !== "server" && d.kind !== "phone" && !d.self && d.group !== "external";
  return `<div class="dh">
    <div class="big">${ic(kindIcon(d))}</div>
    <div style="min-width:0"><h1>${d.name} <span class="pill ${pc}">${pc === "ai" ? `<span class="pulse ai live"></span>` : pc === "ok" ? `<span class="pulse"></span>` : ""}${label}</span>${d.self ? `<span class="pill line">本机</span>` : ""}</h1>
      <div class="meta">${meta.join("")}</div></div>
    <div class="actions">
      ${canRemote ? `<button class="btn" data-a="openRemote">${ic("cast")}<span class="label">远程画面</span></button>` : ""}
      ${d.online && !d.noNode ? `<button class="btn ghost icon" data-a="devMenu" title="更多">${ic("more")}</button>` : ""}
    </div>
  </div>`;
}

function detailBanner(d) {
  if (netDown() || !d.online || d.noNode) return "";
  const b = [];
  if (d.id === "win-pc") {
    if (S.control === "ai") b.push(`<div class="banner ai">
      <span class="pulse ai live"></span>
      <div class="t"><span class="who">${avatar("claude", "sm")}Claude Code 正在操作桌面</span> <span class="d">来自 linux-server · 2 分 14 秒 · 本地动鼠标即可抢回</span></div>
      <div class="btns"><button class="btn sm" data-a="openRemote">${ic("eye")}查看</button><button class="btn sm" data-a="takeover">${ic("hand")}接管</button><button class="btn sm danger-soft" data-a="stopAI">${ic("stop")}停止</button></div></div>`);
    if (S.control === "local") b.push(`<div class="banner info">${ic("mouse")}
      <div class="t"><b>本地用户正在使用这台电脑</b> <span class="d">AI 的鼠标键盘操作已暂停，Claude Code 在排队。本地停止操作 3 秒后自动继续。命令和文件操作不受影响。</span></div></div>`);
    if (S.control === "human") b.push(`<div class="banner info">${ic("hand")}
      <div class="t"><b>你正在远程操作桌面</b> <span class="d">Claude Code 的鼠标键盘操作在排队。60 秒无操作会自动释放。</span></div>
      <div class="btns"><button class="btn sm" data-a="openRemote">${ic("cast")}打开画面</button><button class="btn sm" data-a="release">释放控制权</button></div></div>`);
    if (S.control === "paused") b.push(`<div class="banner bad">${ic("pause")}
      <div class="t"><b>被控端已暂停所有远程操作</b> <span class="d">本地用户在 13:58 按下了暂停。新的远程操作会被拒绝，已经在运行的后台命令继续运行。只能在被控端解除。</span></div></div>`);
  }
  if (d.path === "relay") b.push(`<div class="banner warn">${ic("route")}
    <div class="t"><b>经中继（${d.relay}）连接，延迟 ${d.latency} 毫秒</b> <span class="d">命令和文件可以正常使用；远程画面暂时只支持局域网直连。</span></div>
    <div class="btns"><button class="btn sm" data-a="noop" data-msg="正在重新探测直连路径…">重新探测</button></div></div>`);
  if (d.outdated) b.push(`<div class="banner info">${ic("info")}
    <div class="t"><b>节点版本 ${d.version} 较旧</b> <span class="d">界面元素和 Agent 需要 0.3.0 以上。</span></div>
    <div class="btns"><button class="btn sm" data-a="noop" data-msg="已向 old-laptop 发送更新请求">远程更新</button></div></div>`);
  if (d.group === "external") b.push(`<div class="banner info">${ic("users")}
    <div class="t"><b>alice 共享给你的设备</b> <span class="d">你的权限：${d.perms.join("、")}。操作会同时记录在你和 alice 的操作记录中。</span></div></div>`);
  if (!b.length) return "";
  return `<div class="ctlbar col">${b.join("")}</div>`;
}

function detailTabs(d) {
  if (!d.online || d.noNode) return "";
  return `<nav class="tabs">${TABS.map(([k, icon, label]) => {
    const blk = tabBlock(d, k);
    const soon = k === "agent" ? `<span class="soon">规划中</span>` : "";
    return `<a class="${S.route.tab === k ? "on" : ""} ${blk ? "dim" : ""}" data-go="#/devices/${d.id}/${k}">${ic(blk && blk[0] === "perm" ? "lock" : icon)}${label}${soon}</a>`;
  }).join("")}</nav>`;
}

function detailPane(d) {
  if (netDown() && !d.self) {
    return `<div class="empty"><div class="art warn">${ic("wifiOff", "xl")}</div><h3>暂时无法连接 ${d.name}</h3><p>组网断开了，恢复后会自动重新连接。上次获取的信息仍然可以查看。</p><div class="btns"><button class="btn" data-a="netRetry">${ic("refresh")}立即重试</button></div></div>`;
  }
  if (!d.online) return paneOffline(d);
  if (d.noNode) return paneNoNode(d);
  const blk = tabBlock(d, S.route.tab);
  if (blk) {
    return `<div class="empty"><div class="art">${ic(blk[0] === "perm" ? "lock" : "monitor", "xl")}</div><h3>${blk[0] === "perm" ? "没有权限" : "不支持"}</h3><p>${blk[1]}。</p>${blk[0] === "perm" ? `<div class="btns"><button class="btn" data-a="noop" data-msg="已向 alice 发送权限申请">向 alice 申请</button></div>` : ""}</div>`;
  }
  switch (S.route.tab) {
    case "cmd": return paneCmd(d);
    case "files": return paneFiles(d);
    case "shot": return paneShot(d);
    case "uia": return paneUia(d);
    case "agent": return paneAgent(d);
    case "log": return `<div class="pane-in" data-sk="log">${activityList(ACTIVITY.filter(a => a.dev === d.id), true)}</div>`;
    default: return paneOverview(d);
  }
}

function paneOffline(d) {
  return `<div class="empty"><div class="art">${ic(kindIcon(d), "xl")}</div><h3>${d.name} 离线</h3>
    <p>最后在线：${d.lastSeen}。${d.controllerOnly ? "这台手机目前只作为控制端使用；安卓被控端还在规划中，所以没有可操作的工具。" : "设备开机并联网后会自动重新连接。"}</p>
    <div class="btns"><button class="btn" data-go="#/activity">查看操作记录</button><button class="btn ghost" data-a="noop" data-msg="已从列表中隐藏 ${d.name}">从列表隐藏</button></div></div>`;
}

function paneNoNode(d) {
  return `<div class="pane-in"><div class="card" style="max-width:620px;margin:20px auto 0">
    <div class="card-b col" style="gap:12px;padding:20px">
      <div class="row" style="gap:12px"><div class="empty" style="padding:0;flex:none"><div class="art" style="margin:0">${ic("hdd", "xl")}</div></div>
        <div><h3 style="font-size:16px">${d.name} 没有运行 DeskBridge</h3><div class="muted" style="margin-top:3px">它在你的内网中（${d.ip}），但没有安装或没有启动 DeskBridge 节点，所以不能执行命令或读写文件。</div></div></div>
      <div class="field"><label>在 ${d.name} 上运行</label>
        <div class="snippet"><div class="codebox">curl -fsSL https://example.com/deskbridge/install.sh | sh\ndeskbridge login</div><button class="btn sm" data-a="noop" data-msg="已复制安装命令">${ic("copy")}复制</button></div>
        <div class="hint">安装后用同一账号登录，这里会自动变为可用。Windows 和 macOS 可以从官网下载安装包。</div></div>
      <div class="row"><button class="btn ghost" data-a="noop" data-msg="已隐藏 ${d.name}">不再显示这台设备</button></div>
    </div></div></div>`;
}

// ---------- 概览 ----------
function sparkSvg(arr) {
  const max = Math.max(...arr) * 1.15, min = Math.min(...arr) * .85;
  const pts = arr.map((v, i) => `${(i / (arr.length - 1)) * 300},${44 - ((v - min) / (max - min)) * 40}`).join(" ");
  return `<svg class="spark" viewBox="0 0 300 46" preserveAspectRatio="none"><polyline points="${pts}" fill="none" stroke="var(--accent)" stroke-width="1.8" vector-effect="non-scaling-stroke"/><polyline points="0,46 ${pts} 300,46" fill="var(--accent-soft)" stroke="none"/></svg>`;
}

function paneOverview(d) {
  const caps = [
    ["terminal", "命令", true], ["folder", "文件", true], ["image", "截图", d.kind !== "server"],
    ["layers", "界面元素", d.kind !== "server" && !d.outdated], ["mouse", "鼠标键盘", d.kind !== "server"], ["bot", "Agent", !d.outdated && d.kind !== "server"],
  ];
  const ctlHolder = { none: "空闲", ai: "Claude Code（AI）", local: "本地用户", human: "你（远程人工）", paused: "已暂停" }[S.control];
  const recent = ACTIVITY.filter(a => a.dev === d.id).slice(0, 4);
  return `<div class="pane-in" data-sk="ov">
    ${d.merged ? `<div class="banner info">${ic("info")}<div class="t">${d.merged}。<a style="text-decoration:underline;cursor:pointer" data-a="noop" data-msg="已拆分为两个设备">拆分显示</a></div></div>` : ""}
    <div class="ov">
      <div class="card"><div class="card-h">${ic("route")}连接<span class="right"><span class="pill ${d.latency >= 150 ? "warn" : "ok"}">${pathText(d)}</span></span></div>
        <div class="card-b">${d.spark ? sparkSvg(d.spark) : ""}
          <dl class="kv" style="margin-top:10px"><dt>延迟</dt><dd>${d.latency} 毫秒（最近 10 分钟）</dd><dt>内网地址</dt><dd class="mono">${d.ip}</dd><dt>节点版本</dt><dd>${d.version}${d.outdated ? ` <span class="pill warn">可更新</span>` : ""}</dd></dl></div></div>
      <div class="card"><div class="card-h">${ic("cpu")}系统</div>
        <div class="card-b"><dl class="kv"><dt>系统</dt><dd>${d.os}</dd><dt>桌面会话</dt><dd>${d.session || "—"}</dd><dt>显示器</dt><dd>${d.displays ? d.displays + " 个" : "无"}</dd><dt>运行方式</dt><dd>${d.runMode || "—"}</dd></dl></div></div>
      ${d.id === "win-pc" ? `<div class="card"><div class="card-h">${ic("hand")}桌面控制权<span class="right"><button class="btn ghost sm" data-go="#/settings/control">规则</button></span></div>
        <div class="card-b"><dl class="kv"><dt>当前持有</dt><dd><b>${ctlHolder}</b></dd><dt>排队</dt><dd>${S.control === "local" || S.control === "human" ? "Claude Code（等待 4 秒）" : "无"}</dd><dt>本地用户</dt><dd>${S.control === "local" ? "正在操作" : "空闲 2 分钟"}</dd><dt>优先级</dt><dd class="muted">本地用户 &gt; 远程人工 &gt; AI</dd></dl></div></div>` : ""}
      <div class="card ${d.id === "win-pc" ? "" : "span2"}"><div class="card-h">${ic("box")}可用能力<span class="sub">由节点的能力报告决定</span></div>
        <div class="card-b"><div class="capgrid">${caps.map(([i, l, ok]) => `<div class="cap ${ok ? "" : "no"}">${ic(i, "sm")}${l}<span class="r">${ok ? ic("check", "sm") : `<span class="small">不可用</span>`}</span></div>`).join("")}</div>
        ${d.id === "win-pc" ? `<div class="small muted" style="margin-top:10px">另有 2 个本机 MCP 服务器（35 个工具）可以转发，见 Agent 标签页。</div>` : ""}</div></div>
      <div class="card span2"><div class="card-h">${ic("history")}最近操作<span class="right"><button class="btn ghost sm" data-go="#/devices/${d.id}/log">全部</button></span></div>
        <div class="card-b mini-log">${recent.length ? recent.map(a => `<div class="it"><span class="w">${a.t}</span>${avatar(a.actor, "sm")}<span class="s"><b>${ACTORS[a.actor].name}</b> · ${esc(a.sum)}</span><span class="pill ${ST_LABEL[a.st][1]}">${ST_LABEL[a.st][0]}</span></div>`).join("") : `<div class="muted small">暂无操作</div>`}</div></div>
    </div></div>`;
}

// ---------- 命令 ----------
function termHTML() {
  const c = S.cmd;
  const st = {
    idle: `<span class="st faint">尚未执行</span>`,
    run: `<span class="st"><span class="spin"></span>运行中</span><span id="runtime">${fmtSec(Date.now() - c.started)}</span><span>进程号 8812</span>`,
    ok: `<span class="st" style="color:var(--ok)">${ic("check", "sm")}完成</span><span>退出码 0</span><span>${c.dur}</span>`,
    cancel: `<span class="st" style="color:var(--muted)">${ic("stop", "sm")}已取消</span><span>进程树已结束</span><span>${c.dur}</span>`,
  }[c.status];
  const lines = c.lines.map(([k, t]) => {
    const html = esc(t).replace(/&lt;hl&gt;(.*?)&lt;\/hl&gt;/g, '<span class="hl">$1</span>');
    return k ? `<span class="${k}">${html}</span>` : html;
  }).join("\n");
  const head = c.status === "idle" ? "" : `<span class="dim">${c.at || "14:02:31"}  开始执行  ${esc(c.shell)}</span>\n`;
  const tail = c.status === "run" ? ` <span class="cursor"></span>` : c.status === "cancel" ? `\n<span class="warn">^C 已取消（用户），用时 ${c.dur}</span>` : c.status === "ok" ? `\n<span class="ok">✓ 退出码 0</span>` : "";
  return `<div class="term" id="term">
    <div class="term-h">${st}<span class="cmdline">${c.status === "idle" ? "" : esc(c.ran || c.text)}</span>
      <span class="tools">
        <button class="btn ghost sm" data-a="noop" data-msg="已复制输出">${ic("copy")}复制</button>
        ${c.status === "run" ? `<button class="btn sm danger-soft" data-a="cancelCmd">${ic("stop")}取消</button>` : `<button class="btn ghost sm" data-a="clearCmd">清空</button>`}
      </span></div>
    <div class="out" id="out">${c.status === "idle" ? `<span class="dim">输出会实时显示在这里。Windows 上的中文输出会自动转成 UTF-8。</span>` : head + lines + tail}</div>
  </div>`;
}

function paneCmd(d) {
  const c = S.cmd;
  const shells = d.os.startsWith("Windows") ? ["PowerShell 7", "命令提示符"] : ["bash", "sh"];
  if (!shells.includes(c.shell)) c.shell = shells[0];
  return `<div class="pane-in">
    <div class="cmd">
      <span class="sh" data-a="cycleShell" data-v="${shells.join("|")}" title="切换命令行">${c.shell}${ic("chevDown", "sm")}</span>
      <input id="cmdin" value="${esc(c.text)}" data-in="cmd.text" placeholder="输入命令，按回车执行" autocomplete="off" spellcheck="false">
      ${c.status === "run" ? `<button class="btn danger-soft" data-a="cancelCmd">${ic("stop")}取消</button>` : `<button class="btn primary" data-a="runCmd">${ic("play")}执行</button>`}
    </div>
    <div class="cmdopts">
      <label data-a="optToggle" data-k="timeout"><span class="check ${c.timeout ? "on" : ""}"></span>超时 120 秒</label>
      <label data-a="optToggle" data-k="admin"><span class="check ${c.admin ? "on" : ""}"></span>以管理员身份（需要被控端确认）</label>
      <span>工作目录 <span class="mono">D:\\data\\projects</span></span>
      <span class="spacer"></span><span>回车执行 · Esc 关闭弹窗</span>
    </div>
    ${termHTML()}
    <div class="card"><div class="card-h">${ic("history")}最近执行<span class="sub">点击填入</span></div>
      <div class="runs" style="padding:4px 6px 6px">${RECENT_RUNS.map(r => `<div class="it" data-a="fillCmd" data-c="${esc(r.c)}"><span class="pill ${r.st === "ok" ? "ok" : r.st === "err" ? "bad" : "off"}">${r.st === "ok" ? "成功" : r.st === "err" ? `退出码 ${r.code}` : "已取消"}</span><span class="c">${esc(r.c)}</span><span class="w">${r.w}</span></div>`).join("")}</div></div>
  </div>`;
}

let cmdTimer = null;
function updateTerm() { const t = $("#term"); if (t) { t.outerHTML = termHTML(); const o = $("#out"); if (o) o.scrollTop = o.scrollHeight; } }
A.runCmd = () => {
  if (netDown()) return toast("bad", "无法执行", "组网已断开");
  if (S.control === "paused") return toast("bad", "被拒绝", "被控端已暂停远程操作");
  const c = S.cmd;
  if (!c.text.trim()) return;
  clearInterval(cmdTimer);
  const src = /ping/i.test(c.text) ? CMD_OUTPUTS.ping : CMD_OUTPUTS.default;
  Object.assign(c, { status: "run", lines: [], started: Date.now(), ran: c.text, at: new Date().toTimeString().slice(0, 8) });
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
A.fillCmd = el => { S.cmd.text = el.dataset.c; render(); $("#cmdin")?.focus(); };
A.cycleShell = el => { const s = el.dataset.v.split("|"); S.cmd.shell = s[(s.indexOf(S.cmd.shell) + 1) % s.length]; render(); };
A.optToggle = el => { S.cmd[el.dataset.k] = !S.cmd[el.dataset.k]; render(); };

// ---------- 文件 ----------
function paneFiles(d) {
  const f = S.fs;
  const parts = f.path.split("\\").filter(Boolean);
  const crumbs = parts.map((p, i) => {
    const path = parts.slice(0, i + 1).join("\\") + (i === 0 ? "\\" : "");
    return `<a data-a="cd" data-p="${esc(path)}">${esc(p)}</a>`;
  }).join(`<span class="sep">${ic("chevRight", "sm")}</span>`);
  const list = FILES[f.path];
  const sel = list && list.find(x => x.n === f.sel);
  let table;
  if (f.path === "D:\\data\\private") {
    table = `<div class="empty"><div class="art bad">${ic("lock", "xl")}</div><h3>拒绝访问</h3><p>节点以普通用户身份运行，没有读取这个文件夹的权限。</p><p><span class="errcode">permission_denied</span></p><div class="btns"><button class="btn" data-a="cd" data-p="D:\\data">返回上一级</button></div></div>`;
  } else if (!list) {
    table = `<div class="empty"><div class="art">${ic("folder", "xl")}</div><h3>空文件夹</h3><p>可以把文件拖到这里上传。</p></div>`;
  } else {
    table = `<table class="table"><thead><tr><th>名称</th><th class="hide-m">修改时间</th><th class="num">大小</th></tr></thead><tbody>
      ${list.map(x => `<tr class="click ${f.sel === x.n ? "sel" : ""}" data-a="${x.d ? "cd" : "selFile"}" data-p="${esc(f.path + "\\" + x.n)}" data-n="${esc(x.n)}">
        <td><span class="fname">${x.d ? `<span class="folder">${ic("folder")}</span>` : ic(x.text ? "fileText" : "file")}${esc(x.n)}</span></td>
        <td class="muted hide-m nowrap">${x.m}</td><td class="num muted">${x.d ? "—" : x.s}</td></tr>`).join("")}
      ${f.path === "D:\\data" ? `<tr class="click" data-a="cd" data-p="D:\\data\\private"><td><span class="fname"><span class="folder">${ic("folder")}</span>private</span></td><td class="muted hide-m">—</td><td class="num muted">—</td></tr>` : ""}
      </tbody></table>`;
  }
  const up = f.upload;
  return `<div class="pane-in">
    <div class="row" style="flex-wrap:wrap">
      <button class="btn ghost icon sm" data-a="cdUp" title="上一级">${ic("chevLeft")}</button>
      <div class="crumbs">${crumbs}</div><span class="spacer"></span>
      <button class="btn sm" data-a="noop" data-msg="已刷新">${ic("refresh")}刷新</button>
      <button class="btn sm primary" data-a="upload">${ic("upload")}上传</button>
    </div>
    ${up ? `<div class="card" style="padding:10px 14px"><div class="row small"><span>${ic("upload", "sm")}上传 <b>${up.n}</b></span><span class="spacer"></span><span class="muted">${up.p}% · 4.2 MB/秒</span><button class="btn ghost sm" data-a="cancelUpload">取消</button></div><div class="progress" style="margin-top:8px"><i style="width:${up.p}%"></i></div></div>` : ""}
    <div class="fs ${sel ? "" : "nopreview"}" style="flex:1;min-height:0">
      <div class="card" style="overflow:auto" data-sk="fs">${table}</div>
      ${sel ? `<div class="card preview"><div class="card-h">${ic(sel.text ? "fileText" : "file")}<span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(sel.n)}</span><span class="right"><button class="btn ghost icon sm" data-a="selFile" data-n="">${ic("x")}</button></span></div>
        ${sel.text ? `<pre>${esc(sel.text)}</pre>` : `<div class="empty"><div class="art">${ic("file", "xl")}</div><p>${sel.s} · 不支持预览</p></div>`}
        <div class="row" style="padding:10px 12px;border-top:1px solid var(--border)"><button class="btn sm" data-a="noop" data-msg="已保存到 下载 文件夹">${ic("download")}下载</button><button class="btn sm ghost" data-a="noop" data-msg="已复制路径">${ic("copy")}复制路径</button><span class="spacer"></span><button class="btn sm ghost danger-soft" data-a="noop" data-msg="删除需要确认（原型未实现）">${ic("trash")}</button></div></div>` : ""}
    </div>
    <div class="drop">${ic("upload", "sm")} 把文件拖到这里上传到 <span class="mono">${esc(f.path)}</span></div>
  </div>`;
}
A.cd = el => { S.fs.path = el.dataset.p; S.fs.sel = null; render(); };
A.cdUp = () => { const p = S.fs.path.split("\\").filter(Boolean); if (p.length > 1) { p.pop(); S.fs.path = p.join("\\") + (p.length === 1 ? "\\" : ""); S.fs.sel = null; render(); } };
A.selFile = el => { S.fs.sel = el.dataset.n || null; render(); };
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

// ---------- 截图 ----------
function mouseSvg(x, y) {
  return `<svg class="mouse" viewBox="0 0 12 18" style="left:${x}%;top:${y}%"><path d="M1 1v14.5l3.6-3.4 2.6 5.6 2.3-1.1-2.6-5.4H11.5z" fill="#fff" stroke="#111" stroke-width="1" stroke-linejoin="round"/></svg>`;
}

function desktopArt({ ai = false, small = false, hl = false } = {}) {
  return `<div class="desk-wrap"><div class="desk ${small ? "small" : ""}">
    <div class="win" style="left:6%;top:8%;width:48%;height:58%"><div class="tb">无标题 - 记事本</div><div class="ln" style="width:70%"></div><div class="ln" style="width:55%"></div><div class="ln" style="width:62%"></div><div class="ln" style="width:30%"></div></div>
    <div class="win dark" style="left:40%;top:30%;width:52%;height:56%"><div class="tb">PowerShell</div><div class="ln b" style="width:40%"></div><div class="ln" style="width:75%"></div><div class="ln" style="width:68%"></div><div class="ln g" style="width:35%"></div><div class="ln" style="width:58%"></div></div>
    ${hl ? `<div class="hit" style="left:6.5%;top:14%;width:47%;height:51%"></div>` : ""}
    ${ai ? `<div class="aiframe"></div><div class="aitip">AI 正在操作（来自 linux-server）</div>${mouseSvg(30, 40)}` : mouseSvg(70, 62)}
    <div class="taskbar"><i></i><i></i><i></i><i></i><i></i></div>
  </div></div>`;
}

function paneShot(d) {
  const s = S.shot;
  const cur = s.list[s.sel];
  return `<div class="pane-in">
    <div class="row" style="flex-wrap:wrap">
      <div class="segmented">${["1", "2", "all"].map(m => `<button class="${s.mon === m ? "on" : ""}" data-a="setMon" data-v="${m}">${m === "all" ? "全部显示器" : "显示器 " + m}</button>`).join("")}</div>
      <select class="select" style="width:110px;height:30px"><option>原始大小</option><option>缩小一半</option></select>
      <span class="spacer"></span>
      <button class="btn sm" data-a="noop" data-msg="已复制到剪贴板">${ic("copy")}复制</button>
      <button class="btn sm" data-a="noop" data-msg="已保存到 图片 文件夹">${ic("download")}保存</button>
      <button class="btn sm primary" data-a="takeShot">${ic("image")}截图</button>
    </div>
    <div class="shot">
      <div class="shot-view card" style="padding:10px">
        <div class="shot-canvas">${s.taking ? `<div class="col" style="align-items:center;color:var(--muted)"><span class="spin lg"></span>正在截图…</div>` : desktopArt({ ai: S.control === "ai" && cur.ai })}</div>
        <div class="row small muted" style="padding:8px 2px 0">${ic("info", "sm")}<span>${cur.t} · ${cur.mon} · 1920 × 1080 · 312 KB · 由 ${cur.by} 截取</span></div>
      </div>
      <div class="thumbs" data-sk="thumbs">${s.list.map((x, i) => `<div class="thumb ${i === s.sel ? "on" : ""}" data-a="selShot" data-i="${i}">${desktopArt({ small: true, ai: x.ai })}<div class="cap2">${x.t} · ${x.by}</div></div>`).join("")}</div>
    </div></div>`;
}
A.setMon = el => { S.shot.mon = el.dataset.v; render(); };
A.selShot = el => { S.shot.sel = +el.dataset.i; render(); };
A.takeShot = () => {
  S.shot.taking = true; render();
  setTimeout(() => {
    S.shot.taking = false;
    S.shot.list.unshift({ t: new Date().toTimeString().slice(0, 8), by: "示例用户", mon: S.shot.mon === "all" ? "全部" : "显示器 " + S.shot.mon });
    S.shot.sel = 0; render();
  }, 900);
};

// ---------- 界面元素 ----------
function treeHTML(n, depth = 0) {
  const has = n.kids && n.kids.length;
  const open = S.uia.open[n.id] ?? n.open;
  return `<div class="node ${S.uia.sel === n.id ? "on" : ""}" style="padding-left:${6 + depth * 16}px" data-a="uiaSel" data-id="${n.id}">
      <span class="tw" ${has ? `data-a="uiaToggle" data-id="${n.id}"` : ""}>${has ? ic(open ? "chevDown" : "chevRight", "sm") : ""}</span>
      <span class="ty">${n.ty}</span><span class="nm">${n.nm ? `“${esc(n.nm)}”` : ""}</span></div>` +
    (has && open ? n.kids.map(k => treeHTML(k, depth + 1)).join("") : "");
}

function paneUia(d) {
  const u = S.uia;
  const win = UI_WINDOWS.find(w => w.id === u.win);
  const p = UI_PROPS[u.sel] || { ty: "—", nm: "—", aid: "—", cls: "—", rect: "—", pat: [], en: true };
  let center;
  if (win.elevated) {
    center = `<div class="empty"><div class="art warn">${ic("shieldAlert", "xl")}</div><h3>窗口权限高于节点</h3><p>“${win.n}”以管理员身份运行，普通权限的节点不能读取或操作它。</p><p><span class="errcode">elevated_target</span></p></div>`;
  } else if (u.win !== "np") {
    center = `<div class="empty"><div class="art">${ic("layers", "xl")}</div><p>原型只准备了“记事本”的示例数据。</p></div>`;
  } else center = `<div class="tree">${treeHTML(UI_TREE)}</div>`;
  return `<div class="pane-in">
    <div class="uia">
      <div class="card wlc"><div class="card-h">${ic("screen2")}窗口<span class="right"><button class="btn ghost icon sm" data-a="noop" data-msg="已刷新窗口列表">${ic("refresh")}</button></span></div>
        <div class="scroll wl">${UI_WINDOWS.map(w => `<a class="${u.win === w.id ? "on" : ""}" data-a="uiaWin" data-id="${w.id}">${w.elevated ? ic("shieldAlert", "sm") : ic("monitor", "sm")}<span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(w.n)}</span></a>`).join("")}</div></div>
      <div class="card"><div class="card-h">${ic("layers")}元素树<span class="sub">${esc(win.n)}</span></div>
        <div style="padding:8px 8px 0"><div class="search" style="height:28px">${ic("search", "sm")}<input placeholder="按名称或类型查找元素"></div></div>
        <div class="scroll" data-sk="tree">${center}</div></div>
      <div class="card props"><div class="card-h">${ic("target")}属性</div>
        <div class="card-b col" style="gap:12px">
          <dl class="kv" style="grid-template-columns:78px 1fr"><dt>类型</dt><dd>${p.ty}</dd><dt>名称</dt><dd>${esc(p.nm)}</dd><dt>自动化 ID</dt><dd class="mono">${p.aid}</dd><dt>类名</dt><dd class="mono">${p.cls}</dd><dt>位置</dt><dd class="mono">${p.rect}</dd><dt>可用</dt><dd>${p.en ? "是" : "否"}</dd></dl>
          <div><div class="small muted" style="margin-bottom:6px">支持的操作</div><div class="chips">${p.pat.map(x => `<span class="tag">${x}</span>`).join("") || `<span class="faint small">无</span>`}</div></div>
          <div class="divider"></div>
          <div class="col" style="gap:6px">
            <button class="btn sm" data-a="noop" data-msg="已在被控端屏幕上高亮这个元素 2 秒">${ic("target")}在屏幕上高亮</button>
            <button class="btn sm" data-a="noop" data-msg="已执行 Invoke（后台操作，不移动鼠标）">${ic("play")}点击（后台）</button>
            <button class="btn sm" data-a="noop" data-msg="已复制选择器">${ic("copy")}复制选择器</button>
          </div>
          <div class="small muted">后台操作不抢鼠标键盘，不需要控制权；目标是前台窗口时按规则处理。</div>
        </div></div>
    </div></div>`;
}
A.uiaSel = (el, ev) => { if (ev.target.closest("[data-a=uiaToggle]")) return A.uiaToggle(ev.target.closest("[data-a=uiaToggle]")); S.uia.sel = el.dataset.id; render(); };
A.uiaToggle = el => { const id = el.dataset.id; const n = findNode(UI_TREE, id); S.uia.open[id] = !(S.uia.open[id] ?? n.open); render(); };
A.uiaWin = el => { S.uia.win = el.dataset.id; render(); };
function findNode(n, id) { if (n.id === id) return n; for (const k of n.kids || []) { const r = findNode(k, id); if (r) return r; } return null; }

// ---------- Agent ----------
const AGENT_STEPS = [
  ["ai", "sparkles", "理解任务", "读取项目结构，准备编译"],
  ["run", "terminal", "执行命令", "<code>go build ./...</code>"],
  ["run", "alert", "发现 2 个编译错误", "internal/proto/codec.go:42、cmd/node/main.go:88"],
  ["run", "edit", "修改文件", "<code>internal/proto/codec.go</code>（+6 −2）"],
  ["run", "edit", "修改文件", "<code>cmd/node/main.go</code>（+1 −1）"],
  ["run", "terminal", "重新编译", "<code>go build ./...</code> 通过"],
  ["ok", "check", "完成", "修改了 2 个文件，编译通过。用时 1 分 48 秒"],
];

function paneAgent(d) {
  const g = S.agent;
  const steps = g.steps.map((s, i) => {
    const last = i === g.steps.length - 1 && g.running;
    return `<div class="step"><span class="si ${last ? "run" : s[0] === "ok" ? "ok" : s[0] === "ai" ? "ai" : "ok"}">${last ? `<span class="spin" style="width:10px;height:10px"></span>` : ic(s[1], "sm")}</span><div class="sb"><div>${s[2]}</div><div class="sd">${s[3]}</div></div></div>`;
  }).join("");
  return `<div class="pane-in">
    <div class="banner info">${ic("info")}<div class="t"><b>规划中的功能</b> <span class="d">调用被控端已有的 Agent、MCP 服务器和 Skill（任务 22）。这里先确定界面位置和交互。</span></div></div>
    <div class="agent">
      <div class="side">
        <div class="card"><div class="card-h">${ic("bot")}Agent</div>${AGENTS.map(a => `<div class="ag"><span class="avatar sm ${a.avc}">${a.av}</span><div><div class="n">${a.n}</div><div class="muted small">${a.v || "—"}</div></div><span class="r pill ${a.ok ? "ok" : "off"}">${a.st}</span></div>`).join("")}</div>
        <div class="card"><div class="card-h">${ic("plug")}MCP 服务器<span class="sub">本机配置</span></div>${MCP_SERVERS.map(m => `<div class="ag"><div><div class="n mono" style="font-size:12.5px">${m.n}</div><div class="muted small">${m.tools} 个工具</div></div><span class="r pill ${m.ok ? "ok" : "off"}">${m.st}</span></div>`).join("")}</div>
        <div class="card"><div class="card-h">${ic("sparkles")}Skill</div><div class="card-b chips">${SKILLS.map(s => `<span class="tag mono">${s}</span>`).join("")}</div></div>
      </div>
      <div class="col" style="min-height:0;gap:12px">
        <div class="card"><div class="card-b col" style="gap:10px">
          <div class="row" style="white-space:nowrap"><span class="small muted">交给</span>
            <div class="segmented">${AGENTS.filter(a => a.ok).map(a => `<button class="${g.who === a.n ? "on" : ""}" data-a="agentWho" data-v="${a.n}">${a.n}</button>`).join("")}</div></div>
          <textarea class="textarea" rows="3" id="agentin" data-in="agent.prompt">${esc(g.prompt)}</textarea>
          <div class="row" style="flex-wrap:wrap"><span class="tag mono" title="工作目录">${ic("folder", "sm")}D:\\data\\projects\\deskbridge</span>
            <select class="select" style="width:170px;height:30px"><option>每次写入前询问我</option><option>只读操作自动允许</option><option>全部自动（危险）</option></select>
            <span class="spacer"></span>${g.running ? `<button class="btn danger-soft" data-a="agentStop">${ic("stop")}取消任务</button>` : `<button class="btn primary" data-a="agentRun">${ic("send")}交给 ${g.who}</button>`}</div>
        </div></div>
        <div class="card" style="flex:1;min-height:160px;display:flex;flex-direction:column">
          <div class="card-h">${ic("gitBranch")}任务过程${g.running ? `<span class="pill info"><span class="spin" style="width:9px;height:9px;border-width:1.5px"></span>进行中</span>` : g.steps.length ? `<span class="pill ok">已完成</span>` : ""}<span class="right small muted">断线后可以回来继续查看</span></div>
          ${g.steps.length ? `<div class="steps" data-sk="steps">${steps}</div>` : `<div class="empty">${ic("bot", "xl")}<p>任务的每一步（命令、文件修改、工具调用）会实时显示在这里。</p></div>`}
        </div>
      </div>
    </div></div>`;
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

// ---------- 远程画面 ----------
function viewRemote() {
  const d = devById(S.remote);
  const ctl = S.control;
  const ctlText = { ai: `<span class="pill ai"><span class="pulse ai live"></span>AI 正在操作 · 你在观看</span>`, human: `<span class="pill info">${ic("hand", "sm")}你正在控制</span>`, local: `<span class="pill info">本地用户正在使用 · 你在观看</span>`, none: `<span class="pill off">只观看</span>`, paused: `<span class="pill bad">被控端已暂停远程操作</span>` }[ctl];
  const ctlBtn = ctl === "human" ? `<button class="btn sm" data-a="release">释放控制权</button>` : ctl === "paused" || ctl === "local" ? "" : `<button class="btn sm" data-a="takeover">${ic("hand")}${ctl === "ai" ? "接管" : "开始控制"}</button>`;
  return `<div class="remote">
    <div class="remote-bar">
      <button class="btn sm" data-a="closeRemote">${ic("chevLeft")}返回</button>
      <b>${d.name}</b>
      <span class="stat">${ic("route", "sm")}局域网直连 · ${d.latency} 毫秒</span>
      <span class="stat">58 帧 · 7.6 Mbps</span>
      <span class="sep"></span>
      ${ctlText}${ctlBtn}
      <span class="spacer"></span>
      <div class="segmented" style="background:#22222a">${["自动", "清晰", "流畅"].map((q, i) => `<button class="${i === 0 ? "on" : ""}" style="${i === 0 ? "background:#3a3a46;color:#fff" : "color:#aaa"}">${q}</button>`).join("")}</div>
      <button class="btn sm" data-a="noop" data-msg="已切换到显示器 2">${ic("screen2")}显示器 1</button>
      <button class="btn sm icon" title="发送 Ctrl+Alt+Del" data-a="noop" data-msg="已发送 Ctrl+Alt+Del">${ic("ctrlAltDel")}</button>
      <button class="btn sm icon" title="截图" data-a="noop" data-msg="已截图并保存">${ic("image")}</button>
      <button class="btn sm icon" title="全屏" data-a="noop" data-msg="全屏（原型未实现）">${ic("maximize")}</button>
      <button class="btn sm danger-soft" data-a="closeRemote">${ic("x")}断开</button>
    </div>
    <div class="remote-body">${desktopArt({ ai: ctl === "ai" })}
      <div class="remote-hint">${ctl === "human" ? `${ic("keyboard", "sm")}键盘和鼠标已发送到 ${d.name} · 按 右 Ctrl 释放键盘捕获` : `${ic("eye", "sm")}正在观看 · 点击“${ctl === "ai" ? "接管" : "开始控制"}”后才会发送鼠标键盘`}</div>
    </div>
  </div>`;
}
A.openRemote = () => {
  const d = devById(S.route.dev || "win-pc");
  if (netDown()) return toast("bad", "无法打开远程画面", "组网已断开");
  if (d.path !== "lan") { S.modal = { type: "noRemote", dev: d.id }; render(); return; }
  S.remote = d.id; render();
};
A.closeRemote = () => { S.remote = null; if (S.control === "human") S.control = "none"; render(); };
A.takeover = () => {
  if (S.control === "ai") {
    S.modal = { type: "takeover" }; render(); return;
  }
  S.control = "human"; toast("info", "你已获得 win-pc 的桌面控制权"); render();
};
A.release = () => { S.control = "none"; toast("ok", "已释放控制权", "Claude Code 可以继续排队获取"); render(); };
A.stopAI = () => { S.modal = { type: "stopAI", opt: "stop" }; render(); };
A.devMenu = el => openMenu(el, [
  { icon: "edit", t: "重命名", msg: "重命名（原型未实现）" },
  { icon: "copy", t: "复制内网地址", msg: "已复制 100.64.0.2" },
  { icon: "terminal", t: "复制命令行用法", msg: "已复制：deskbridge exec win-pc \"…\"" },
  "-",
  { icon: "fileText", t: "查看节点日志", msg: "已打开节点日志" },
  { icon: "refresh", t: "重启节点", msg: "已向 win-pc 发送重启请求" },
  { icon: "share", t: "共享给别人…", msg: "请在“外部设备”页邀请" },
  "-",
  { icon: "trash", t: "从列表隐藏", msg: "已隐藏", bad: true },
]);
A.noop = el => { if (el && el.dataset.msg) toast("ok", el.dataset.msg); };

// ---------- 设备相关的对话框 ----------
MODALS.noRemote = () => {
  const d = devById(S.modal.dev);
  return modal({
    title: "远程画面暂时不可用", sub: `${d.name} 当前${d.path === "relay" ? `经中继（${d.relay}）` : pathText(d)}连接`,
    icon: `<span class="empty" style="padding:0;flex:none"><span class="art warn" style="width:40px;height:40px;border-radius:12px;margin:0">${ic("cast")}</span></span>`,
    body: `<p class="muted" style="line-height:1.6">远程画面需要较高的带宽和稳定的延迟，目前只在局域网直连时开放。命令、文件和截图在任何连接方式下都可以使用。</p>`,
    foot: `<button class="btn" data-a="closeModal">知道了</button><button class="btn primary" data-a="goShot">${ic("image")}改用截图</button>`,
  });
};
A.goShot = () => { const id = S.modal.dev; S.modal = null; go(`#/devices/${id}/shot`); };

MODALS.takeover = () => modal({
  title: "从 AI 手中接管桌面？", sub: "Claude Code 正在操作 win-pc 的鼠标键盘",
  body: `<p class="muted" style="line-height:1.6">接管后，Claude Code 当前的鼠标键盘操作会立即中断（返回 <span class="mono">preempted</span>），它会排队等你释放控制权。它在后台运行的命令不受影响。</p>`,
  foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn primary" data-a="doTakeover">${ic("hand")}接管</button>`,
});
A.doTakeover = () => { S.modal = null; S.control = "human"; toast("info", "你已接管 win-pc", "Claude Code 在排队"); render(); };

MODALS.stopAI = () => {
  const o = S.modal.opt;
  const opt = (v, t, d) => `<div class="opt ${o === v ? "on" : ""}" data-a="modalOpt" data-k="opt" data-v="${v}"><span class="radio ${o === v ? "on" : ""}"></span><div><div class="t">${t}</div><div class="d">${d}</div></div></div>`;
  return modal({
    title: "停止 AI 在 win-pc 上的操作", sub: "Claude Code（来自 linux-server）",
    body: opt("stop", "紧急停止", "取消 Claude Code 在这台设备上的全部请求，并拒绝它后续的请求，直到你在这里恢复。") +
      opt("kill", "紧急停止并结束它启动的进程", "在紧急停止的基础上，结束 AI 启动的 2 个进程（pwsh.exe、go.exe）。做了一半的操作可能留下不完整的文件，会写入操作记录。") +
      opt("take", "只接管鼠标键盘", "中断 AI 的前台输入，由你远程控制；AI 的后台命令继续运行。"),
    foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn ${o === "take" ? "primary" : "danger"}" data-a="doStopAI">${o === "take" ? "接管" : o === "kill" ? "停止并结束进程" : "紧急停止"}</button>`,
  });
};
A.doStopAI = () => {
  const o = S.modal.opt; S.modal = null;
  if (o === "take") { S.control = "human"; toast("info", "你已接管 win-pc"); }
  else { S.control = "none"; toast("ok", o === "kill" ? "已停止 Claude Code 并结束 2 个进程" : "已紧急停止 Claude Code", "可以在“设置 → 控制权”中恢复"); }
  render();
};

A.addDevice = () => { S.modal = { type: "addDevice", kind: "pc", waiting: true }; render(); };
MODALS.addDevice = () => {
  const k = S.modal.kind;
  const tab = (v, icon, l) => `<button class="${k === v ? "on" : ""}" data-a="modalOpt" data-k="kind" data-v="${v}">${ic(icon, "sm")}${l}</button>`;
  const body = {
    pc: `<div class="col" style="gap:12px">
        <div class="feature"><div class="fi">1</div><div><div class="t">在另一台电脑上下载 DeskBridge</div><div class="d">Windows 10/11（macOS 版规划中）</div><div class="row" style="margin-top:8px"><button class="btn sm" data-a="noop" data-msg="已复制下载链接">${ic("copy")}复制下载链接</button></div></div></div>
        <div class="feature"><div class="fi">2</div><div><div class="t">用同一账号登录</div><div class="d">当前账号：user@example.com</div></div></div>
        <div class="feature"><div class="fi">3</div><div><div class="t">它会自动出现在设备列表里</div><div class="d">不需要输入地址或配对码</div></div></div></div>`,
    server: `<div class="col" style="gap:10px"><div class="muted">在 Linux 服务器上运行下面的命令，然后打开输出中的登录链接：</div>
        <div class="snippet"><div class="codebox">curl -fsSL https://example.com/deskbridge/install.sh | sh\ndeskbridge login</div><button class="btn sm" data-a="noop" data-msg="已复制">${ic("copy")}复制</button></div>
        <div class="muted small">没有浏览器的服务器可以用授权密钥登录：<span class="mono">deskbridge login --auth-key &lt;密钥&gt;</span>，密钥在“设置 → 组网”中生成。</div></div>`,
    phone: `<div class="row" style="gap:16px;align-items:flex-start"><div style="width:120px;height:120px;flex:none;border-radius:10px;background:repeating-conic-gradient(var(--text) 0 25%, var(--surface) 0 50%) 0 0/16px 16px;border:6px solid var(--surface);box-shadow:0 0 0 1px var(--border)"></div>
        <div class="col" style="gap:8px"><div class="t" style="font-weight:600">用安卓手机扫码安装</div><div class="muted small" style="line-height:1.6">手机目前作为控制端：查看和操作你的电脑。让电脑控制手机（安卓被控端）还在调研中。</div></div></div>`,
  }[k];
  return modal({
    title: "添加设备", sub: "新设备用同一账号登录后会自动出现", wide: true,
    body: `<div class="segmented">${tab("pc", "monitor", "电脑")}${tab("server", "server", "服务器")}${tab("phone", "phone", "手机")}</div>${body}
      <div class="banner info"><span class="spin"></span><div class="t">正在等待新设备上线…</div><div class="btns"><button class="btn sm" data-a="simNewDevice">模拟新设备上线</button></div></div>`,
    foot: `<button class="btn" data-a="closeModal">完成</button>`,
  });
};
A.simNewDevice = () => { S.modal = null; toast("ok", "发现新设备 new-pc", "Windows 11 · 局域网直连 · 3 毫秒"); render(); };

// ---------- 手机布局 ----------
function phoneList() {
  return `<div class="mtop"><h1>设备</h1><button class="btn ghost icon" data-a="openPalette">${ic("search")}</button><button class="btn primary icon" data-a="addDevice">${ic("plus")}</button></div>
    <div class="devices">${deviceList().replace('<div class="dlist-h">', '<div class="dlist-h" style="display:none">')}</div>`;
}

function phoneDetail() {
  const d = devById(S.route.dev);
  return `<div class="mtop"><button class="btn ghost icon" data-go="#/devices">${ic("chevLeft", "lg")}</button><h2>${d.name}</h2>
      ${d.online && !d.noNode && d.path === "lan" ? `<button class="btn ghost icon" data-a="openRemote">${ic("cast")}</button>` : ""}
      ${d.online && !d.noNode ? `<button class="btn ghost icon" data-a="devMenu">${ic("more")}</button>` : ""}</div>
    <div class="detail" style="flex:1">${detailHead(d).replace('<h1>', '<h1 style="font-size:17px">')}${detailBanner(d)}${detailTabs(d)}<div class="pane">${detailPane(d)}</div></div>`;
}
