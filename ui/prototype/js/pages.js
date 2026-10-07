// 其他页面：操作记录、审批、外部设备、设置、首次登录。

function phoneTop(title, extra = "") {
  return isPhone() ? `<div class="mtop"><h1>${title}</h1>${extra}</div>` : "";
}

// ---------- 操作记录 ----------
function activityList(items, compact = false) {
  if (S.scenario === "loading") return Array.from({ length: 6 }, () => `<div class="act"><div class="act-r"><div class="sk" style="height:10px"></div><div class="sk" style="height:10px"></div><div class="sk" style="height:10px"></div><div class="sk" style="height:10px"></div></div></div>`).join("");
  if (S.scenario === "empty" || !items.length) {
    return `<div class="empty"><div class="art">${ic("history", "xl")}</div><h3>还没有操作记录</h3><p>你和 AI 在各设备上的每一次操作都会记录在这里，方便回看和检查 AI 做了什么。</p></div>`;
  }
  let day = "";
  return items.map(a => {
    const i = ACTIVITY.indexOf(a);
    const actor = ACTORS[a.actor];
    const [stl, stc] = ST_LABEL[a.st];
    const head = a.day !== day ? `<div class="act-day">${(day = a.day)}</div>` : "";
    const open = S.act.open === String(i);
    return head + `<div class="act">
      <div class="act-r" data-a="actOpen" data-i="${i}">
        <span class="tm">${a.t}</span>
        <span class="who">${avatar(a.actor, "sm")}<span>${actor.name}</span></span>
        <span class="dv tag">${ic(kindIcon(devById(a.dev) || { kind: "monitor" }), "sm")}${a.dev}</span>
        <span class="sm"><span class="mono faint" style="margin-right:8px">${a.tool}</span>${esc(a.sum)}</span>
        <span class="stc"><span class="pill ${stc}">${a.st === "run" ? `<span class="spin" style="width:9px;height:9px;border-width:1.5px"></span>` : ""}${stl}</span></span>
        <span class="du">${a.du}</span>
      </div>
      ${open ? `<div class="act-x">
        <div class="col" style="gap:10px">
          <div><div class="muted small" style="margin-bottom:6px">调用链</div><div class="chain">${a.chain.map((c, k) => {
            if (ACTORS[c]) return `<div class="lk">${avatar(c, "sm")}<b>${ACTORS[c].name}</b><span class="muted">${ACTORS[c].type === "ai" ? "AI" : ACTORS[c].type === "local" ? "被控端本地" : ACTORS[c].type === "ext" ? "外部" : "人"} · ${ACTORS[c].from}</span></div>`;
            const [dv, tool] = c.split(":");
            return `<div class="lk"><span class="avatar sm" style="background:var(--surface3);color:var(--muted)">${ic(k === a.chain.length - 1 ? "zap" : "arrowRight", "sm")}</span><span class="tag">${dv}</span><span class="mono">${tool}</span></div>`;
          }).join("")}</div></div>
          <div class="kv small" style="grid-template-columns:80px 1fr"><dt>请求编号</dt><dd class="mono">req_7f3a${i}c91e</dd><dt>开始时间</dt><dd>${a.day} ${a.t}:31</dd><dt>耗时</dt><dd>${a.du}</dd></div>
        </div>
        <div class="col" style="gap:10px">
          <div><div class="muted small" style="margin-bottom:6px">参数（敏感字段已脱敏）</div><div class="codebox">${esc(a.args)}</div></div>
          <div><div class="muted small" style="margin-bottom:6px">结果</div><div class="codebox">${esc(a.out)}</div></div>
          <div class="row"><button class="btn sm" data-go="#/devices/${a.dev}/overview">${ic("monitor")}查看设备</button><button class="btn sm ghost" data-a="noop" data-msg="已复制请求编号">${ic("copy")}复制编号</button></div>
        </div>
      </div>` : ""}
    </div>`;
  }).join("");
}

A.actOpen = el => { S.act.open = S.act.open === el.dataset.i ? null : el.dataset.i; render(); };
A.actWho = el => { S.act.who = el.dataset.v; render(); };

function viewActivity() {
  const who = S.act.who;
  let items = ACTIVITY;
  if (who !== "all") items = items.filter(a => ACTORS[a.actor].type === who || (who === "human" && ACTORS[a.actor].type === "local"));
  if (S.act.dev !== "all") items = items.filter(a => a.dev === S.act.dev);
  const seg = (v, l) => `<button class="${who === v ? "on" : ""}" data-a="actWho" data-v="${v}">${l}</button>`;
  return `${phoneTop("操作记录")}
    ${isPhone() ? "" : `<div class="ph"><div><h1>操作记录</h1><div class="sub">谁（AI 或人）在哪台设备上做了什么</div></div>
      <div class="actions"><span class="pill ok"><span class="pulse live"></span>实时</span><button class="btn sm" data-a="noop" data-msg="已导出为 CSV">${ic("download")}导出</button></div></div>`}
    <div class="toolbar">
      <div class="segmented">${seg("all", "全部")}${seg("ai", "AI")}${seg("human", "人")}${seg("ext", "外部")}</div>
      <select class="select" style="width:150px;height:30px" data-in="act.dev" data-rerender="1">
        <option value="all">全部设备</option>${DEVICES.filter(d => !d.self && !d.noNode).map(d => `<option value="${d.id}" ${S.act.dev === d.id ? "selected" : ""}>${d.name}</option>`).join("")}</select>
      ${isPhone() ? "" : `<select class="select" style="width:120px;height:30px"><option>全部结果</option><option>失败</option><option>被拒绝</option><option>被抢占</option></select>
      <div class="search" style="width:220px">${ic("search", "sm")}<input placeholder="搜索命令、文件、工具…"></div>`}
    </div>
    <div class="scroll pad" data-sk="act">${activityList(items)}</div>`;
}

// ---------- 审批 ----------
function apprCard(a, done = false) {
  const actor = ACTORS[a.actor];
  const risk = { high: ["高风险", "bad"], mid: ["中风险", "warn"] }[a.risk];
  return `<div class="card appr ${done ? "done" : ""}">
    <div class="top">${avatar(a.actor)}<div><b>${actor.name}</b> <span class="muted">请求在</span> <b>${a.dev}</b> <span class="muted">上执行</span><div class="small muted">${actor.type === "ext" ? "外部 · " : "AI · "}来自 ${actor.from}</div></div>
      <span class="pill ${risk[1]}">${risk[0]}</span>
      ${done ? `<span class="tmr">${a.result}</span>` : `<span class="tmr">${ic("clock", "sm")}<span data-tmr="${a.id}">${Math.floor(a.wait / 60)}:${String(a.wait % 60).padStart(2, "0")} 后自动拒绝</span></span>`}</div>
    <div class="codebox" style="font-size:13px">${esc(a.what)}</div>
    <div class="grid"><div><div class="l">工具</div><span class="mono">${a.tool}</span></div><div><div class="l">触发原因</div>${a.why}</div><div><div class="l">说明</div>${a.reason}</div></div>
    ${done ? "" : `<div class="btns"><button class="btn" data-a="apprDo" data-id="${a.id}" data-r="deny">${ic("x")}拒绝</button>
      <span class="right"><button class="btn" data-a="apprDo" data-id="${a.id}" data-r="session">本次会话内都允许</button><button class="btn primary" data-a="apprDo" data-id="${a.id}" data-r="once">${ic("check")}允许一次</button></span></div>`}
  </div>`;
}

A.apprTab = el => { S.appr.tab = el.dataset.v; render(); };
A.apprDo = el => {
  const a = S.appr.items.find(x => x.id === el.dataset.id);
  const r = el.dataset.r;
  S.appr.items = S.appr.items.filter(x => x !== a);
  a.result = { deny: "已拒绝", once: "已允许一次", session: "本次会话内允许" }[r];
  S.appr.done.unshift(a);
  toast(r === "deny" ? "warn" : "ok", a.result, `${ACTORS[a.actor].name} · ${a.what.slice(0, 40)}`);
  render();
};
A.ruleToggle = el => { const r = S.rules[+el.dataset.i]; r.on = !r.on; render(); };

function viewApprovals() {
  const t = S.appr.tab;
  const seg = (v, l) => `<button class="${t === v ? "on" : ""}" data-a="apprTab" data-v="${v}">${l}</button>`;
  let body;
  if (S.scenario === "loading") body = `<div class="card appr"><div class="sk" style="height:14px;width:50%"></div><div class="sk" style="height:40px"></div></div>`;
  else if (t === "pending") {
    body = S.appr.items.length ? S.appr.items.map(a => apprCard(a)).join("")
      : `<div class="empty"><div class="art accent">${ic("shield", "xl")}</div><h3>没有待审批的操作</h3><p>AI 或外部用户触发审批规则时，会在这里等你决定，同时弹出系统通知。</p></div>`;
  } else if (t === "done") {
    body = S.appr.done.length ? S.appr.done.map(a => apprCard(a, true)).join("") : `<div class="empty"><div class="art">${ic("history", "xl")}</div><h3>还没有处理过的审批</h3></div>`;
  } else {
    body = `<div class="card">${S.rules.map((r, i) => `<div class="rule"><div class="b"><div class="t">${r.t}</div><div class="d">${r.d}</div></div><span class="tag">${r.scope}</span><button class="switch ${r.on ? "on" : ""}" data-a="ruleToggle" data-i="${i}"></button></div>`).join("")}</div>
      <div class="muted small" style="margin-top:10px">没有人处理的审批会在 15 分钟后自动拒绝（可在设置中修改）。</div>`;
  }
  return `${phoneTop("审批")}
    ${isPhone() ? "" : `<div class="ph"><div><h1>审批</h1><div class="sub">危险操作和外部访问会在这里等你确认</div></div>
      <div class="actions">${t === "rules" ? `<button class="btn sm primary" data-a="noop" data-msg="新建规则（原型未实现）">${ic("plus")}新建规则</button>` : ""}</div></div>`}
    <div class="toolbar"><div class="segmented">${seg("pending", `待审批${S.appr.items.length ? ` <span class="badge">${S.appr.items.length}</span>` : ""}`)}${seg("done", "已处理")}${seg("rules", "规则")}</div></div>
    <div class="scroll pad" data-sk="appr">${body}</div>`;
}

// ---------- 外部设备 ----------
function viewShared() {
  const card = (s, out) => `<div class="card share">
    <div class="top"><div class="dic">${ic("monitor")}<span class="pulse"></span></div><div><b>${s.dev}</b><div class="small muted">${out ? `共享给 ${s.to}` : `来自 ${s.owner}`} · ${s.since} 起</div></div>
      <span class="spacer"></span>${out ? `<button class="btn sm ghost" data-a="noop" data-msg="已撤销对 ${s.to} 的共享">撤销</button>` : `<button class="btn sm" data-go="#/devices/${s.dev}/overview">打开</button>`}</div>
    <div class="perms">${s.perms.map(p => `<span class="tag">${p}</span>`).join("")}</div>
    <div class="small muted">有效期：${s.exp}</div>
  </div>`;
  return `${phoneTop("外部设备", `<button class="btn primary icon" data-a="invite">${ic("plus")}</button>`)}
    ${isPhone() ? "" : `<div class="ph"><div><h1>外部设备</h1><div class="sub">和别人共享设备，对方只能使用你明确授予的权限</div></div>
      <div class="actions"><button class="btn sm" data-a="acceptInvite">${ic("link")}输入邀请链接</button><button class="btn sm primary" data-a="invite">${ic("share")}共享我的设备</button></div></div>`}
    <div class="scroll pad" data-sk="shared">
      <div class="banner info" style="margin-bottom:16px">${ic("info")}<div class="t">外部设备是加分功能（任务 17），这里先确定位置和流程。外部用户的每次操作都会在双方的操作记录中保留。</div></div>
      <h3 style="font-size:13px;margin:0 0 10px">共享给我的</h3>
      <div class="shares">${SHARED_IN.map(s => card(s, false)).join("")}</div>
      <h3 style="font-size:13px;margin:22px 0 10px">我共享出去的</h3>
      <div class="shares">${SHARED_OUT.map(s => card(s, true)).join("")}</div>
    </div>`;
}

A.invite = () => { S.modal = { type: "invite", step: 1, dev: "win-pc", p_cmd: true, p_read: true, p_write: false, p_shot: true, p_desk: false, p_agent: false, approve: true }; render(); };
A.acceptInvite = () => { S.modal = { type: "accept" }; render(); };
A.inviteNext = () => { S.modal.step = 2; render(); };

MODALS.invite = () => {
  const m = S.modal;
  if (m.step === 2) {
    return modal({
      title: "邀请链接已生成", sub: "把链接发给对方，对方用自己的 DeskBridge 打开",
      body: `<div class="snippet"><div class="codebox">deskbridge://invite/9f2c-7a1e-example</div><button class="btn sm" data-a="noop" data-msg="已复制邀请链接">${ic("copy")}复制</button></div>
        <div class="muted small" style="line-height:1.6">链接 24 小时内有效，只能使用一次。对方接受后，你会收到通知，并可以随时在这里撤销。</div>`,
      foot: `<button class="btn primary" data-a="closeModal">完成</button>`,
    });
  }
  const perm = (k, l, d) => `<div class="opt ${m[k] ? "on" : ""}" data-a="modalToggle" data-k="${k}" style="padding:9px 12px"><span class="check ${m[k] ? "on" : ""}"></span><div><div class="t">${l}</div><div class="d">${d}</div></div></div>`;
  return modal({
    title: "共享我的设备", sub: "对方只能使用你勾选的权限", wide: true,
    body: `<div class="field"><label>设备</label><select class="select"><option>win-pc</option><option>linux-server</option></select></div>
      <div class="field"><label>权限</label><div class="col" style="gap:6px">
        ${perm("p_cmd", "执行命令", "以你的用户身份运行命令")}
        ${perm("p_read", "读文件", "浏览和下载文件")}
        ${perm("p_write", "写文件", "上传、修改和删除文件")}
        ${perm("p_shot", "截图和界面元素", "查看屏幕内容")}
        ${perm("p_desk", "鼠标键盘", "操作桌面，受控制权规则约束")}
        ${perm("p_agent", "调用 Agent 和 MCP", "使用这台设备上的 AI 工具")}
      </div></div>
      <div class="row"><div class="field" style="flex:1"><label>有效期</label><select class="select"><option>7 天</option><option>30 天</option><option>长期</option></select></div>
        <div class="field" style="flex:1"><label>审批</label><div class="row" style="height:32px"><button class="switch ${m.approve ? "on" : ""}" data-a="modalToggle" data-k="approve"></button><span class="small">每次截图和写文件都要我同意</span></div></div></div>`,
    foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn primary" data-a="inviteNext">生成邀请链接</button>`,
  });
};
MODALS.accept = () => modal({
  title: "输入邀请链接", sub: "别人共享给你的设备会出现在“外部设备”分组",
  body: `<input class="input mono" placeholder="deskbridge://invite/…" id="inv">`,
  foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn primary" data-a="noop" data-msg="邀请无效或已过期（示例）">接受</button>`,
});

// ---------- 设置 ----------
const SET_SECS = [
  ["general", "sliders", "通用"],
  ["appearance", "palette", "外观"],
  ["thisdev", "laptop", "本机"],
  ["control", "hand", "桌面控制权"],
  ["network", "globe", "组网"],
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
      sgroup("语言和通知", [sitem("界面语言", "", sel("lang", ["简体中文", "English"])), sitem("审批通知", "有待审批的操作时弹出系统通知", `<button class="switch on"></button>`), sitem("AI 操作桌面时通知我", "AI 开始在任何设备上使用鼠标键盘时通知", `<button class="switch on"></button>`)]);
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
    case "thisdev": return `<h2>本机</h2><div class="sub">my-laptop 作为被控端时，允许其他设备和 AI 做什么</div>` +
      sgroup("设备", [sitem("设备名称", "其他设备和 AI 用这个名字找到本机", `<input class="input" value="my-laptop">`), sitem("内网地址", "", `<span class="mono">100.64.0.1</span>`)]) +
      sgroup("允许远程操作本机", [
        sitem("命令和文件", "执行命令、读写文件", sw("allowCmd")),
        sitem("截图和界面元素", "查看屏幕内容、读取窗口里的控件", sw("allowDesktop")),
        sitem("鼠标键盘", "模拟输入，受“桌面控制权”规则约束", sw("allowInput")),
        sitem("调用本机的 Agent 和 MCP 服务器", "规划中", sw("allowAgent")),
      ]);
    case "control": return `<h2>桌面控制权</h2><div class="sub">本地用户、远程人工和 AI 同时使用桌面时的规则。优先级：本地用户 &gt; 远程人工 &gt; AI</div>` +
      sgroup("自动释放", [
        sitem("本地用户停止操作后多久让 AI 继续", "本地一有鼠标键盘输入，AI 会立即停止；这个时间只决定何时继续", `${sel("release", ["1", "3", "5", "10"])}<span class="small muted">秒</span>`),
        sitem("远程一方的租约时长", "连接断开后，最多这么久自动释放控制权", `${sel("lease", ["10", "15", "30"])}<span class="small muted">秒</span>`),
        sitem("持有但不操作时自动释放", "", `${sel("idle", ["30", "60", "120"])}<span class="small muted">秒</span>`),
      ]) +
      sgroup("策略", [
        sitem("只在我离开时允许 AI 操作桌面", "本地无输入超过 5 分钟或屏幕已锁定时，才允许 AI 使用鼠标键盘", sw("onlyAway")),
        sitem("AI 使用鼠标键盘前先询问我", "在本机弹出确认", sw("askBeforeInput")),
      ]) +
      sgroup("紧急操作（在被控端本机使用）", [
        sitem("暂停所有远程操作", "中断远程的鼠标键盘，拒绝新的远程操作，后台命令继续", `<span class="kbd" style="margin:0">${s.hkPause}</span><button class="btn sm ghost">修改</button>`),
        sitem("终止", "暂停，并结束远程一方启动的所有进程", `<span class="kbd" style="margin:0">${s.hkStop}</span><button class="btn sm ghost">修改</button>`),
        sitem("已紧急停止的 AI", "被你在设备页停止的 AI 会话", `<span class="muted small">无</span>`),
      ]);
    case "network": return `<h2>组网</h2><div class="sub">内置组网（Tailscale），不需要另外安装</div>` +
      sgroup("状态", [
        sitem(`<span class="row">${netDown() ? `<span class="pulse bad"></span>已断开，正在重连` : `<span class="pulse"></span>已连接`}</span>`, netDown() ? "最近错误：无法连接控制服务器（超时）" : "本机 100.64.0.1 · 5 台设备 · 最近一次同步 10 秒前", `<button class="btn sm" data-a="${netDown() ? "netRetry" : "noop"}" data-msg="诊断完成：连接正常">${ic("refresh")}${netDown() ? "重试" : "诊断"}</button>`),
        sitem("账号", "user@example.com（GitHub 登录）", `<button class="btn sm ghost danger-soft" data-a="noop" data-msg="退出登录需要确认（原型未实现）">${ic("logout")}退出登录</button>`),
      ]) +
      sgroup("高级", [
        sitem("控制服务器", "可以改成自建的 Headscale", sel("server", ["默认（Tailscale）", "自定义…"])),
        sitem("授权密钥", "给没有浏览器的服务器登录用", `<button class="btn sm">生成密钥</button>`),
        sitem("自建中继", "暂时搁置（ADR-0006）", `<span class="soon">规划中</span>`),
      ]);
    case "ai": return `<h2>AI 接入</h2><div class="sub">让 Claude Code 等 AI Agent 通过 MCP 使用你的设备</div>` +
      `<div class="sgroup"><h3>MCP 配置</h3><div class="card"><div class="card-b col" style="gap:10px">
        <div class="muted small">把下面的配置加到 Agent 的 MCP 设置里（例如 Claude Code 的 <span class="mono">.mcp.json</span>）：</div>
        <div class="snippet"><div class="codebox">{
  "mcpServers": {
    "deskbridge": { "command": "deskbridge", "args": ["mcp"] }
  }
}</div><button class="btn sm" data-a="noop" data-msg="已复制 MCP 配置">${ic("copy")}复制</button></div>
        <div class="muted small">或者在命令行运行：<span class="mono">claude mcp add deskbridge -- deskbridge mcp</span></div></div></div></div>` +
      sgroup("已连接的 AI 会话", [
        sitem(`<span class="row">${avatar("claude", "sm")}Claude Code</span>`, "linux-server · 已连接 42 分钟 · 调用 128 次", `<button class="btn sm ghost danger-soft" data-a="noop" data-msg="已断开">断开</button>`),
        sitem(`<span class="row">${avatar("codex", "sm")}Codex</span>`, "win-pc · 已连接 3 小时 · 调用 16 次", `<button class="btn sm ghost danger-soft" data-a="noop" data-msg="已断开">断开</button>`),
      ]);
    case "about": return `<h2>关于</h2><div class="sub">DeskBridge 0.3.0</div>` +
      sgroup("", [sitem("版本", "0.3.0（2026-10-07）", `<button class="btn sm" data-a="noop" data-msg="已是最新版本">检查更新</button>`), sitem("开源许可证", "以及使用的第三方组件", `<button class="btn sm ghost">${ic("external")}查看</button>`), sitem("日志", "遇到问题时可以附上日志", `<button class="btn sm ghost">打开日志目录</button>`)]);
  }
  return "";
}

function viewSettings() {
  const sec = S.route.sec;
  if (isPhone()) {
    if (!sec) {
      return `${phoneTop("设置")}<div class="row" style="padding:4px 16px 14px;gap:12px"><span class="avatar" style="width:44px;height:44px;font-size:16px">示</span><div><b>示例用户</b><div class="small muted">user@example.com</div></div></div>
        <div class="mlist">${SET_SECS.map(([k, i, l]) => `<a data-go="#/settings/${k}"><span class="ic">${ic(i)}</span>${l}<span class="r">${ic("chevRight")}</span></a>`).join("")}
        <a data-go="#/shared"><span class="ic">${ic("users")}</span>外部设备<span class="r">${ic("chevRight")}</span></a></div>`;
    }
    return `<div class="mtop"><button class="btn ghost icon" data-go="#/settings">${ic("chevLeft", "lg")}</button><h2>${SET_SECS.find(s => s[0] === sec)[2]}</h2></div><div class="sbody">${settingsSec(sec).replace(/<h2>.*?<\/h2>/, "")}</div>`;
  }
  return `<div class="settings">
    <nav class="snav">${SET_SECS.map(([k, i, l]) => `<a class="${sec === k ? "on" : ""}" data-go="#/settings/${k}">${ic(i)}<span class="label">${l}</span></a>`).join("")}</nav>
    <div class="sbody" data-sk="set">${settingsSec(sec)}</div>
  </div>`;
}

// ---------- 首次登录 ----------
A.onbNext = () => {
  const o = S.onb;
  o.step += 1;
  render();
  if (o.step === 1 && S.scenario !== "error") setTimeout(() => { if (S.onb.step === 1) { S.onb.step = 2; render(); } }, 2600);
};
A.onbBack = () => { S.onb.step = Math.max(0, S.onb.step - 1); render(); };
A.onbAdv = () => { S.onb.adv = !S.onb.adv; render(); };
A.onbToggle = el => { S.onb[el.dataset.k] = !S.onb[el.dataset.k]; render(); };
A.onbDone = () => { S.onb.step = 0; go("#/devices/win-pc/overview"); toast("ok", "欢迎使用 DeskBridge", "已加入内网，发现 5 台设备"); };

function viewWelcome() {
  const o = S.onb;
  const steps = `<div class="onb-steps">${[0, 1, 2, 3].map(i => `<i class="${i <= o.step ? "on" : ""}"></i>`).join("")}</div>`;
  let c;
  if (o.step === 0) {
    c = `<span class="logo big">${ic("bridge")}</span>
      <div><h1>欢迎使用 DeskBridge</h1><p class="lead" style="margin-top:6px">让你和 AI 都能操作自己的所有设备：命令、文件、截图和桌面。</p></div>
      <div class="col" style="gap:12px">
        <div class="feature"><div class="fi">${ic("globe")}</div><div><div class="t">内置组网</div><div class="d">不需要公网 IP 或另装 VPN，设备之间直接加密连接</div></div></div>
        <div class="feature"><div class="fi">${ic("bot")}</div><div><div class="t">给 AI 用</div><div class="d">Claude Code 等 Agent 通过 MCP 操作你的设备</div></div></div>
        <div class="feature"><div class="fi">${ic("shield")}</div><div><div class="t">看得见、停得住</div><div class="d">每次操作都有记录；你动一下鼠标，AI 就让出桌面</div></div></div>
      </div>
      <button class="btn primary lg block" data-a="onbNext">${ic("external")}用浏览器登录</button>
      <div class="small muted" style="text-align:center">支持 Google、GitHub、Microsoft 等账号</div>
      <a class="small" style="text-align:center;color:var(--muted);cursor:pointer" data-a="onbAdv">${o.adv ? "收起" : "使用自建的控制服务器"}</a>
      ${o.adv ? `<div class="field"><label>控制服务器地址</label><input class="input mono" placeholder="https://headscale.example.com"><div class="hint">兼容 Headscale。留空使用默认服务器。</div></div>` : ""}`;
  } else if (o.step === 1) {
    c = S.scenario === "error"
      ? `<div class="empty" style="padding:10px 0"><div class="art bad">${ic("wifiOff", "xl")}</div><h3>无法连接登录服务器</h3><p>请检查网络。如果你在中国大陆，部分网络可能无法访问默认服务器，可以稍后重试或使用自建的控制服务器。</p><p><span class="errcode">control_unreachable · 超时 15 秒</span></p></div>
        <div class="row"><button class="btn" data-a="onbBack">返回</button><span class="spacer"></span><button class="btn primary" data-a="onbRetry">${ic("refresh")}重试</button></div>`
      : `<div class="empty" style="padding:16px 0"><span class="spin lg"></span><h3 style="margin-top:8px">请在浏览器中完成登录</h3><p>已在默认浏览器中打开登录页面。登录完成后会自动回到这里。</p></div>
        <div class="row" style="justify-content:center"><button class="btn sm" data-a="noop" data-msg="已复制登录链接">${ic("copy")}复制登录链接</button><button class="btn sm ghost" data-a="noop" data-msg="已重新打开浏览器">重新打开浏览器</button></div>
        <button class="btn ghost" data-a="onbBack">取消</button>`;
  } else if (o.step === 2) {
    const t = (k, l, d) => `<div class="opt ${o[k] ? "on" : ""}" data-a="onbToggle" data-k="${k}" style="padding:10px 12px"><span class="check ${o[k] ? "on" : ""}"></span><div><div class="t">${l}</div><div class="d">${d}</div></div></div>`;
    c = `<div><h1>设置这台电脑</h1><p class="lead" style="margin-top:6px">已登录 user@example.com。</p></div>
      <div class="field"><label>设备名称</label><input class="input" value="${esc(o.name)}" data-in="onb.name"><div class="hint">其他设备和 AI 用这个名字找到它</div></div>
      <div class="field"><label>允许其他设备和 AI 操作这台电脑</label><div class="col" style="gap:6px">
        ${t("allowCmd", "命令和文件", "执行命令、读写文件")}
        ${t("allowDesk", "截图和界面元素", "查看屏幕内容")}
        ${t("allowInput", "鼠标键盘", "AI 操作时会有明显提示，你动鼠标就能抢回")}
      </div><div class="hint">只用来控制别的设备的话，可以都不勾。以后可以在设置中修改。</div></div>
      <div class="row"><button class="switch ${o.autostart ? "on" : ""}" data-a="onbToggle" data-k="autostart"></button><span>开机时自动启动</span></div>
      <button class="btn primary lg block" data-a="onbNext">继续</button>`;
  } else {
    c = `<div class="empty" style="padding:6px 0"><div class="art accent" style="background:var(--ok-soft);color:var(--ok)">${ic("check", "xl")}</div><h3>已加入你的内网</h3><p>发现 5 台设备，其中 3 台运行着 DeskBridge。</p></div>
      <div class="card">${DEVICES.filter(d => !d.self && d.group === "mine").slice(0, 4).map(d => `<div class="dev" style="margin:0;cursor:default"><div class="dic">${ic(kindIcon(d))}${dot(d)}</div><div class="main"><div class="n">${d.name}</div><div class="m">${d.os}</div></div></div>`).join("")}</div>
      <div class="banner info">${ic("plug")}<div class="t">要让 AI 使用这些设备，可以在“设置 → AI 接入”复制 MCP 配置。</div></div>
      <button class="btn primary lg block" data-a="onbDone">进入 DeskBridge</button>`;
  }
  const inner = `<div class="onb"><div class="card onb-card">${steps}${c}</div></div>`;
  return isPhone() ? `<div class="mshell">${inner}</div>` : inner;
}
A.onbRetry = () => { S.scenario = "normal"; S.onb.step = 0; A.onbNext(); };
