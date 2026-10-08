// 设备（初版范围）：首页按连接方式横向切换（默认只有局域网，Tailscale 通过“添加连接方式”加入），每台设备一行 → 设备页（概览、终端、文件）→ 远程桌面。次要信息放进展开器、抽屉和弹出面板。

const TABS = [
  ["overview", "info", "概览"],
  ["term", "terminal", "终端"],
  ["files", "folder", "文件"],
];

function viewDevices() {
  return S.route.dev ? devicePage(curDev()) : devicesHome();
}

// =========================================================
// 首页
// =========================================================
function isActive(d) { return d.online && !d.noNode; }
const unpaired = d => d.via === "lan" && !d.self && !d.paired; // 局域网中发现、尚未配对

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
  const pill = statusPill(d) || (S.sessions[entryKey(d)] !== undefined ? `<span class="pill ok">${ic("cast", "sm")}远程窗口已打开</span>` : "");
  const usable = isActive(d) && !d.self && !down && !unpaired(d);
  const lat = usable ? `${latBars(d.latency)}${d.latency} 毫秒` : "";
  const right = unpaired(d)
    ? `<button class="btn sm primary pair-action" data-a="pairDevice" data-id="${d.id}">配对</button>`
    : `${usable ? `<span class="quick">
        <button class="btn ghost icon sm" title="终端" aria-label="打开 ${d.name} 终端" data-go="${devUrl(d, "term")}">${ic("terminal")}</button>
        <button class="btn ghost icon sm" title="文件" aria-label="打开 ${d.name} 文件" data-go="${devUrl(d, "files")}">${ic("folder")}</button>
        ${canRemote(d) ? `<button class="btn ghost icon sm" title="远程桌面" aria-label="打开 ${d.name} 远程桌面" data-a="openRemote" data-key="${entryKey(d)}">${ic("cast")}</button>` : ""}
      </span>` : ""}${ic("chevRight", "sm")}`;
  const nav = unpaired(d) ? `style="cursor:default"` : `data-go="${devUrl(d)}"`;
  return `<div class="drow ${dim} ${d.self ? "self" : ""}" data-id="${d.id}" data-via="${d.via}" ${nav}>
    <div class="c-name"><div class="dic">${ic(kindIcon(d))}${dot(d)}</div>
      <div style="min-width:0"><div class="nm">${d.name}${pill}</div><div class="os">${d.os}</div></div></div>
    <div class="c-path">${path}</div>
    <div class="c-lat">${lat}</div>
    <div class="c-act">${right}</div>
  </div>`;
}

// 连接方式切换：每种连接方式一页，横向切换；标签上显示状态点和设备数
function connState(id) {
  if (id === "tailscale") return !tsOn() ? ["off", "未登录"] : netDown() ? ["bad", "已断开"] : ["ok", "已连接"];
  return S.set.lanOn ? ["ok", "已开启"] : ["off", "已关闭"];
}
function connTabs(match) {
  const ids = ["lan", ...(tsAdded() ? ["tailscale"] : [])];
  const meta = { lan: ["wifi", "局域网"], tailscale: ["globe", "Tailscale"] };
  const tab = (id, icon, name) => {
    const [c, label] = connState(id);
    const n = (id === "tailscale" && !tsOn()) || (id === "lan" && !S.set.lanOn) ? 0 : DEVICES.filter(d => d.via === id && match(d)).length;
    return `<a class="${S.conn === id ? "on" : ""}" data-a="setConn" data-k="${id}" title="${name} · ${label}">${ic(icon, "sm")}${name}<span class="pulse ${c}"></span>${n ? `<span class="cnt">${n}</span>` : ""}</a>`;
  };
  return `<nav class="conn-tabs">${ids.map(id => tab(id, ...meta[id])).join("")}
    <button class="btn ghost ${tsAdded() ? "icon " : ""}sm" title="添加连接方式" data-a="addConn">${ic("plus", "sm")}${tsAdded() ? "" : "添加连接方式"}</button></nav>`;
}
A.setConn = el => { S.conn = el.dataset.k; render(); };
A.rowTerm = el => go(devUrl(devByKey(el.dataset.key), "term"));
A.rowFiles = el => go(devUrl(devByKey(el.dataset.key), "files"));

// 当前连接方式的状态条：状态、账号或本机地址，右侧操作
function connBar(status, right = "") {
  return `<div class="conn-bar">${status}<span class="right">${right}</span></div>`;
}

function tsPane(q, match) {
  const set = `<button class="btn ghost sm" data-go="#/settings/network">${ic("sliders", "sm")}设置</button>`;
  if (!tsOn()) {
    const busy = S.tsLogging;
    if (S.tsErr && !busy) {
      return `<div class="empty conn-empty"><div class="art bad">${ic("alert", "xl")}</div><h3>无法连接登录服务器</h3>
        <p>检查网络是否能访问控制服务器；使用自建 Headscale 时，确认服务器地址是否正确。</p><p><span class="errcode">control_unreachable</span></p>
        <div class="btns"><button class="btn" data-go="#/settings/network">${ic("sliders")}设置</button><button class="btn primary" data-a="tsLogin">${ic("refresh")}重试</button></div></div>`;
    }
    return `<div class="empty conn-empty"><div class="art accent">${ic("globe", "xl")}</div>
        <h3>${busy ? "请在浏览器中完成登录" : "登录 Tailscale"}</h3>
        <p>${busy ? "已在浏览器中打开登录页，登录完成后会自动回到这里。" : "登录 Tailscale 后，同一账号下的设备会自动出现在这里"}</p>
        <div class="btns">${busy ? `<span class="spin"></span>` : `<button class="btn primary lg" data-a="tsLogin">${ic("external")}登录 Tailscale</button>`}</div>
        ${busy ? "" : `<p class="small"><a class="link" data-go="#/settings/network">使用自建控制服务器（Headscale）</a> · <a class="link" data-a="tsRemove">不使用 Tailscale</a></p>`}</div>`;
  }
  const status = netDown()
    ? `<span class="pill bad"><span class="pulse bad"></span>已断开 · 正在重连（第 ${S.netTry} 次）</span>`
    : `<span class="pill ok"><span class="pulse"></span>已连接</span><span class="conn-meta">${CONNECTORS[0].acct} · 本机 <span class="mono">${CONNECTORS[0].ip}</span></span>`;
  const right = (netDown() ? `<button class="btn sm" data-a="netRetry">${ic("refresh", "sm")}立即重试</button>` : "") + set;
  const mine = DEVICES.filter(d => d.via === "tailscale" && match(d));
  const active = mine.filter(isActive);
  const inactive = mine.filter(d => !isActive(d));
  const open = S.exp.inactive || q;
  return `${connBar(status, right)}
    ${netDown() ? `<div class="small muted conn-note">设备状态可能不是最新的，恢复后会自动刷新。局域网页不受影响，同一局域网里的设备可以从那里连接。</div>` : ""}
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
  const right = `<button class="btn ghost sm" data-a="showPairCode" aria-haspopup="dialog">${ic("link", "sm")}本机配对码</button><button class="btn ghost sm" data-go="#/settings/network">${ic("sliders", "sm")}设置</button>`;
  const all = DEVICES.filter(d => d.via === "lan");
  const list = all.filter(match);
  const notice = S.lanNotice ? `<div class="banner info conn-note">${ic("info")}<div class="t">局域网中的其他设备能看到本机名称 <b>my-laptop</b>；对方要输入本机显示的配对码才能连接。</div><div class="btns"><button class="btn sm" data-a="lanNoticeOk">知道了</button></div></div>` : "";
  const tip = tsAdded() ? "" : `<div class="small muted conn-tip">要连接不在同一局域网的设备？<a class="link" data-a="addConn">添加 Tailscale</a></div>`;
  return `${connBar(status, right)}${notice}
    ${list.length ? `<div class="dlist">${list.map(drow).join("")}</div>`
      : all.length ? noMatch(q)
      : `<div class="empty conn-empty"><div class="art">${ic("wifi", "xl")}</div><h3>没有发现其他设备</h3><p>在对方设备上打开 DeskBridge，并确保在同一局域网内。</p></div>`}${tip}`;
}

// 本机配对码：对方在它的 DeskBridge 里输入，配对后双方保存对方的设备公钥
A.showPairCode = () => { S.modal = { type: "myCode", code: "482913" }; render(); };
A.newPairCode = () => { S.modal.code = String(100000 + Math.floor(Math.random() * 900000)); toast("ok", "已生成新的配对码", "旧的配对码立即失效"); render(); };
A.simPeerPaired = () => {
  const d = devById("media-pc", "lan");
  d.paired = true; S.modal = null; S.conn = "lan";
  toast("ok", `${d.name} 已与本机配对`, "以后在局域网中自动连接"); go("#/devices");
};
MODALS.myCode = () => modal({
  title: "本机配对码", sub: `my-laptop · 局域网 <span class="mono">${CONNECTORS[1].ip}</span>`,
  body: `<p class="muted" style="line-height:1.7">在对方设备的 DeskBridge 里，找到 my-laptop 并点“配对”，输入下面的配对码。配对成功后这个窗口会自动关闭。</p>
    <div class="paircode mono">${S.modal.code.slice(0, 3)} ${S.modal.code.slice(3)}</div>
    <div class="row small muted"><span class="spin"></span>等待对方输入… 5 分钟内有效，输错 5 次后失效<span class="spacer"></span><a class="link" data-a="simPeerPaired">模拟对方完成配对</a></div>`,
  foot: `<button class="btn" data-a="newPairCode">${ic("refresh")}重新生成</button><button class="btn primary" data-a="closeModal">完成</button>`,
});
A.lanNoticeOk = () => { S.lanNotice = false; render(); };

// 搜索时当前连接方式里没有结果，提示其他连接方式里的匹配
function noMatch(q) {
  const other = S.conn === "tailscale" ? "lan" : "tailscale";
  const n = q && (other === "lan" || tsOn()) ? DEVICES.filter(d => d.via === other && (d.name + d.ip + d.os).toLowerCase().includes(q)).length : 0;
  return `<div class="empty conn-empty"><h3>没有匹配的设备</h3>${n ? `<p><a class="link" data-a="setConn" data-k="${other}">${other === "lan" ? "局域网" : "Tailscale"}中有 ${n} 台匹配</a></p>` : ""}</div>`;
}

function devicesHome() {
  // 同一台设备可能在多种连接方式里各出现一次，在线数按设备去重
  const online = new Set(DEVICES.filter(d => isActive(d) && !d.self && !unreach(d) && !(d.via === "tailscale" && !tsAdded()) && !unpaired(d)).map(d => d.id)).size;
  const head = isPhone()
    ? `<div class="mtop"><h1>设备</h1><button class="btn primary icon" data-a="addDevice">${ic("plus")}</button></div>`
    : `<div class="v2-head"><div><h1>设备</h1><div class="sub">${!tsOn() && !S.set.lanOn ? "还没有可用的连接方式" : `${online} 台在线 · 你和 AI 都可以操作它们`}</div></div>
        <div class="actions"><div class="search" style="width:220px;background:var(--surface)">${ic("search", "sm")}<input id="dsearch" placeholder="搜索设备" value="${esc(S.search)}" data-in="search" data-rerender="1"></div>
        <button class="btn primary" data-a="addDevice">${ic("plus")}添加设备</button></div></div>`;
  const q = isPhone() ? "" : S.search.trim().toLowerCase();
  const match = d => !q || (d.name + d.ip + d.os).toLowerCase().includes(q);
  let body;
  if (S.scenario === "loading") {
    body = `<div class="dlist">${Array.from({ length: 5 }, () => `<div class="drow" style="cursor:default"><div class="c-name"><div class="dic sk" style="border:0"></div><div style="flex:1"><div class="sk" style="height:13px;width:40%"></div><div class="sk" style="height:10px;width:60%;margin-top:8px"></div></div></div><div class="c-path"><div class="sk" style="height:10px;width:70%"></div></div><div class="c-lat"></div><div class="c-act"></div></div>`).join("")}</div>`;
  } else if (S.scenario === "error") {
    body = `<div class="empty"><div class="art bad">${ic("alert", "xl")}</div><h3>无法读取设备列表</h3><p>后台服务没有响应，正在重启它。</p><p><span class="errcode">ui_core_timeout</span></p><div class="btns"><button class="btn" data-a="noop" data-msg="已打开日志目录">打开日志</button><button class="btn primary" data-a="retryList">${ic("refresh")}重试</button></div></div>`;
  } else {
    body = S.conn === "tailscale" && tsAdded() ? tsPane(q, match) : lanPane(q, match);
  }
  return `<div class="v2-page" data-sk="home"><div class="v2-in">${head}${S.scenario === "normal" ? connTabs(match) : ""}${body}</div></div>`;
}

A.tsLogin = () => {
  if (S.tsLogging) return; // 登录进行中，不重复打开
  if (!tsAdded()) S.ts = "login";
  if (S.route.page === "devices") S.conn = "tailscale";
  S.modal = null; S.tsLogging = true; S.tsErr = false; render();
  toast("info", "已在浏览器中打开登录页", "登录完成后，同一账号下的设备会自动出现");
  setTimeout(() => {
    if (!S.tsLogging) return;
    S.tsLogging = false;
    // 原型：流程“Tailscale 登录失败”会让下一次登录失败一次
    if (S.tsFailNext) { S.tsFailNext = false; S.tsErr = true; toast("bad", "无法连接登录服务器", "control_unreachable"); render(); return; }
    if (!tsOn()) {
      S.ts = "on"; if (S.scenario === "firstrun") S.scenario = "normal";
      toast("ok", "已登录 Tailscale", `发现 ${DEVICES.filter(d => d.via === "tailscale" && !d.self).length} 台设备`); render();
    }
  }, 2000);
};

// 退出登录：保留 Tailscale 标签，回到登录组件；移除：标签和设备列表一起消失，局域网页不受影响
A.tsLogout = () => { S.ts = "login"; S.tsLogging = false; closeSessions("tailscale"); toast("ok", "已退出 Tailscale", "Tailscale 页里的设备暂时不可用"); render(); };
A.tsRemove = () => { S.ts = "none"; S.tsLogging = false; S.tsErr = false; S.conn = "lan"; closeSessions("tailscale"); toast("ok", "已移除 Tailscale", "可以随时从“添加连接方式”重新添加"); render(); };
A.askTsLogout = () => confirmBox({ title: "退出 Tailscale？", body: "退出后，Tailscale 页里的设备暂时不可用，经 Tailscale 打开的远程窗口会断开。局域网页不受影响。", ok: "退出登录", act: "tsLogout", danger: true });
A.askTsRemove = () => confirmBox({ title: "移除 Tailscale？", body: "移除后，设备页不再显示 Tailscale 标签和其中的设备，经 Tailscale 打开的远程窗口会断开。局域网页不受影响，以后可以重新添加。", ok: "移除", act: "tsRemove", danger: true });
function closeSessions(via) {
  Object.keys(S.sessions).filter(k => k.endsWith("@" + via)).forEach(k => { const w = S.sessions[k]; if (w && !w.closed) w.close(); delete S.sessions[k]; });
  if (S.remote && S.remote.endsWith("@" + via) && S.win !== "remote") S.remote = null;
}

// ---------- 局域网配对 ----------
const PAIR_TRIES = 5; // 配对码最多尝试次数（原型暂定，任务 28 中确定）
A.pairDevice = el => { S.modal = { type: "pair", id: el.dataset.id, code: "", err: "", tries: 0, busy: false }; render(); setTimeout(() => $("#paircode")?.focus(), 0); };
MODALS.pair = () => {
  const d = devById(S.modal.id, "lan");
  const locked = S.modal.tries >= PAIR_TRIES;
  return modal({
    title: `配对 ${d.name}`, sub: `局域网 · <span class="mono">${d.ip}</span>`,
    body: `<p class="muted" style="line-height:1.7">在对方设备上打开 DeskBridge，会显示 6 位配对码。把它输入到下面。</p>
      <div class="field"><label>配对码</label><input class="input mono ${S.modal.err ? "invalid" : ""}" id="paircode" maxlength="6" inputmode="numeric" placeholder="000000" value="${esc(S.modal.code)}" data-in="modal.code" autocomplete="off" ${locked ? "disabled" : ""} style="letter-spacing:.3em;font-size:18px;max-width:200px">
        ${S.modal.err ? `<div class="field-err">${S.modal.err}</div>` : `<div class="hint">原型中输入 123456 会配对成功，其他数字模拟配对码错误</div>`}</div>`,
    foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn primary ${S.modal.busy || locked ? "disabled" : ""}" data-a="pairConfirm">${S.modal.busy ? `<span class="spin"></span>正在配对` : "确认"}</button>`,
  });
};
A.pairConfirm = () => {
  const m = S.modal;
  if (m.busy || m.tries >= PAIR_TRIES) return;
  if (!/^\d{6}$/.test(m.code)) { m.err = "请输入 6 位数字"; render(); return; }
  m.busy = true; m.err = ""; render();
  setTimeout(() => {
    if (S.modal !== m) return;
    m.busy = false;
    if (m.code === "123456") {
      const d = devById(m.id, "lan");
      d.paired = true; S.modal = null;
      toast("ok", "已配对，以后自动连接", d.name); render(); return;
    }
    m.tries += 1; m.code = "";
    m.err = m.tries >= PAIR_TRIES
      ? `尝试次数过多，这个配对码已失效。请在对方设备上重新生成配对码（pairing_failed）`
      : `配对码不正确或已过期，还可以尝试 ${PAIR_TRIES - m.tries} 次`;
    render(); $("#paircode")?.focus();
  }, 800);
};
A.unpair = el => { const d = devById(el.dataset.id, "lan"); d.paired = false; toast("ok", `已取消与 ${d.name} 的配对`, "需要重新输入配对码才能再次连接"); if (S.route.page === "devices") go("#/devices"); else render(); };

// ---------- 添加连接方式 ----------
A.addConn = () => { S.modal = { type: "addConn" }; render(); };
MODALS.addConn = () => modal({
  title: "添加连接方式", sub: "局域网默认开启；要连接不在同一局域网的设备，添加 Tailscale",
  body: `<div class="list2">${CONNECTORS.filter(c => c.id !== "lan").map(c => {
      const plan = c.st === "plan", added = c.id === "tailscale" && tsAdded();
      const r = plan ? `<span class="soon">后续支持</span>` : added ? `<span class="pill line">已添加</span>` : `<button class="btn sm primary" data-a="tsLogin">${ic("external", "sm")}添加并登录</button>`;
      return `<div class="li" style="cursor:default;${plan ? "opacity:.7" : ""}">
      <span class="fname">${ic(c.icon, "lg")}</span><div class="b"><div class="t">${c.name}</div><div class="d">${c.d}</div></div><div class="r">${r}</div></div>`;
    }).join("")}</div>`,
  foot: `<button class="btn" data-a="closeModal">关闭</button>`,
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
        ${canRemote(d) ? `<button class="btn primary" data-a="openRemote">${ic(S.sessions[entryKey(d)] !== undefined ? "external" : "cast")}${S.sessions[entryKey(d)] !== undefined ? "切换到远程窗口" : "远程桌面"}</button>` : ""}
        ${shown ? `<button class="btn icon" data-a="devMenu" title="更多">${ic("more")}</button>` : ""}
      </div></div>`;
  const tabs = shown || (unreach(d) && isActive(d)) ? `<nav class="tabs2">${TABS.map(([k, icon, l]) => {
    return `<a class="${S.route.tab === k ? "on" : ""}" data-go="${devUrl(d, k)}">${ic(icon)}${l}</a>`;
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
  const id = `data-id="${d.id}"`;
  let b = {
      ai: { k: "ai", lead: `<span class="pulse ai live"></span>${avatar("claude", "sm")}`, t: `<b>Claude Code 正在操作桌面</b> · 2 分 14 秒`,
        btns: `<button class="btn sm" data-a="takeover" ${id}>${ic("hand")}接管</button><button class="btn sm danger-soft" data-a="stopAI" ${id}>${ic("stop")}停止</button>`,
        x: "来自 linux-server。本地用户动一下鼠标键盘，AI 会立即停下；也可以在这里接管或紧急停止。命令和文件操作不受控制权影响。" },
      local: { k: "info", lead: ic("mouse"), t: `<b>本地用户正在使用</b> · Claude Code 在排队`, btns: "",
        x: "AI 的鼠标键盘操作已暂停，本地停止操作 3 秒后自动继续。可以在“设置 → 桌面控制权”中修改这个时间。" },
      human: { k: "info", lead: ic("hand"), t: `<b>你正在远程操作桌面</b>`, btns: `<button class="btn sm" data-a="release" ${id}>释放</button>`,
        x: "Claude Code 的鼠标键盘操作在排队。你 60 秒不操作会自动释放。" },
      paused: { k: "bad", lead: ic("pause"), t: `<b>被控端已暂停所有远程操作</b>`, btns: "",
        x: "本地用户在 13:58 按下了暂停。新的远程操作会被拒绝，正在运行的后台命令继续。只能在被控端解除。" },
    }[ctlOf(d)] || null;
  const st = S.stopped[d.id];
  if (!b && st) b = { k: "bad", lead: ic("stop"), t: `<b>已紧急停止 ${st.who}</b> · 它在这台设备上的请求会被拒绝`,
    btns: `<button class="btn sm" data-a="resumeAI" ${id}>恢复</button>`,
    x: `在 ${st.t} 停止${st.kill ? "，并结束了它启动的 2 个进程" : ""}。恢复后它可以重新发起请求；已取消的操作不会自动重做。也可以在“设置 → 桌面控制权”中恢复。` };
  if (!b && d.path === "relay") b = { k: "warn", lead: ic("route"), t: `<b>经中继连接，延迟较高</b> · 远程桌面为截图模式`, btns: "",
    x: `当前经${d.relay}的中继连接，延迟 ${d.latency} 毫秒。命令和文件可以正常使用；实时画面暂时只支持局域网直连，其他路径下远程桌面改为截图模式：可以点击和输入，每次操作后刷新截图。<a data-a="noop" data-msg="正在重新探测直连路径…" style="text-decoration:underline;cursor:pointer">重新探测</a>` };
  if (!b) return "";
  const open = S.exp["bar-" + d.id];
  return `<div class="statusbar ${b.k}" data-a="toggleExp" data-k="bar-${d.id}">${b.lead}<span class="t">${b.t}</span>
      <span class="more">${open ? "收起" : "详情"}${ic(open ? "chevDown" : "chevRight", "sm")}</span>${b.btns ? `<span class="btns">${b.btns}</span>` : ""}</div>
    ${open ? `<div class="statusbar-x">${b.x}</div>` : ""}`;
}

function devicePane(d) {
  const why = unreachWhy(d);
  if (why === "lanoff") return `<div class="empty"><div class="art">${ic("wifi", "xl")}</div><h3>局域网发现已关闭</h3><p>开启后才能通过局域网连接 ${d.name}。</p><div class="btns"><button class="btn primary" data-a="setToggle" data-k="lanOn">开启局域网发现</button></div></div>`;
  if (why === "tsoff") return `<div class="empty"><div class="art">${ic("globe", "xl")}</div><h3>Tailscale 未登录</h3><p>登录后才能通过 Tailscale 连接 ${d.name}。</p><div class="btns"><button class="btn primary" data-a="tsLogin">${ic("external")}登录 Tailscale</button></div></div>`;
  if (unreach(d)) {
    if (S.route.tab === "overview" && isActive(d)) return `<div class="banner warn">${ic("clock")}<div class="t"><b>显示的是 2 分钟前的信息</b> <span class="d">组网恢复后自动刷新。</span></div></div>` + paneOverview(d);
    return `<div class="empty"><div class="art warn">${ic("wifiOff", "xl")}</div><h3>暂时无法连接 ${d.name}</h3><p>组网断开了，恢复后会自动重新连接。</p><div class="btns"><button class="btn" data-a="netRetry">${ic("refresh")}立即重试</button></div></div>`;
  }
  if (unpaired(d)) return `<div class="empty"><div class="art">${ic("link", "xl")}</div><h3>${d.name} 还没有配对</h3><p>在局域网中发现了它，配对后才能操作。</p><div class="btns"><button class="btn primary" data-a="pairDevice" data-id="${d.id}">配对</button></div></div>`;
  if (!d.online) return paneOffline(d);
  if (d.noNode) return paneNoNode(d);
  switch (S.route.tab) {
    case "term": return paneTerm(d);
    case "files": return paneFiles(d);
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
  const tile = (go, icon, t, desc, off) => `<div class="tile ${off ? "off" : ""}" ${go.startsWith("#") ? `data-go="${go}"` : `data-a="${go}" data-id="${d.id}" data-dev="${d.id}" data-key="${entryKey(d)}"`}><span class="ti">${ic(icon)}</span><div><div class="tt">${t}</div><div class="td">${desc}</div></div></div>`;
  const c = CTL[ctlOf(d)];
  const desk = d.kind === "server" ? ["无桌面", "—"] : ctlOf(d) === "none" ? [c.stat, d.session || "—"] : [c.stat, c.sub];
  const recent = ACTIVITY.filter(a => a.dev === d.id).slice(0, 3);
  const caps = [["terminal", "命令", true], ["folder", "文件", true], ["image", "截图", d.kind !== "server"], ["layers", "界面元素", d.kind !== "server" && !d.outdated], ["mouse", "鼠标键盘", d.kind !== "server"],];
  return `
    <div class="tiles">
      ${tile(devUrl(d, "term"), "terminal", "终端", "执行命令")}
      ${tile(devUrl(d, "files"), "folder", "文件", "浏览和传输")}
      ${tile("openRemote", "cast", "远程桌面", !canRemote(d) ? "没有图形桌面" : d.path === "lan" ? "实时画面" : "截图模式", !canRemote(d))}
      ${tile("actForDev", "history", "操作记录", "AI 和人做过什么")}
    </div>
    <div class="stats">
      <div class="stat"><div class="l">${ic("route", "sm")}连接</div><div class="v">${d.self ? "本机" : `${d.latency} 毫秒`}</div><div class="s">${pathText(d)}${d.latency >= 150 ? " · 波动较大" : " · 稳定"}</div></div>
      <div class="stat"><div class="l">${ic("hand", "sm")}桌面</div><div class="v">${desk[0]}</div><div class="s">${desk[1]}</div></div>
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

// ---------- 终端（每台设备一份状态） ----------
function cmdOf(id) {
  if (!S.cmds[id]) {
    const d = devById(id), win = isWin(d);
    S.cmds[id] = { shell: win ? "PowerShell 7" : "bash", text: win ? "Get-ChildItem D:\\data -Recurse | Measure-Object -Sum Length" : "systemctl status deskbridge",
      status: "idle", lines: [], timeout: true, admin: false };
    // 示例：win-pc 上有一条正在运行的命令
    if (id === "win-pc") Object.assign(S.cmds[id], { status: "run", lines: CMD_OUTPUTS.default.slice(0, 12), started: Date.now() - 8400, ran: S.cmds[id].text });
  }
  return S.cmds[id];
}

function termStatus(c) {
  return {
    idle: `<span class="st faint" style="font-weight:500">就绪</span>`,
    run: `<span class="st"><span class="spin"></span>运行中</span><span id="runtime">${fmtSec(Date.now() - c.started)}</span>`,
    ok: `<span class="st" style="color:var(--ok)">${ic("check", "sm")}完成</span><span>退出码 0 · ${c.dur}</span>`,
    cancel: `<span class="st" style="color:var(--muted)">${ic("stop", "sm")}已取消</span><span>${c.dur}</span>`,
    err: `<span class="st" style="color:var(--bad)">${ic("x", "sm")}失败</span><span>退出码 ${c.code} · ${c.dur}</span>`,
    timeout: `<span class="st" style="color:var(--warn)">${ic("clock", "sm")}超时</span><span>超过 120 秒，已结束进程树</span>`,
  }[c.status];
}

function termHTML(d) {
  const c = cmdOf(d.id);
  const lines = c.lines.map(([k, t]) => {
    const html = esc(t).replace(/&lt;hl&gt;(.*?)&lt;\/hl&gt;/g, '<span class="hl">$1</span>');
    return k ? `<span class="${k}">${html}</span>` : html;
  }).join("\n");
  const head = c.status === "idle" ? "" : `<span class="dim">$ ${esc(c.ran || c.text)}</span>\n`;
  const tail = c.status === "run" ? ` <span class="cursor"></span>`
    : c.status === "cancel" ? `\n<span class="warn">^C 已取消，进程树已结束</span>`
    : c.status === "timeout" ? `\n<span class="warn">已超时（120 秒），进程树已结束</span>`
    : c.status === "err" ? `\n<span class="err">退出码 ${c.code}</span>` : "";
  const hint = `输入命令后按回车执行。输出会实时显示${isWin(d) ? "，Windows 上的中文会自动转成 UTF-8" : ""}。`;
  return `<div class="out" id="out">${c.status === "idle" ? `<span class="dim">${hint}</span>` : head + lines + tail}</div>`;
}

function termBar(d) {
  const c = cmdOf(d.id);
  return `<div class="bar" id="tbar">${termStatus(c)}
    <span class="tools">
      <button class="btn ghost sm" data-a="popHistory">${ic("history")}历史</button>
      <button class="btn ghost sm" data-a="popOptions">${ic("sliders")}选项</button>
      <button class="btn ghost icon sm" data-a="noop" data-msg="已复制输出" title="复制输出">${ic("copy")}</button>
      ${c.status !== "idle" && c.status !== "run" ? `<button class="btn ghost icon sm" data-a="clearCmd" title="清空">${ic("trash")}</button>` : ""}
    </span></div>`;
}

function paneTerm(d) {
  const c = cmdOf(d.id);
  const shells = isWin(d) ? ["PowerShell 7", "命令提示符"] : ["bash", "sh"];
  return `<div class="term2" id="term">
    ${termBar(d)}
    ${termHTML(d)}
    <div class="inrow">
      <span class="sh" data-a="cycleShell" data-v="${shells.join("|")}" title="切换命令行">${c.shell}${ic("chevDown", "sm")}</span>
      <span class="prompt">›</span>
      <input id="cmdin" value="${esc(c.text)}" data-in="cmds.${d.id}.text" placeholder="输入命令，回车执行" autocomplete="off" spellcheck="false">
      ${c.status === "run" ? `<button class="btn danger-soft" data-a="cancelCmd">${ic("stop")}取消</button>` : `<button class="btn primary" data-a="runCmd">${ic("play")}执行</button>`}
    </div>
  </div>`;
}

const cmdTimers = {};
function updateTerm(id) {
  if (S.route.dev !== id || S.route.tab !== "term") return;
  const o = $("#out"); if (o) { o.outerHTML = termHTML(devById(id)); const n = $("#out"); n.scrollTop = n.scrollHeight; }
}
A.runCmd = () => {
  const d = curDev(), c = cmdOf(d.id);
  if (unreach(d)) return toast("bad", "无法执行", "组网已断开");
  if (ctlOf(d) === "paused") return toast("bad", "被拒绝", "被控端已暂停远程操作");
  if (!c.text.trim()) return;
  clearInterval(cmdTimers[d.id]);
  // 原型：含 git 的命令模拟失败；含 winget 或 sleep 的命令模拟超时（120 秒缩短为 3 秒）
  const kind = /\bgit\b/i.test(c.text) ? "err" : /winget|sleep/i.test(c.text) ? "timeout" : "ok";
  const src = kind === "err" ? CMD_OUTPUTS.err : kind === "timeout" ? CMD_OUTPUTS.slow : /ping/i.test(c.text) ? CMD_OUTPUTS.ping : isWin(d) ? CMD_OUTPUTS.default : CMD_OUTPUTS.linux;
  Object.assign(c, { status: "run", lines: [], started: Date.now(), ran: c.text });
  render();
  let i = 0;
  const finish = () => {
    clearInterval(cmdTimers[d.id]);
    c.dur = fmtSec(Date.now() - c.started);
    if (kind === "err") Object.assign(c, { status: "err", code: 1 });
    else if (kind === "timeout") { c.status = "timeout"; toast("warn", "命令超时，已结束进程树", "原型中把 120 秒缩短为 3 秒"); }
    else c.status = "ok";
    if (S.route.dev === d.id) render();
  };
  cmdTimers[d.id] = setInterval(() => {
    if (c.status !== "run") return clearInterval(cmdTimers[d.id]);
    if (i < src.length) { c.lines.push(src[i++]); updateTerm(d.id); }
    if (i >= src.length && (kind !== "timeout" || Date.now() - c.started > 3000)) finish();
  }, 450);
};
A.cancelCmd = () => {
  const id = S.route.dev, c = cmdOf(id);
  clearInterval(cmdTimers[id]);
  c.status = "cancel"; c.dur = fmtSec(Date.now() - c.started);
  toast("ok", "已取消", "命令及其子进程已结束");
  render();
};
A.clearCmd = () => { Object.assign(cmdOf(S.route.dev), { status: "idle", lines: [] }); render(); };
A.fillCmd = el => { cmdOf(S.route.dev).text = el.dataset.c; S.pop = null; render(); $("#cmdin")?.focus(); };
A.cycleShell = el => { const c = cmdOf(S.route.dev), s = el.dataset.v.split("|"); c.shell = s[(s.indexOf(c.shell) + 1) % s.length]; render(); };
A.optToggle = el => { const c = cmdOf(S.route.dev); c[el.dataset.k] = !c[el.dataset.k]; render(); };
A.popHistory = el => openPop(el, "history");
A.popOptions = el => openPop(el, "options");

POPS.history = () => {
  const os = isWin(curDev()) ? "win" : "linux";
  return `<div class="ph2">最近执行</div>${RECENT_RUNS.filter(r => r.os === os).map(r => `<div class="pi" data-a="fillCmd" data-c="${esc(r.c)}"><span class="pill ${r.st === "ok" ? "ok" : r.st === "err" ? "bad" : "off"}">${r.st === "ok" ? "成功" : r.st === "err" ? `退出码 ${r.code}` : "已取消"}</span><span class="c">${esc(r.c)}</span><span class="w">${r.w}</span></div>`).join("")}`;
};
POPS.options = () => {
  const d = curDev(), c = cmdOf(d.id);
  return `<div class="ph2">执行选项</div>
    <div class="pi" data-a="optToggle" data-k="timeout"><span class="check ${c.timeout ? "on" : ""}"></span>超时 120 秒后结束</div>
    <div class="pi" data-a="optToggle" data-k="admin"><span class="check ${c.admin ? "on" : ""}"></span>以管理员身份（需要服务模式，规划中）</div>
    <div class="divider"></div>
    <div class="pi" style="cursor:default">${ic("folder", "sm")}<span class="muted">工作目录</span><span class="c" style="text-align:right">${esc(fsOf(d.id).path)}</span></div>`;
};

// ---------- 文件（每台设备一份状态；Windows 用 \，Linux 用 /） ----------
function fsOf(id) {
  if (!S.fss[id]) S.fss[id] = { path: isWin(devById(id)) ? "D:\\data\\projects" : "/etc", upload: null };
  return S.fss[id];
}
const fsTree = d => isWin(d) ? FILES : FILES_LINUX;
// 路径拆成面包屑：[[名称, 完整路径], …]
function fsCrumbs(d, path) {
  if (!isWin(d)) {
    const p = path.split("/").filter(Boolean);
    return [["/", "/"], ...p.map((n, i) => [n, "/" + p.slice(0, i + 1).join("/")])];
  }
  const p = path.split("\\").filter(Boolean);
  return p.map((n, i) => [n, p.slice(0, i + 1).join("\\") + (i === 0 ? "\\" : "")]);
}
function fsChild(d, path, name) {
  if (isWin(d)) return path.endsWith("\\") ? path + name : path + "\\" + name;
  return path === "/" ? "/" + name : path + "/" + name;
}

function paneFiles(d) {
  const f = fsOf(d.id);
  const crumbs = fsCrumbs(d, f.path).map(([n, p]) => `<a data-a="cd" data-path="${esc(p)}">${esc(n)}</a>`).join(`<span class="sep">${ic("chevRight", "sm")}</span>`);
  const list = fsTree(d)[f.path];
  const priv = isWin(d) ? "D:\\data\\private" : null;
  let body;
  if (f.path === priv) {
    body = `<div class="empty"><div class="art bad">${ic("lock", "xl")}</div><h3>拒绝访问</h3><p>节点以普通用户身份运行，没有读取这个文件夹的权限。</p><p><span class="errcode">permission_denied</span></p><div class="btns"><button class="btn" data-a="cdUp">返回上一级</button></div></div>`;
  } else if (!list) {
    body = `<div class="empty"><div class="art">${ic("folder", "xl")}</div><h3>空文件夹</h3><p>把文件拖到这里即可上传。</p></div>`;
  } else {
    const rows = list.concat(isWin(d) && f.path === "D:\\data" ? [{ n: "private", d: true, m: "—" }] : []);
    body = `<div class="list2">${rows.map(x => `<div class="li" data-a="${x.d ? "cd" : "fileDrawer"}" data-path="${esc(fsChild(d, f.path, x.n))}" data-n="${esc(x.n)}">
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
A.cd = el => { fsOf(S.route.dev).path = el.dataset.path; render(); };
A.cdUp = () => {
  const d = curDev(), f = fsOf(d.id), c = fsCrumbs(d, f.path);
  if (c.length > 1) { f.path = c[c.length - 2][1]; render(); }
};
A.fileDrawer = el => { S.drawer = { type: "file", n: el.dataset.n }; render(); };
const upTimers = {};
A.upload = () => {
  const d = curDev(), id = d.id, f = fsOf(id);
  if (unreach(d)) return toast("bad", "无法上传", "连接已断开");
  if (ctlOf(d) === "paused") return toast("bad", "被拒绝", "被控端已暂停远程操作");
  f.upload = { n: "报告-2026Q3.xlsx", p: 0 }; render();
  clearInterval(upTimers[id]);
  upTimers[id] = setInterval(() => {
    if (!f.upload) return clearInterval(upTimers[id]);
    f.upload.p = Math.min(100, f.upload.p + 17);
    if (f.upload.p >= 100) { clearInterval(upTimers[id]); f.upload = null; toast("ok", "上传完成", `报告-2026Q3.xlsx → ${id}:${f.path}`); }
    render();
  }, 400);
};
A.cancelUpload = () => { const id = S.route.dev; clearInterval(upTimers[id]); fsOf(id).upload = null; toast("ok", "已取消上传"); render(); };

DRAWERS.file = () => {
  const d = curDev();
  const x = (fsTree(d)[fsOf(d.id).path] || []).find(f => f.n === S.drawer.n) || {};
  return drawer({
    title: esc(x.n), sub: `${x.s || ""} · 修改于 ${x.m || ""}`,
    body: x.text ? `<div class="codebox" style="font-size:12.5px;line-height:1.7">${esc(x.text)}</div>` : `<div class="empty"><div class="art">${ic("file", "xl")}</div><p>这种文件不支持预览。</p></div>`,
    foot: `<button class="btn primary" data-a="noop" data-msg="已保存到 下载 文件夹">${ic("download")}下载</button><button class="btn" data-a="noop" data-msg="已复制路径">${ic("copy")}复制路径</button><span class="spacer"></span><button class="btn ghost danger-soft icon" data-a="noop" data-msg="删除需要确认（原型未实现）">${ic("trash")}</button>`,
  });
};

// ---------- 桌面示意图（远程桌面、操作记录中的截图） ----------
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

// =========================================================
// 远程桌面
// =========================================================
// 每台设备的远程桌面在独立窗口中打开（参考 ToDesk、UU 远程），可以同时操作多台。
// 手机上、或设置为“在主窗口中打开”时，覆盖在主窗口上。
// 局域网直连时是实时画面；其他路径（如中继）降级为截图模式，可以点击和输入，每次操作后刷新（ADR-0006）
const shotMode = d => d.path !== "lan";

function remoteBar(d, inWindow) {
  const ctl = ctlOf(d), c = CTL[ctl], id = `data-id="${d.id}"`;
  const ctlText = `<span class="pill ${c.cls}">${c.live ? `<span class="pulse ai live"></span>` : ""}${ctl === "human" ? ic("hand", "sm") : ""}${c.label}${ctl === "human" ? "" : " · 你在观看"}</span>`;
  const ctlBtn = ctl === "human" ? `<button class="btn sm" data-a="release" ${id}>释放控制权</button>` : ctl === "paused" || ctl === "local" ? "" : `<button class="btn sm" data-a="takeover" ${id}>${ic("hand")}${ctl === "ai" ? "接管" : "开始控制"}</button>`;
  return `<div class="remote-bar">
      ${inWindow ? `<span class="logo">${ic("bridge")}</span>` : `<button class="btn sm" data-a="closeRemote">${ic("chevLeft")}返回</button>`}
      <b>${d.name}</b><span class="rstat">${S.connecting[entryKey(d)] ? `<span class="spin"></span>连接中 · ${VIA_LABEL[d.via]}` : `${latBars(d.latency)}${VIA_LABEL[d.via]} · ${pathText(d)} · ${d.latency} 毫秒 · ${shotMode(d) ? "截图模式" : "58 帧"}`}</span>
      <span class="sep"></span>${ctlText}${ctlBtn}
      <span class="spacer"></span>
      ${shotMode(d) ? `<button class="btn sm" data-a="noop" data-msg="已刷新截图">${ic("refresh")}刷新</button>` : ""}<button class="btn sm" data-a="popRemote" data-key="${entryKey(d)}">${ic("sliders")}${shotMode(d) ? "刷新与显示器" : "画质与显示器"}</button>
      <button class="btn sm icon" title="发送 Ctrl+Alt+Del" data-a="noop" data-msg="已发送 Ctrl+Alt+Del">${ic("ctrlAltDel")}</button>
      <button class="btn sm icon" title="全屏" data-a="noop" data-msg="全屏（原型未实现）">${ic("maximize")}</button>
      <button class="btn sm danger-soft" data-a="closeRemote">${ic("x")}断开</button>
      ${inWindow ? `<div class="ctl"><span>—</span><span>☐</span><span data-a="closeRemote">✕</span></div>` : ""}
    </div>`;
}

function remoteBody(d) {
  if (S.connecting[entryKey(d)]) {
    return `<div class="remote-body"><div class="remote-wait"><span class="spin lg"></span><b>正在连接 ${d.name}…</b><span>${VIA_LABEL[d.via]} · ${pathText(d)}</span></div></div>`;
  }
  const ctl = ctlOf(d);
  const elev = S.elevated ? `<div class="remote-notice">${ic("lock", "sm")}<div><b>前台是管理员窗口，无法发送鼠标键盘</b><div>DeskBridge 以普通用户身份运行，Windows 不允许它操作管理员窗口。可以在被控端本机处理，或切换到其他窗口后继续。<span class="errcode">elevated_target</span></div></div></div>` : "";
  const shot = shotMode(d) ? `${ic("image", "sm")}截图模式：经${d.path === "relay" ? `${d.relay}的中继` : pathText(d)}连接，实时画面不可用 · 截于 14:03:22 · 每 5 秒自动刷新，操作后立即刷新 · ` : "";
  return `<div class="remote-body">${elev}${desktopArt({ ai: ctl === "ai" })}
      <div class="remote-hint">${shot}${ctl === "human" ? `${ic("keyboard", "sm")}键盘和鼠标已发送到 ${d.name} · 按右 Ctrl 释放键盘捕获` : `${ic("eye", "sm")}正在观看 · 点击“${ctl === "ai" ? "接管" : "开始控制"}”后才会发送鼠标键盘`}</div>
    </div>`;
}

function viewRemote() {
  const d = devByKey(S.remote);
  return `<div class="remote">${remoteBar(d, false)}${remoteBody(d)}</div>`;
}

function remoteWindow(d) {
  return `<div class="remote rwin">${remoteBar(d, true)}${remoteBody(d)}</div>`;
}

A.popRemote = el => { S.popDev = el.dataset.key; openPop(el, "remote"); };
POPS.remote = () => shotMode(devByKey(S.popDev)) ? `<div class="ph2">自动刷新</div>${["每 2 秒", "每 5 秒", "每 15 秒", "不自动刷新"].map(q => remoteOption("refresh", q)).join("")}
  <div class="divider"></div><div class="ph2">显示器</div>${["显示器 1（主）", "显示器 2"].map(q => remoteOption("monitor", q)).join("")}
  <div class="divider"></div><div class="pi" style="cursor:default"><span class="muted">实时画面只在局域网直连时可用</span></div>` : `<div class="ph2">画质</div>${["自动（推荐）", "清晰优先", "流畅优先"].map(q => remoteOption("quality", q)).join("")}
  <div class="divider"></div><div class="ph2">显示器</div>${["显示器 1（主）", "显示器 2"].map(q => remoteOption("monitor", q)).join("")}
  <div class="divider"></div><div class="pi" style="cursor:default"><span class="muted">局域网直连 · 7.6 Mbps</span></div>`;

A.openRemote = el => {
  const d = el?.dataset?.key ? devByKey(el.dataset.key) : curDev();
  const k = entryKey(d);
  if (unreach(d)) return toast("bad", "无法打开远程桌面", "连接已断开");
  if (S.sessions[k] !== undefined) return A.focusRemote({ dataset: { key: k } });
  if (isPhone() || S.set.remoteMode !== "独立窗口") { S.remote = k; startConnecting(k); render(); return; }
  const q = new URLSearchParams({ win: "remote", dev: d.id, via: d.via, theme: S.theme, mode: S.mode, control: ctlOf(d) });
  const w = window.open(`${location.href.split(/[?#]/)[0]}?${q}`, "remote-" + k, "width=1280,height=760");
  if (!w) { S.remote = k; startConnecting(k); toast("warn", "浏览器阻止了新窗口", "改为在当前窗口中显示"); render(); return; }
  S.sessions[k] = w;
  toast("info", `已在新窗口打开 ${d.name} 的远程桌面`, shotMode(d) ? "经中继连接，使用截图模式" : "可以同时打开多台设备，在标题栏的“远程窗口”中切换");
  render();
};
// 打开远程桌面后先显示“连接中”，原型里 1.2 秒后连上
function startConnecting(k) {
  S.connecting[k] = true;
  setTimeout(() => { delete S.connecting[k]; render(); }, 1200);
}
A.focusRemote = el => {
  const w = S.sessions[el.dataset.key];
  S.pop = null;
  if (w && !w.closed) w.focus(); else toast("info", `已切换到 ${devByKey(el.dataset.key).name} 的远程窗口`, "原型里这是示例会话");
  render();
};
function remoteOption(key, value) {
  const on = S.remoteOpts[key] === value;
  return `<button class="pi" data-a="remoteOpt" data-k="${key}" data-v="${value}" aria-pressed="${on}"><span class="radio ${on ? "on" : ""}"></span>${value}</button>`;
}
A.remoteOpt = el => { S.remoteOpts[el.dataset.k] = el.dataset.v; toast("ok", `已选择 ${el.dataset.v}`); render(); };
A.closeSession = el => {
  const k = el.dataset.key, w = S.sessions[k];
  if (w && !w.closed) w.close();
  delete S.sessions[k]; S.pop = null;
  toast("ok", `已断开 ${devByKey(k).name}`); render();
};
A.closeRemote = () => {
  if (S.win === "remote") { window.close(); toast("info", "已断开", "窗口没有自动关闭时可以直接关掉"); return; }
  const d = devByKey(S.remote);
  if (ctlOf(d) === "human") setCtl(d, "none");
  S.remote = null; S.pop = null; S.elevated = false; render();
};
// 控制权操作作用于按钮所属的设备：data-id，或当前远程桌面、当前设备页
const ctxDev = el => el?.dataset?.id ? devById(el.dataset.id) : S.remote ? devByKey(S.remote) : curDev();
A.takeover = el => {
  const d = ctxDev(el);
  if (ctlOf(d) === "ai") { S.modal = { type: "takeover", id: d.id }; render(); return; }
  setCtl(d, "human"); toast("info", `你已获得 ${d.name} 的桌面控制权`); render();
};
A.release = el => { setCtl(ctxDev(el), "none"); toast("ok", "已释放控制权", "Claude Code 可以继续排队获取"); render(); };
A.stopAI = el => { S.modal = { type: "stopAI", opt: "stop", id: ctxDev(el).id }; render(); };
A.devMenu = el => {
  const d = curDev();
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
MODALS.takeover = () => modal({
  title: "从 AI 手中接管桌面？", sub: `Claude Code 正在操作 ${S.modal.id} 的鼠标键盘`,
  body: `<p class="muted" style="line-height:1.7">接管后，AI 当前的鼠标键盘操作会立即中断，它会排队等你释放控制权。它在后台运行的命令不受影响。</p>`,
  foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn primary" data-a="doTakeover">${ic("hand")}接管</button>`,
});
A.doTakeover = () => { const d = devById(S.modal.id); S.modal = null; setCtl(d, "human"); toast("info", `你已接管 ${d.name}`, "Claude Code 在排队"); render(); };

MODALS.stopAI = () => {
  const o = S.modal.opt;
  const opt = (v, t, d) => `<div class="opt ${o === v ? "on" : ""}" data-a="modalOpt" data-k="opt" data-v="${v}"><span class="radio ${o === v ? "on" : ""}"></span><div><div class="t">${t}</div><div class="d">${d}</div></div></div>`;
  return modal({
    title: "停止 Claude Code", sub: `它正在操作 ${S.modal.id}`,
    body: opt("stop", "紧急停止", "取消它在这台设备上的全部请求，并拒绝后续请求，直到你恢复。") +
      opt("kill", "紧急停止并结束它启动的进程", "另外结束它启动的 2 个进程。做了一半的操作可能留下不完整的文件。") +
      opt("take", "只接管鼠标键盘", "由你远程控制；它的后台命令继续运行。"),
    foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn ${o === "take" ? "primary" : "danger"}" data-a="doStopAI">${o === "take" ? "接管" : o === "kill" ? "停止并结束进程" : "紧急停止"}</button>`,
  });
};
A.doStopAI = () => {
  const o = S.modal.opt, d = devById(S.modal.id); S.modal = null;
  if (o === "take") { setCtl(d, "human"); toast("info", `你已接管 ${d.name}`); }
  else {
    setCtl(d, "none");
    S.stopped[d.id] = { who: "Claude Code", t: new Date().toTimeString().slice(0, 5), kill: o === "kill" };
    toast("ok", o === "kill" ? "已停止 Claude Code 并结束 2 个进程" : "已紧急停止 Claude Code", "可以在设备页或“设置 → 桌面控制权”中恢复");
  }
  render();
};

A.resumeAI = el => { const id = el.dataset.id; const st = S.stopped[id]; delete S.stopped[id]; toast("ok", `已恢复 ${st ? st.who : "AI"}`, `它可以重新在 ${id} 上发起请求`); render(); };

A.addDevice = () => { S.modal = { type: "addDevice" }; render(); };
MODALS.addDevice = () => modal({
  title: "添加设备", sub: "先在要添加的设备上安装 DeskBridge，再用下面任一种连接方式连上",
  body: `<div class="col" style="gap:16px">
      <div class="feature"><div class="fi">1</div><div><div class="t">安装 DeskBridge</div><div class="d">电脑（Windows 10/11）：下载安装包 · 服务器（Linux）：运行一行安装命令 · 安卓手机：目前只能作为控制端<br>
        <a class="link" data-a="noop" data-msg="已复制下载链接">复制下载链接</a> · <a class="link" data-a="noop" data-msg="已复制安装命令">复制 Linux 安装命令</a></div></div></div>
      <div class="feature"><div class="fi">${ic("wifi")}</div><div><div class="t">在同一局域网中配对</div><div class="d">新设备会出现在“局域网”页。点它的“配对”，输入对方显示的 6 位配对码；也可以点“本机配对码”，让对方输入本机的配对码</div></div></div>
      <div class="feature"><div class="fi">${ic("globe")}</div><div><div class="t">登录同一个 Tailscale 账号</div><div class="d">${tsOn()
        ? `适合不在同一局域网的设备。在新设备上用 ${CONNECTORS[0].acct} 登录，它会自动出现在“Tailscale”页，不需要配对码`
        : `适合不在同一局域网的设备。本机还没有添加 Tailscale，<a class="link" data-a="addConn">先添加 Tailscale</a>`}</div></div></div></div>
    <div class="row small muted" style="gap:10px"><span class="spin"></span>正在等待新设备…<span class="spacer"></span><a class="link" data-a="simNewDevice">模拟局域网中出现新设备</a></div>`,
  foot: `<button class="btn" data-a="closeModal">完成</button>`,
});
A.simNewDevice = () => {
  if (!devById("new-pc", "lan")) DEVICES.push({ id: "new-pc", name: "new-pc", kind: "monitor", os: "Windows 11 家庭版", via: "lan", paired: false, online: true, path: "lan", latency: 3, ip: "192.168.1.52", version: "0.3.0" });
  S.modal = null; S.conn = "lan"; S.set.lanOn = true;
  toast("ok", "局域网中发现新设备 new-pc", "点它的“配对”后就能使用"); go("#/devices");
};
