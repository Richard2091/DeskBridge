// 其他页面（初版范围）：操作记录、设置。初版不做访问权限、发起端确认、首次登录向导。

function phoneTop(title, extra = "") {
  return isPhone() ? `<div class="mtop"><h1>${title}</h1>${extra}</div>` : "";
}

function pageHead(title, sub, actions = "") {
  return isPhone() ? phoneTop(title, actions) : `<div class="v2-head"><div><h1>${title}</h1><div class="sub">${sub}</div></div><div class="actions">${actions}</div></div>`;
}

// ---------- 操作记录 ----------
function viewActivity() {
  const who = S.act.who;
  let items = ACTIVITY;
  if (who !== "all") items = items.filter(a => ACTORS[a.actor].type === who || (who === "human" && ACTORS[a.actor].type === "local"));
  if (S.act.dev !== "all") items = items.filter(a => a.dev === S.act.dev);
  const seg = (v, l) => `<button class="${who === v ? "on" : ""}" data-a="actWho" data-v="${v}">${l}</button>`;
  let body;
  if (S.scenario === "loading") body = `<div class="list2">${Array.from({ length: 5 }, () => `<div class="li"><span class="avatar sk"></span><div class="b"><div class="sk" style="height:12px;width:50%"></div><div class="sk" style="height:10px;width:70%;margin-top:8px"></div></div></div>`).join("")}</div>`;
  else if (!items.length && (who !== "all" || S.act.dev !== "all")) body = `<div class="empty"><div class="art">${ic("filter", "xl")}</div><h3>没有符合条件的记录</h3><p>换一个筛选条件试试。</p><div class="btns"><button class="btn" data-a="actClear">清除筛选</button></div></div>`;
  else if (!items.length) body = `<div class="empty"><div class="art">${ic("history", "xl")}</div><h3>还没有操作记录</h3><p>你和 AI 在各设备上的每一次操作都会记录在这里，方便回看和检查 AI 做了什么。</p></div>`;
  else {
    const days = [...new Set(items.map(a => a.day))];
    body = days.map(dy => `<div class="day">${dy}</div><div class="list2">${items.filter(a => a.day === dy).map(actItem).join("")}</div>`).join("");
  }
  const filtered = S.act.dev !== "all";
  return `<div class="v2-page" data-sk="act"><div class="v2-in">
    ${pageHead("操作记录", "谁（AI 或人）在哪台设备上做了什么", isPhone() ? "" : `<button class="btn" data-a="noop" data-msg="已导出为 CSV">${ic("download")}导出</button>`)}
    <div class="row" style="flex-wrap:wrap;gap:10px">
      <div class="segmented">${seg("all", "全部")}${seg("ai", "AI")}${seg("human", "人")}</div>
      <button class="btn ${filtered ? "primary" : ""}" data-a="popActFilter">${ic("filter")}${filtered ? S.act.dev : "筛选"}</button>
      ${filtered ? `<button class="btn ghost sm" data-a="actClear">清除</button>` : ""}
      <span class="spacer"></span>${isPhone() ? "" : `<span class="pill ok"><span class="pulse live"></span>实时</span>`}
    </div>
    ${body}
  </div></div>`;
}

A.actWho = el => { S.act.who = el.dataset.v; render(); };
A.actClear = () => { S.act.dev = "all"; S.act.who = "all"; render(); };
// 从操作记录打开设备：按当前可用的连接方式选一条；都不可用时说明原因
A.openDevice = el => {
  const e = pickEntry(el.dataset.id);
  S.drawer = null;
  if (!e) { toast("info", `当前的连接方式里没有 ${el.dataset.id}`, "它可能在 Tailscale 里，添加 Tailscale 后可以打开"); render(); return; }
  go(devUrl(e));
};
A.actDrawer = el => { S.drawer = { type: "act", i: +el.dataset.i }; render(); };
A.popActFilter = el => openPop(el, "actFilter");
A.actDev = el => { S.act.dev = el.dataset.v; S.pop = null; render(); };
POPS.actFilter = () => `<div class="ph2">设备</div>${[{ id: "all", name: "全部设备" }].concat([...new Map(DEVICES.filter(d => !d.self && !d.noNode).map(d => [d.id, d])).values()]).map(d => `<div class="pi" data-a="actDev" data-v="${d.id}"><span class="radio ${S.act.dev === d.id ? "on" : ""}"></span>${d.name}</div>`).join("")}
  <div class="divider"></div><div class="ph2">结果</div><div class="chips" style="padding:0 6px 6px">${["全部", "失败", "被拒绝", "被抢占"].map((x, i) => `<button class="${i === 0 ? "on" : ""}">${x}</button>`).join("")}</div>`;

DRAWERS.act = () => {
  const a = ACTIVITY[S.drawer.i];
  const [stl, stc] = ST_LABEL[a.st];
  const chain = a.chain.map((c, k) => {
    if (ACTORS[c]) return `<div class="lk">${avatar(c, "sm")}<b>${ACTORS[c].name}</b><span class="muted">${ACTORS[c].from}</span></div>`;
    const [dv, tool] = c.split(":");
    return `<div class="lk"><span class="avatar sm" style="background:var(--surface3);color:var(--muted)">${ic(k === a.chain.length - 1 ? "zap" : "arrowRight", "sm")}</span><span class="tag">${dv}</span><span class="mono">${tool}</span></div>`;
  }).join("");
  return drawer({
    title: `${ACTORS[a.actor].name} ${actVerb(a)}`, sub: `${a.day} ${a.t} · ${a.dev} · <span class="mono">${a.tool}</span>`,
    body: `<div class="row"><span class="pill ${stc}">${stl}</span><span class="muted small">耗时 ${a.du}</span></div>
      <div><div class="small muted" style="margin-bottom:8px">内容</div><div class="codebox">${esc(a.sum)}</div></div>
      <div><div class="small muted" style="margin-bottom:8px">调用链</div><div class="chain">${chain}</div></div>
      <div><div class="small muted" style="margin-bottom:8px">结果</div>${a.tool === "screen.capture" ? `<div class="shot-box">${desktopArt({ ai: true })}</div><div class="small muted" style="margin-top:6px">${esc(a.out)}</div>` : `<div class="codebox">${esc(a.out)}</div>`}</div>
      ${expander("actArgs", "terminal", "参数", "敏感字段已脱敏", `<div class="codebox">${esc(a.args)}</div><div class="small muted">请求编号 <span class="mono">req_7f3a${S.drawer.i}c91e</span></div>`)}`,
    foot: `<button class="btn" data-a="openDevice" data-id="${a.dev}">${ic("monitor")}打开设备</button><button class="btn ghost" data-a="noop" data-msg="已复制请求编号">${ic("copy")}复制编号</button>`,
  });
};

// ---------- 设置 ----------
const SET_SECS = [
  ["general", "sliders", "通用"],
  ["appearance", "palette", "外观"],
  ["thisdev", "laptop", "本机"],
  ["control", "hand", "桌面控制权"],
  ["network", "globe", "连接方式"],
  ["ai", "plug", "AI 接入"],
  ["about", "info", "关于"],
];

function sw(k) { return `<button class="switch ${S.set[k] ? "on" : ""}" data-a="setToggle" data-k="${k}"></button>`; }
function sitem(t, d, ctl) { return `<div class="sitem"><div class="b"><div class="t">${t}</div>${d ? `<div class="d">${d}</div>` : ""}</div><div class="ctl">${ctl}</div></div>`; }
function sgroup(title, items) { return `<div class="sgroup">${title ? `<h3>${title}</h3>` : ""}<div class="card">${items.join("")}</div></div>`; }
function sel(k, opts) { return `<select class="select" data-in="set.${k}">${opts.map(o => `<option ${S.set[k] === o ? "selected" : ""}>${o}</option>`).join("")}</select>`; }
A.setToggle = el => { S.set[el.dataset.k] = !S.set[el.dataset.k]; render(); };
A.setTheme = el => { S.theme = el.dataset.v; render(); };
A.setMode = el => { S.mode = el.dataset.v; render(); };

function themeCard(v, name, desc, c) {
  return `<div class="theme-card ${S.theme === v ? "on" : ""}" data-a="setTheme" data-v="${v}">
    <div class="pv" style="background:${c.page}"><div class="a" style="background:${c.side}"><i style="background:${c.acc};width:70%"></i><i style="background:${c.line}"></i><i style="background:${c.line};width:80%"></i></div>
      <div class="b" style="background:${c.surf};border:1px solid ${c.line};border-radius:${c.r}"><i style="background:${c.line};width:50%"></i><i style="background:${c.line}"></i><i style="background:${c.acc};width:30%;height:10px;border-radius:${c.r}"></i></div></div>
    <div class="cap3"><span class="radio ${S.theme === v ? "on" : ""}"></span>${name}<span class="d">${desc}</span></div></div>`;
}

function settingsSec(sec) {
  const s = S.set;
  switch (sec) {
    case "general": return `<h2>通用</h2><div class="sub">启动方式、语言和通知</div>` +
      sgroup("启动", [sitem("开机时自动启动", "登录 Windows 后在后台运行，其他设备和 AI 才能连接到本机", sw("autostart")), sitem("关闭窗口时最小化到托盘", "", sw("tray"))]) +
      sgroup("远程桌面", [sitem("打开方式", "独立窗口可以同时操作多台设备，每台一个窗口", sel("remoteMode", ["独立窗口", "在主窗口中打开"]))]) +
      sgroup("语言和通知", [sitem("界面语言", "", sel("lang", ["简体中文", "English"])), sitem("AI 操作桌面时通知我", "AI 开始在任何设备上使用鼠标键盘时通知", `<button class="switch on"></button>`)]);
    case "appearance": return `<h2>外观</h2><div class="sub">主题、明暗和显示密度</div>` +
      `<div class="sgroup"><h3>主题</h3><div class="card"><div class="themes">
        ${themeCard("clean", "清爽", "默认", { page: "#f4f4f6", side: "#f4f4f6", surf: "#fff", line: "#e6e6ea", acc: "#5b5bd6", r: "4px" })}
        ${themeCard("mono", "黑白", "克制", { page: "#fafafa", side: "#fafafa", surf: "#fff", line: "#eaeaea", acc: "#0a0a0a", r: "3px" })}
        ${themeCard("soft", "柔光", "活泼", { page: "linear-gradient(160deg,#e8eefc,#fbeef2)", side: "transparent", surf: "rgba(255,255,255,.85)", line: "rgba(20,20,40,.08)", acc: "linear-gradient(135deg,#3b6cf6,#7c5cf6)", r: "6px" })}
      </div></div></div>` +
      sgroup("", [
        sitem("明暗", "", `<div class="segmented">${[["light", "sun", "浅色"], ["dark", "moon", "深色"], ["system", "monitor", "跟随系统"]].map(([v, i, l]) => `<button class="${S.mode === v ? "on" : ""}" data-a="setMode" data-v="${v}">${ic(i, "sm")}${l}</button>`).join("")}</div>`),
        sitem("显示密度", "紧凑模式下一屏能显示更多设备和输出", sel("density", ["紧凑", "标准", "宽松"])),
        sitem("界面字号", "", sel("fontsize", ["13", "14", "15", "16"])),
        sitem("终端字体", "命令输出和代码使用的等宽字体", `<select class="select"><option>Cascadia Mono</option><option>Consolas</option><option>JetBrains Mono</option></select>`),
      ]);
    case "thisdev": return `<h2>本机</h2><div class="sub">my-laptop 的名称和地址</div>` +
      sgroup("设备", [
        sitem("设备名称", "其他设备和 AI 用这个名字找到本机", `<input class="input" value="my-laptop">`),
        sitem("局域网地址", "", `<span class="mono">${CONNECTORS[1].ip}</span>`),
        ...(tsOn() ? [sitem("Tailscale 地址", "", `<span class="mono">${CONNECTORS[0].ip}</span>`)] : []),
      ]);
    case "control": return `<h2>桌面控制权</h2><div class="sub">本地用户、远程人工和 AI 同时使用桌面时的规则。优先级：本地用户 &gt; 远程人工 &gt; AI</div>` +
      sgroup("自动释放", [
        sitem("本地用户停止操作后多久让 AI 继续", "本地一有鼠标键盘输入，AI 会立即停止；这个时间只决定何时继续", `${sel("release", ["1", "3", "5", "10"])}<span class="small muted">秒</span>`),
        sitem("远程一方的租约时长", "连接断开后，最多这么久自动释放控制权", `${sel("lease", ["10", "15", "30"])}<span class="small muted">秒</span>`),
        sitem("持有但不操作时自动释放", "", `${sel("idle", ["30", "60", "120"])}<span class="small muted">秒</span>`),
      ]) +
      sgroup("策略", [
        sitem("只在我离开时允许 AI 操作桌面", "本地无输入超过 5 分钟或屏幕已锁定时，才允许 AI 使用鼠标键盘", sw("onlyAway")),
      ]) +
      sgroup("紧急操作（在被控端本机使用）", [
        sitem("暂停所有远程操作", "中断远程的鼠标键盘，拒绝新的远程操作，后台命令继续", `<span class="kbd" style="margin:0">${s.hkPause}</span><button class="btn sm ghost" data-a="noop" data-msg="原型未实现">修改</button>`),
        sitem("终止", "暂停，并结束远程一方启动的所有进程", `<span class="kbd" style="margin:0">${s.hkStop}</span><button class="btn sm ghost" data-a="noop" data-msg="原型未实现">修改</button>`),
        ...(Object.keys(S.stopped).length
          ? Object.entries(S.stopped).map(([id, st]) => sitem(`已紧急停止：${st.who}`, `${id} · ${st.t} 停止，它在这台设备上的请求会被拒绝`, `<button class="btn sm" data-a="resumeAI" data-id="${id}">恢复</button>`))
          : [sitem("已紧急停止的 AI", "被你在设备页停止的 AI 会话", `<span class="muted small">无</span>`)]),
      ]);
    case "network": {
      const ts = CONNECTORS[0];
      const paired = DEVICES.filter(d => d.via === "lan" && d.paired);
      const tsPill = !tsAdded() ? `<span class="pill off">未添加</span>` : !tsOn() ? `<span class="pill off">未登录</span>` : netDown() ? `<span class="pill bad">已断开</span>` : `<span class="pill ok">已连接</span>`;
      const loginBtn = S.tsLogging ? `<span class="spin"></span><span class="small muted">等待浏览器登录…</span>` : `<button class="btn sm primary" data-a="tsLogin">${ic("external")}${tsAdded() ? "登录 Tailscale" : "添加并登录"}</button>`;
      const tsItems = !tsAdded()
        ? [sitem("添加 Tailscale", "要连接不在同一局域网的设备时添加。添加后用 Tailscale 账号登录，同一账号的设备会出现在设备页的“Tailscale”中", loginBtn)]
        : !tsOn()
        ? [sitem("账号", S.tsErr ? `<span style="color:var(--bad)">上次登录失败：无法连接登录服务器（control_unreachable）</span>` : "登录后，同一账号下的设备会自动出现在设备页的“Tailscale”中", loginBtn)]
        : [
          sitem(`<span class="row">${netDown() ? `<span class="pulse bad"></span>正在重连（第 ${S.netTry} 次）` : `<span class="pulse"></span>本机 <span class="mono">${ts.ip}</span>`}</span>`, netDown() ? "最近错误：无法连接控制服务器（超时）" : "最近一次同步 10 秒前", `<button class="btn sm" data-a="${netDown() ? "netRetry" : "noop"}" data-msg="诊断完成：连接正常">${ic("refresh")}${netDown() ? "立即重试" : "诊断"}</button>`),
          sitem("账号", ts.acct, `<button class="btn sm ghost danger-soft" data-a="askTsLogout">${ic("logout")}退出登录</button>`),
        ];
      if (tsAdded()) tsItems.push(
        sitem("控制服务器", "默认使用 Tailscale 官方服务器，也可以改成自建的 Headscale", `<select class="select" data-in="set.server" data-rerender="1">${["默认（Tailscale 官方）", "自建 Headscale"].map(o => `<option ${s.server === o ? "selected" : ""}>${o}</option>`).join("")}</select>`),
        ...(s.server === "自建 Headscale" ? [sitem("服务器地址", "", `<input class="input mono" placeholder="https://headscale.example.com" value="${esc(s.serverUrl)}" data-in="set.serverUrl" style="width:260px">`)] : []),
        ...(tsOn() ? [sitem("授权密钥", "给没有浏览器的服务器登录用", `<button class="btn sm" data-a="noop" data-msg="已生成授权密钥（示例）">生成密钥</button>`)] : []),
        sitem("移除 Tailscale", "设备页不再显示 Tailscale 标签和其中的设备；局域网不受影响", `<button class="btn sm ghost danger-soft" data-a="askTsRemove">${ic("trash")}移除</button>`),
      );
      return `<h2>连接方式</h2><div class="sub">每种连接方式各自列出设备，互不影响；从哪个页面连接就走哪种方式。局域网默认开启，Tailscale 需要添加并登录</div>` +
      sgroup(`<span class="row">${ic("globe", "sm")}Tailscale${tsPill}</span>`, tsItems) +
      sgroup(`<span class="row">${ic("wifi", "sm")}局域网${s.lanOn ? `<span class="pill ok">已开启</span>` : `<span class="pill off">已关闭</span>`}</span>`, [
        sitem("在局域网中发现设备并允许配对", `本机 <span class="mono">${CONNECTORS[1].ip}</span> · 关闭后不会被局域网中的其他设备发现，已配对的设备也无法经局域网连接`, sw("lanOn")),
        ...(s.lanOn ? [sitem("本机配对码", "让对方在它的 DeskBridge 里输入，配对后双方自动连接", `<button class="btn sm" data-a="showPairCode">${ic("link")}显示配对码</button>`)] : []),
        ...(paired.length ? paired.map(d => sitem(`<span class="row">${ic(kindIcon(d), "sm")}${d.name}</span>`, `已配对 · <span class="mono">${d.ip}</span>`, `<button class="btn sm ghost danger-soft" data-a="unpair" data-id="${d.id}">取消配对</button>`))
          : [sitem("已配对的设备", "还没有配对的设备。在设备页的“局域网”中点“配对”", "")]),
      ]) +
      sgroup(`<span class="row">${ic("link", "sm")}WireGuard 直连<span class="soon">后续支持</span></span>`, [
        sitem("两台设备配对后直接连接", "不依赖账号，适合不想使用 Tailscale 的情况", `<button class="btn sm" disabled style="opacity:.5">配对设备</button>`),
      ]) +
      sgroup(`<span class="row">${ic("route", "sm")}自建中继<span class="soon">后续支持</span></span>`, [
        sitem("自己部署中继服务器", "无法直连时转发流量", `<button class="btn sm" disabled style="opacity:.5">添加中继</button>`),
      ]);
    }
    case "ai": return `<h2>AI 接入</h2><div class="sub">给本机的 AI 客户端（Claude Code、Codex 等）通过 MCP 控制其他设备</div>` +
      `<div class="sgroup"><h3>MCP 配置</h3><div class="card"><div class="card-b col" style="gap:10px">
        <div class="muted small">把下面的配置加到 Agent 的 MCP 设置里（例如 Claude Code 的 <span class="mono">.mcp.json</span>）：</div>
        <div class="snippet"><div class="codebox">{
  "mcpServers": {
    "deskbridge": { "command": "deskbridge", "args": ["mcp"] }
  }
}</div><button class="btn sm" data-a="noop" data-msg="已复制 MCP 配置">${ic("copy")}复制</button></div>
        <div class="muted small">或者在命令行运行：<span class="mono">claude mcp add deskbridge -- deskbridge mcp</span></div></div></div></div>` +
      sgroup("已连接的 AI 会话", (() => {
        // 只列出来源设备当前能连上的会话
        const live = [["claude", "已连接 42 分钟 · 调用 128 次"], ["codex", "已连接 3 小时 · 调用 16 次"]].filter(([k]) => { const e = pickEntry(ACTORS[k].from); return e && !unreach(e) && !unpaired(e); });
        return live.length
          ? live.map(([k, t]) => { const e = pickEntry(ACTORS[k].from); return sitem(`<span class="row">${avatar(k, "sm")}${ACTORS[k].name}</span>`, `${e.name} · ${VIA_LABEL[e.via]} · ${t}`, `<button class="btn sm ghost danger-soft" data-a="noop" data-msg="已断开">断开</button>`); })
          : [sitem("还没有 AI 连接", "其他设备上的 AI 客户端通过 MCP 连接后，会显示在这里", "")];
      })());
    case "about": return `<h2>关于</h2><div class="sub">DeskBridge 0.3.0</div>` +
      sgroup("", [sitem("版本", "0.3.0（2026-10-07）", `<button class="btn sm" data-a="noop" data-msg="已是最新版本">检查更新</button>`), sitem("开源许可证", "以及使用的第三方组件", `<button class="btn sm ghost" data-a="noop" data-msg="原型未实现">查看</button>`), sitem("日志", "遇到问题时可以附上日志", `<button class="btn sm ghost" data-a="noop" data-msg="原型未实现">打开日志目录</button>`)]);
  }
  return "";
}

function viewSettings() {
  const sec = S.route.sec;
  if (isPhone()) {
    if (!sec) {
      return `${phoneTop("设置")}<div class="mlist">${SET_SECS.map(([k, i, l]) => `<a data-go="#/settings/${k}"><span class="ic">${ic(i)}</span>${l}<span class="r">${ic("chevRight")}</span></a>`).join("")}
</div>`;
    }
    return `<div class="mtop"><button class="btn ghost icon" data-go="#/settings">${ic("chevLeft", "lg")}</button><h2>${SET_SECS.find(s => s[0] === sec)[2]}</h2></div><div class="sbody">${settingsSec(sec).replace(/<h2>.*?<\/h2>/, "")}</div>`;
  }
  return `<div class="settings">
    <nav class="snav"><div class="small muted" style="padding:6px 10px 10px;font-weight:600">设置</div>${SET_SECS.map(([k, i, l]) => `<a class="${sec === k ? "on" : ""}" data-go="#/settings/${k}">${ic(i)}<span class="label">${l}</span></a>`).join("")}</nav>
    <div class="sbody" data-sk="set">${settingsSec(sec)}</div>
  </div>`;
}
