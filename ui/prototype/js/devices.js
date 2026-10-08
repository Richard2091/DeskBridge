// 设备（初版范围）：首页按连接方式横向切换（Tailscale、局域网），每台设备一行→ 设备页（概览、终端、文件、桌面）。次要信息放进展开器、抽屉和弹出面板。

const TABS = [
  ["overview", "info", "概览"],
  ["term", "terminal", "终端"],
  ["files", "folder", "文件"],
  ["desk", "monitor", "桌面"],
];

// 设备对某个标签页的可用性：null 表示可用，否则返回 [原因类型, 说明]
function tabBlock(d, tab) {
  if (tab === "desk" && d.kind === "server") return ["nodesk", "这台设备没有图形桌面"];
  return null;
}

function viewDevices() {
  return S.route.dev ? devicePage(devById(S.route.dev)) : devicesHome();
}

// =========================================================
// 首页
// =========================================================
function isActive(d) { return d.online && !d.noNode; }
const unpaired = d => d.via === "lan" && !d.paired; // 局域网中发现、尚未配对

function statusPill(d) {
  const [, , label, pc] = devState(d);
  if (d.self) return `<span class="pill line">本机</span>`;
  if (pc === "ok") return "";
  return `<span class="pill ${pc}">${pc === "ai" ? `<span class="pulse ai live"></span>` : ""}${label}</span>`;
}

// 设备行：图标和状态点、名称和系统、路径、延迟；右侧是配对按钮或悬停时的快捷按钮
function drow(d) {
  const down = unreach(d);
  const dim = !isActive(d) || down ? "dim" : "";
  let path;
  if (!d.online) path = `${ic("clock", "sm")}${d.lastSeen}在线`;
  else if (d.noNode) path = `${ic("info", "sm")}没有运行 DeskBridge`;
  else if (d.self) path = `${ic("check", "sm")}可被其他设备和 AI 访问`;
  else if (unpaired(d)) path = `${ic("link", "sm")}未配对`;
  else path = `${ic("route", "sm")}${pathText(d)}`;
  const pill = statusPill(d) || (S.sessions[d.id] !== undefined ? `<span class="pill info">${ic("cast", "sm")}远程窗口已打开</span>` : "");
  const usable = isActive(d) && !d.self && !down && !unpaired(d);
  const lat = usable ? `${latBars(d.latency)}${d.latency} 毫秒` : "";
  const right = unpaired(d)
    ? `<button class="btn sm primary" data-a="pairDevice" data-id="${d.id}">配对</button>`
    : `${usable ? `<span class="quick">
        <button class="btn ghost icon sm" title="终端" data-go="#/devices/${d.id}/term">${ic("terminal")}</button>
        <button class="btn ghost icon sm" title="文件" data-go="#/devices/${d.id}/files">${ic("folder")}</button>
        ${d.kind !== "server" ? `<button class="btn ghost icon sm" title="桌面" data-go="#/devices/${d.id}/desk">${ic("monitor")}</button>` : ""}
      </span>` : ""}${ic("chevRight", "sm")}`;
  const nav = unpaired(d) ? `style="cursor:default"` : `data-go="#/devices/${d.id}/overview"`;
  return `<div class="drow ${dim} ${d.self ? "self" : ""}" ${nav}>
    <div class="c-name"><div class="dic">${ic(kindIcon(d))}${dot(d)}</div>
      <div style="min-width:0"><div class="nm">${d.name}${pill}</div><div class="os">${d.os}</div></div></div>
    <div class="c-path">${path}</div>
    <div class="c-lat">${lat}</div>
    <div class="c-act">${right}</div>
  </div>`;
}

// 连接方式切换：每种连接方式一页，横向切换；标签上显示状态点和设备数
function connState(id) {
  if (id === "tailscale") return noLogin() ? ["off", "未登录"] : netDown() ? ["bad", "已断开"] : ["ok", "已连接"];
  return S.set.lanOn ? ["ok", "已开启"] : ["off", "已关闭"];
}
function connTabs(match) {
  const tab = (id, icon, name) => {
    const [c, label] = connState(id);
    const n = (id === "tailscale" && noLogin()) || (id === "lan" && !S.set.lanOn) ? 0 : DEVICES.filter(d => d.via === id && match(d)).length;
    return `<a class="${S.conn === id ? "on" : ""}" data-a="setConn" data-k="${id}" title="${name} · ${label}">${ic(icon, "sm")}${name}<span class="pulse ${c}"></span>${n ? `<span class="cnt">${n}</span>` : ""}</a>`;
  };
  return `<nav class="conn-tabs">${tab("tailscale", "globe", "Tailscale")}${tab("lan", "wifi", "局域网")}
    <button class="btn ghost icon sm" title="添加连接方式" data-a="addConn">${ic("plus", "sm")}</button></nav>`;
}
A.setConn = el => { S.conn = el.dataset.k; render(); };

// 当前连接方式的状态条：状态、账号或本机地址，右侧操作
function connBar(status, right = "") {
  return `<div class="conn-bar">${status}<span class="right">${right}</span></div>`;
}

function tsPane(q, match) {
  const set = `<button class="btn ghost sm" data-go="#/settings/network">${ic("sliders", "sm")}设置</button>`;
  if (noLogin()) {
    const busy = S.tsLogging;
    return `<div class="empty conn-empty"><div class="art accent">${ic("globe", "xl")}</div>
        <h3>${busy ? "请在浏览器中完成登录" : "登录 Tailscale"}</h3>
        <p>${busy ? "已在浏览器中打开登录页，登录完成后会自动回到这里。" : "登录 Tailscale 后，同一账号下的设备会自动出现在这里"}</p>
        <div class="btns">${busy ? `<span class="spin"></span>` : `<button class="btn primary lg" data-a="tsLogin">${ic("external")}登录 Tailscale</button>`}</div>
        ${busy ? "" : `<p class="small"><a class="link" data-go="#/settings/network">使用自建控制服务器（Headscale）</a></p>`}</div>`;
  }
  const status = netDown()
    ? `<span class="pill bad"><span class="pulse bad"></span>已断开 · 正在重连（第 ${S.netTry} 次）</span>`
    : `<span class="pill ok"><span class="pulse"></span>已连接</span><span class="conn-meta">user@example.com · 本机 <span class="mono">${CONNECTORS[0].ip}</span></span>`;
  const right = (netDown() ? `<button class="btn sm" data-a="netRetry">${ic("refresh", "sm")}立即重试</button>` : "") + set;
  const mine = DEVICES.filter(d => d.via === "tailscale" && match(d));
  const active = mine.filter(isActive);
  const inactive = mine.filter(d => !isActive(d));
  const open = S.exp.inactive || q;
  return `${connBar(status, right)}
    ${netDown() ? `<div class="small muted" style="margin:-6px 0 12px">设备状态可能不是最新的，恢复后会自动刷新。局域网设备不受影响。</div>` : ""}
    ${mine.length ? `<div class="dlist">${active.map(drow).join("")}${inactive.length ? (open
      ? `${inactive.map(drow).join("")}${q ? "" : `<div class="drow more" data-a="toggleExp" data-k="inactive">${ic("chevDown", "sm")}收起</div>`}`
      : `<div class="drow more" data-a="toggleExp" data-k="inactive">${ic("chevRight", "sm")}还有 ${inactive.length} 台离线或未安装 DeskBridge 的设备</div>`) : ""}</div>`
      : noMatch(q)}`;
}

function lanPane(q, match) {
  if (!S.set.lanOn) {
    return `<div class="empty conn-empty"><div class="art">${ic("wifi", "xl")}</div><h3>局域网发现已关闭</h3>
      <p>开启后，会在同一局域网中发现运行着 DeskBridge 的设备，配对后就能使用。</p>
      <div class="btns"><button class="btn primary lg" data-a="setToggle" data-k="lanOn">开启局域网发现</button></div></div>`;
  }
  const status = `<span class="pill ok"><span class="pulse"></span>已开启</span><span class="conn-meta">本机 <span class="mono">${CONNECTORS[1].ip}</span></span>`;
  const right = `<button class="btn ghost sm" data-go="#/settings/network">${ic("sliders", "sm")}设置</button>`;
  const all = DEVICES.filter(d => d.via === "lan");
  const list = all.filter(match);
  return `${connBar(status, right)}
    ${list.length ? `<div class="dlist">${list.map(drow).join("")}</div>`
      : all.length ? noMatch(q)
      : `<div class="empty conn-empty"><div class="art">${ic("wifi", "xl")}</div><h3>没有发现其他设备</h3><p>在对方设备上打开 DeskBridge，并确保在同一局域网内。</p></div>`}`;
}

// 搜索时当前连接方式里没有结果，提示其他连接方式里的匹配
function noMatch(q) {
  const other = S.conn === "tailscale" ? "lan" : "tailscale";
  const n = q ? DEVICES.filter(d => d.via === other && (d.name + d.ip + d.os).toLowerCase().includes(q)).length : 0;
  return `<div class="empty conn-empty"><h3>没有匹配的设备</h3>${n ? `<p><a class="link" data-a="setConn" data-k="${other}">${other === "lan" ? "局域网" : "Tailscale"}中有 ${n} 台匹配</a></p>` : ""}</div>`;
}

function devicesHome() {
  const online = DEVICES.filter(d => isActive(d) && !d.self && !unreach(d) && !(noLogin() && d.via === "tailscale") && !unpaired(d)).length;
  const head = isPhone()
    ? `<div class="mtop"><h1>设备</h1><button class="btn primary icon" data-a="addDevice">${ic("plus")}</button></div>`
    : `<div class="v2-head"><div><h1>设备</h1><div class="sub">${noLogin() && !S.set.lanOn ? "还没有可用的连接方式" : `${online} 台在线 · 你和 AI 都可以操作它们`}</div></div>
        <div class="actions"><div class="search" style="width:220px;background:var(--surface)">${ic("search", "sm")}<input id="dsearch" placeholder="搜索设备" value="${esc(S.search)}" data-in="search" data-rerender="1"></div>
        <button class="btn primary" data-a="addDevice">${ic("plus")}添加设备</button></div></div>`;
  const q = S.search.trim().toLowerCase();
  const match = d => !q || (d.name + d.ip + d.os).toLowerCase().includes(q);
  let body;
  if (S.scenario === "loading") {
    body = `<div class="dlist">${Array.from({ length: 5 }, () => `<div class="drow" style="cursor:default"><div class="c-name"><div class="dic sk" style="border:0"></div><div style="flex:1"><div class="sk" style="height:13px;width:40%"></div><div class="sk" style="height:10px;width:60%;margin-top:8px"></div></div></div><div class="c-path"><div class="sk" style="height:10px;width:70%"></div></div><div class="c-lat"></div><div class="c-act"></div></div>`).join("")}</div>`;
  } else if (S.scenario === "error") {
    body = `<div class="empty"><div class="art bad">${ic("alert", "xl")}</div><h3>无法读取设备列表</h3><p>后台服务没有响应，正在重启它。</p><p><span class="errcode">ui_core_timeout</span></p><div class="btns"><button class="btn" data-a="noop" data-msg="已打开日志目录">打开日志</button><button class="btn primary" data-a="retryList">${ic("refresh")}重试</button></div></div>`;
  } else {
    body = S.conn === "lan" ? lanPane(q, match) : tsPane(q, match);
  }
  return `<div class="v2-page" data-sk="home"><div class="v2-in">${head}${connTabs(match)}${body}</div></div>`;
}

A.tsLogin = () => {
  S.tsLogging = true; render();
  toast("info", "已在浏览器中打开登录页", "登录完成后，同一账号下的设备会自动出现");
  setTimeout(() => {
    if (!S.tsLogging) return;
    S.tsLogging = false;
    if (noLogin()) { S.scenario = "normal"; toast("ok", "已登录 Tailscale", "发现 6 台设备"); render(); }
  }, 2000);
};

// ---------- 局域网配对 ----------
A.pairDevice = el => { S.modal = { type: "pair", id: el.dataset.id, code: "" }; render(); setTimeout(() => $("#paircode")?.focus(), 0); };
MODALS.pair = () => {
  const d = devById(S.modal.id);
  return modal({
    title: `配对 ${d.name}`, sub: `局域网 · <span class="mono">${d.ip}</span>`,
    body: `<p class="muted" style="line-height:1.7">在对方设备上打开 DeskBridge，会显示 6 位配对码。把它输入到下面。</p>
      <div class="field"><label>配对码</label><input class="input mono" id="paircode" maxlength="6" inputmode="numeric" placeholder="000000" value="${esc(S.modal.code)}" data-in="modal.code" autocomplete="off" style="letter-spacing:.3em;font-size:18px;max-width:200px"></div>`,
    foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn primary" data-a="pairConfirm">确认</button>`,
  });
};
A.pairConfirm = () => {
  const m = S.modal;
  if (!/^\d{6}$/.test(m.code)) return toast("bad", "配对码不正确", "请输入 6 位数字");
  const d = devById(m.id);
  d.paired = true; S.modal = null;
  toast("ok", "已配对，以后自动连接", d.name);
  render();
};
A.unpair = el => { const d = devById(el.dataset.id); d.paired = false; toast("ok", `已取消与 ${d.name} 的配对`, "需要重新输入配对码才能再次连接"); if (S.route.page === "devices") go("#/devices"); else render(); };

// ---------- 添加连接方式 ----------
A.addConn = () => { S.modal = { type: "addConn" }; render(); };
MODALS.addConn = () => modal({
  title: "添加连接方式", sub: "目前支持 Tailscale 和局域网，其余方式后续支持",
  body: `<div class="list2">${CONNECTORS.filter(c => c.st === "plan").map(c => `<div class="li" style="cursor:default;opacity:.7">
      <span class="fname">${ic(c.icon, "lg")}</span><div class="b"><div class="t">${c.name}<span class="soon">后续支持</span></div><div class="d">${c.d}</div></div></div>`).join("")}</div>`,
  foot: `<button class="btn" data-a="closeModal">知道了</button>`,
});

A.toggleExp = el => { const k = el.dataset.k; S.exp[k] = !S.exp[k]; render(); };
A.retryList = () => { S.scenario = "loading"; render(); setTimeout(() => { S.scenario = "normal"; parseRoute(); render(); toast("ok", "设备列表已刷新"); }, 1200); };

// =========================================================
// 设备页
// =========================================================
function canRemote(d) { return !unreach(d) && isActive(d) && !["server", "phone"].includes(d.kind) && !d.self && !unpaired(d); }

function devicePage(d) {
  const [, , label, pc] = devState(d);
  const line = [d.os];
  if (isActive(d) && !unreach(d) && !unpaired(d)) line.push(d.self ? `${VIA_LABEL[d.via]} · <span class="mono">${d.ip}</span>` : `${VIA_LABEL[d.via]} · ${pathText(d)} · ${d.latency} 毫秒`);
  if (!d.online) line.push(`${d.lastSeen}在线`);
  const shown = isActive(d) && !unreach(d) && !unpaired(d); // 可以操作
  const head = `<div class="dhead">
      <div class="big">${ic(kindIcon(d))}</div>
      <div style="min-width:0"><h1>${d.name}${d.self ? `<span class="pill line">本机</span>` : `<span class="pill ${pc}">${pc === "ai" ? `<span class="pulse ai live"></span>` : pc === "ok" ? `<span class="pulse"></span>` : ""}${label}</span>`}</h1>
        <div class="line">${line.join(`<span class="faint">·</span>`)}</div></div>
      <div class="actions">
        ${canRemote(d) ? `<button class="btn primary" data-a="openRemote">${ic(S.sessions[d.id] !== undefined ? "external" : "cast")}${S.sessions[d.id] !== undefined ? "切换到远程窗口" : "远程画面"}</button>` : ""}
        ${shown ? `<button class="btn icon" data-a="devMenu" title="更多">${ic("more")}</button>` : ""}
      </div></div>`;
  const tabs = shown || (unreach(d) && isActive(d)) ? `<nav class="tabs2">${TABS.map(([k, icon, l]) => {
    const blk = tabBlock(d, k);
    return `<a class="${S.route.tab === k ? "on" : ""} ${blk ? "dim" : ""}" data-go="#/devices/${d.id}/${k}">${ic(icon)}${l}</a>`;
  }).join("")}</nav>` : "";
  const top = isPhone()
    ? `<div class="mtop"><button class="btn ghost icon" data-go="#/devices">${ic("chevLeft", "lg")}</button><h2 class="muted" style="font-size:14px;font-weight:500">设备</h2>
        ${canRemote(d) ? `<button class="btn ghost icon" data-a="openRemote">${ic("cast")}</button>` : ""}${shown ? `<button class="btn ghost icon" data-a="devMenu">${ic("more")}</button>` : ""}</div>`
    : "";
  return `${top}<div class="work">
    <div class="dtop">${isPhone() ? "" : `<a class="back" data-go="#/devices">${ic("chevLeft", "sm")}设备</a>`}${head}${statusBar(d)}${tabs}</div>
    <div class="work-in" data-sk="work-${S.route.tab}">${devicePane(d)}</div>
  </div>`;
}

// 只显示一条最重要的状态，点击展开说明
function statusBar(d) {
  if (unreach(d) || !isActive(d) || unpaired(d)) return "";
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
  if (!b) return "";
  const open = S.exp["bar-" + d.id];
  return `<div class="statusbar ${b.k}" data-a="toggleExp" data-k="bar-${d.id}">${b.lead}<span class="t">${b.t}</span>
      <span class="more">${open ? "收起" : "详情"}${ic(open ? "chevDown" : "chevRight", "sm")}</span>${b.btns ? `<span class="btns">${b.btns}</span>` : ""}</div>
    ${open ? `<div class="statusbar-x">${b.x}</div>` : ""}`;
}

function devicePane(d) {
  if (unreach(d)) {
    if (S.route.tab === "overview" && isActive(d)) return `<div class="banner warn">${ic("clock")}<div class="t"><b>显示的是 2 分钟前的信息</b> <span class="d">组网恢复后自动刷新。</span></div></div>` + paneOverview(d);
    return `<div class="empty"><div class="art warn">${ic("wifiOff", "xl")}</div><h3>暂时无法连接 ${d.name}</h3><p>组网断开了，恢复后会自动重新连接。</p><div class="btns"><button class="btn" data-a="netRetry">${ic("refresh")}立即重试</button></div></div>`;
  }
  if (unpaired(d)) return `<div class="empty"><div class="art">${ic("link", "xl")}</div><h3>${d.name} 还没有配对</h3><p>在局域网中发现了它，配对后才能操作。</p><div class="btns"><button class="btn primary" data-a="pairDevice" data-id="${d.id}">配对</button></div></div>`;
  if (!d.online) return paneOffline(d);
  if (d.noNode) return paneNoNode(d);
  const blk = tabBlock(d, S.route.tab);
  if (blk) return `<div class="empty"><div class="art">${ic("monitor", "xl")}</div><h3>不可用</h3><p>${blk[1]}。</p></div>`;
  switch (S.route.tab) {
    case "term": return paneTerm(d);
    case "files": return paneFiles(d);
    case "desk": return paneDesk(d);
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
      <p>它在你的 ${VIA_LABEL[d.via]} 网络中（${d.ip}），但没有安装或没有启动 DeskBridge，所以还不能操作。</p></div>
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
  return { "proc.exec": "执行命令", "input.click": "点击", "control.preempt": "抢回桌面", "fs.write": "上传文件", "screen.capture": "截图", "ui.tree": "读取界面元素", "fs.read": "读取文件" }[a.tool] || a.tool;
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
  const caps = [["terminal", "命令", true], ["folder", "文件", true], ["image", "截图", d.kind !== "server"], ["layers", "界面元素", d.kind !== "server" && !d.outdated], ["mouse", "鼠标键盘", d.kind !== "server"],];
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
      ${d.outdated ? `<div class="banner info">${ic("info")}<div class="t">节点版本 ${d.version} 较旧，界面元素需要 0.3.0 以上。</div><div class="btns"><button class="btn sm" data-a="noop" data-msg="已发送更新请求">远程更新</button></div></div>` : ""}
      <div class="ov">
        <div><div class="small muted" style="margin-bottom:8px">延迟（最近 10 分钟）</div>${d.spark ? sparkSvg(d.spark) : ""}</div>
        <dl class="kv"><dt>连接方式</dt><dd>${VIA_LABEL[d.via]} · ${pathText(d)}</dd><dt>地址</dt><dd class="mono">${d.ip}</dd><dt>桌面会话</dt><dd>${d.session || "—"}</dd><dt>显示器</dt><dd>${d.displays ? d.displays + " 个" : "无"}</dd><dt>运行方式</dt><dd>${d.runMode || "—"}</dd></dl>
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
  if (unreach(devById(S.route.dev))) return toast("bad", "无法执行", "组网已断开");
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
    <div class="pi" data-a="optToggle" data-k="admin"><span class="check ${c.admin ? "on" : ""}"></span>以管理员身份（需要服务模式，规划中）</div>
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
        <div class="r"><span class="hide-m">${x.m}</span><span style="width:64px;text-align:right">${x.d ? "" : x.s}</span><span style="width:14px;display:inline-flex">${x.d ? ic("chevRight", "sm") : ""}</span></div></div>`).join("")}</div>`;
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
    <div class="card" style="padding:16px;display:flex;flex-direction:column;gap:12px">
      <div class="shot-box">${s.taking ? `<div class="col" style="align-items:center;color:var(--muted)"><span class="spin lg"></span>正在截图…</div>` : desktopArt({ ai: S.control === "ai" && cur.ai })}</div>
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
    S.shot.list.unshift({ t: new Date().toTimeString().slice(0, 8), by: "我", mon: S.shot.mon === "all" ? "全部显示器" : "显示器 " + S.shot.mon });
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

// =========================================================
// 远程画面
// =========================================================
// 每台设备的远程画面在独立窗口中打开（参考 ToDesk、UU 远程），可以同时操作多台。
// 手机上、或设置为“在主窗口中打开”时，覆盖在主窗口上。
function remoteBar(d, inWindow) {
  const ctl = S.control;
  const ctlText = { ai: `<span class="pill ai"><span class="pulse ai live"></span>AI 正在操作 · 你在观看</span>`, human: `<span class="pill info">${ic("hand", "sm")}你正在控制</span>`, local: `<span class="pill info">本地用户正在使用</span>`, none: `<span class="pill off">只观看</span>`, paused: `<span class="pill bad">被控端已暂停</span>` }[ctl];
  const ctlBtn = ctl === "human" ? `<button class="btn sm" data-a="release">释放控制权</button>` : ctl === "paused" || ctl === "local" ? "" : `<button class="btn sm" data-a="takeover">${ic("hand")}${ctl === "ai" ? "接管" : "开始控制"}</button>`;
  return `<div class="remote-bar">
      ${inWindow ? `<span class="logo">${ic("bridge")}</span>` : `<button class="btn sm" data-a="closeRemote">${ic("chevLeft")}返回</button>`}
      <b>${d.name}</b><span class="rstat">${latBars(d.latency)}${pathText(d)} · ${d.latency} 毫秒 · 58 帧</span>
      <span class="sep"></span>${ctlText}${ctlBtn}
      <span class="spacer"></span>
      <button class="btn sm" data-a="popRemote">${ic("sliders")}画质与显示器</button>
      <button class="btn sm icon" title="发送 Ctrl+Alt+Del" data-a="noop" data-msg="已发送 Ctrl+Alt+Del">${ic("ctrlAltDel")}</button>
      <button class="btn sm icon" title="全屏" data-a="noop" data-msg="全屏（原型未实现）">${ic("maximize")}</button>
      <button class="btn sm danger-soft" data-a="closeRemote">${ic("x")}断开</button>
      ${inWindow ? `<div class="ctl"><span>—</span><span>☐</span><span data-a="closeRemote">✕</span></div>` : ""}
    </div>`;
}

function remoteBody(d) {
  const ctl = S.control;
  return `<div class="remote-body">${desktopArt({ ai: ctl === "ai" })}
      <div class="remote-hint">${ctl === "human" ? `${ic("keyboard", "sm")}键盘和鼠标已发送到 ${d.name} · 按右 Ctrl 释放键盘捕获` : `${ic("eye", "sm")}正在观看 · 点击“${ctl === "ai" ? "接管" : "开始控制"}”后才会发送鼠标键盘`}</div>
    </div>`;
}

function viewRemote() {
  const d = devById(S.remote);
  return `<div class="remote">${remoteBar(d, false)}${remoteBody(d)}</div>`;
}

function remoteWindow(d) {
  return `<div class="remote rwin">${remoteBar(d, true)}${remoteBody(d)}</div>`;
}

A.popRemote = el => openPop(el, "remote");
POPS.remote = () => `<div class="ph2">画质</div>${["自动（推荐）", "清晰优先", "流畅优先"].map((q, i) => `<div class="pi"><span class="radio ${i === 0 ? "on" : ""}"></span>${q}</div>`).join("")}
  <div class="divider"></div><div class="ph2">显示器</div>${["显示器 1（主）", "显示器 2"].map((q, i) => `<div class="pi"><span class="radio ${i === 0 ? "on" : ""}"></span>${q}</div>`).join("")}
  <div class="divider"></div><div class="pi" style="cursor:default"><span class="muted">局域网直连 · 7.6 Mbps</span></div>`;

A.openRemote = () => {
  const d = devById(S.route.dev || "win-pc");
  if (unreach(d)) return toast("bad", "无法打开远程画面", "连接已断开");
  if (d.path !== "lan") { S.modal = { type: "noRemote", dev: d.id }; render(); return; }
  if (S.sessions[d.id] !== undefined) return A.focusRemote({ dataset: { dev: d.id } });
  if (isPhone() || S.set.remoteMode !== "独立窗口") { S.remote = d.id; render(); return; }
  const q = new URLSearchParams({ win: "remote", dev: d.id, theme: S.theme, mode: S.mode, control: d.id === "win-pc" ? S.control : "none" });
  const w = window.open(`${location.href.split(/[?#]/)[0]}?${q}`, "remote-" + d.id, "width=1280,height=760");
  if (!w) { S.remote = d.id; toast("warn", "浏览器阻止了新窗口", "改为在当前窗口中显示"); render(); return; }
  S.sessions[d.id] = w;
  toast("info", `已在新窗口打开 ${d.name} 的远程画面`, "可以同时打开多台设备，在标题栏的“远程窗口”中切换");
  render();
};
A.focusRemote = el => {
  const w = S.sessions[el.dataset.dev];
  S.pop = null;
  if (w && !w.closed) w.focus(); else toast("info", `已切换到 ${el.dataset.dev} 的远程窗口`, "原型里这是示例会话");
  render();
};
A.closeSession = el => {
  const id = el.dataset.dev, w = S.sessions[id];
  if (w && !w.closed) w.close();
  delete S.sessions[id]; S.pop = null;
  toast("ok", `已断开 ${id}`); render();
};
A.closeRemote = () => {
  if (S.win === "remote") { window.close(); toast("info", "已断开", "窗口没有自动关闭时可以直接关掉"); return; }
  S.remote = null; S.pop = null; if (S.control === "human") S.control = "none"; render();
};
A.takeover = () => { if (S.control === "ai") { S.modal = { type: "takeover" }; render(); return; } S.control = "human"; toast("info", "你已获得 win-pc 的桌面控制权"); render(); };
A.release = () => { S.control = "none"; toast("ok", "已释放控制权", "Claude Code 可以继续排队获取"); render(); };
A.stopAI = () => { S.modal = { type: "stopAI", opt: "stop" }; render(); };
A.devMenu = el => {
  const d = devById(S.route.dev);
  openMenu(el, [
    { icon: "edit", t: "重命名", msg: "重命名（原型未实现）" },
    { icon: "copy", t: "复制地址", msg: `已复制 ${d.ip}` },
    { icon: "terminal", t: "复制命令行用法", msg: `已复制：deskbridge exec ${d.name} "…"` },
    "-",
    { icon: "fileText", t: "查看节点日志", msg: "已打开节点日志" },
    { icon: "refresh", t: "重启节点", msg: `已向 ${d.name} 发送重启请求` },
    ...(d.via === "lan" && d.paired ? ["-", { icon: "link", t: "取消配对", act: "unpair", id: d.id, bad: true }] : []),
    "-",
    { icon: "trash", t: "从列表隐藏", msg: "已隐藏", bad: true },
  ]);
};
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

A.addDevice = () => { S.modal = { type: "addDevice" }; render(); };
MODALS.addDevice = () => modal({
  title: "添加设备", sub: "先在要添加的设备上安装 DeskBridge，然后二选一",
  body: `<div class="col" style="gap:16px">
      <div class="feature"><div class="fi">1</div><div><div class="t">安装 DeskBridge</div><div class="d">Windows 10/11 · Linux · <a style="cursor:pointer;text-decoration:underline" data-a="noop" data-msg="已复制下载链接">复制下载链接</a></div></div></div>
      <div class="feature"><div class="fi">${ic("globe")}</div><div><div class="t">方式一：登录同一个 Tailscale 账号</div><div class="d">在新设备上用 user@example.com 登录，它会自动出现在 Tailscale 分组，不需要输入地址或配对码</div></div></div>
      <div class="feature"><div class="fi">${ic("wifi")}</div><div><div class="t">方式二：在同一局域网中配对</div><div class="d">新设备和本机在同一局域网时，会出现在“局域网”分组。点“配对”，输入对方显示的 6 位配对码</div></div></div></div>
    <div class="row small muted" style="gap:10px"><span class="spin"></span>正在等待新设备上线…<span class="spacer"></span><a style="cursor:pointer;text-decoration:underline" data-a="simNewDevice">模拟新设备上线</a></div>`,
  foot: `<button class="btn" data-a="closeModal">完成</button>`,
});
A.simNewDevice = () => { S.modal = null; toast("ok", "发现新设备 new-pc", "Windows 11 · Tailscale · 跨网直连 · 24 毫秒"); render(); };
