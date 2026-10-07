// 其他页面：操作记录、访问权限（含共享）、设置、首次登录。

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
  else if (S.scenario === "empty" || !items.length) body = `<div class="empty"><div class="art">${ic("history", "xl")}</div><h3>还没有操作记录</h3><p>你和 AI 在各设备上的每一次操作都会记录在这里，方便回看和检查 AI 做了什么。</p></div>`;
  else {
    const days = [...new Set(items.map(a => a.day))];
    body = days.map(dy => `<div class="day">${dy}</div><div class="list2">${items.filter(a => a.day === dy).map(actItem).join("")}</div>`).join("");
  }
  const filtered = S.act.dev !== "all";
  return `<div class="v2-page" data-sk="act"><div class="v2-in">
    ${pageHead("操作记录", "谁（AI 或人）在哪台设备上做了什么", isPhone() ? "" : `<button class="btn" data-a="noop" data-msg="已导出为 CSV">${ic("download")}导出</button>`)}
    <div class="row" style="flex-wrap:wrap;gap:10px">
      <div class="segmented">${seg("all", "全部")}${seg("ai", "AI")}${seg("human", "人")}${seg("ext", "外部")}</div>
      <button class="btn ${filtered ? "primary" : ""}" data-a="popActFilter">${ic("filter")}${filtered ? S.act.dev : "筛选"}</button>
      ${filtered ? `<button class="btn ghost sm" data-a="actClear">清除</button>` : ""}
      <span class="spacer"></span>${isPhone() ? "" : `<span class="pill ok"><span class="pulse live"></span>实时</span>`}
    </div>
    ${body}
  </div></div>`;
}

A.actWho = el => { S.act.who = el.dataset.v; render(); };
A.actClear = () => { S.act.dev = "all"; render(); };
A.actDrawer = el => { S.drawer = { type: "act", i: +el.dataset.i }; render(); };
A.popActFilter = el => openPop(el, "actFilter");
A.actDev = el => { S.act.dev = el.dataset.v; S.pop = null; render(); };
POPS.actFilter = () => `<div class="ph2">设备</div>${[{ id: "all", name: "全部设备" }].concat(DEVICES.filter(d => !d.self && !d.noNode)).map(d => `<div class="pi" data-a="actDev" data-v="${d.id}"><span class="radio ${S.act.dev === d.id ? "on" : ""}"></span>${d.name}</div>`).join("")}
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
      <div><div class="small muted" style="margin-bottom:8px">结果</div><div class="codebox">${esc(a.out)}</div></div>
      ${expander("actArgs", "terminal", "参数", "敏感字段已脱敏", `<div class="codebox">${esc(a.args)}</div><div class="small muted">请求编号 <span class="mono">req_7f3a${S.drawer.i}c91e</span></div>`)}`,
    foot: `<button class="btn" data-go="#/devices/${a.dev}/overview">${ic("monitor")}打开设备</button><button class="btn ghost" data-a="noop" data-msg="已复制请求编号">${ic("copy")}复制编号</button>`,
  });
};

// ---------- 访问权限（被控端）与操作确认（发起端） ----------
// 被控端只按权限放行或拒绝，不弹窗等人；危险操作的确认在发起端完成。
function permSummary(g) {
  if (g.locked) return "完全访问 · 全部设备";
  const on = PERM_DEFS.filter(([, , items]) => items.some(([k]) => g.p[k])).map(([grp]) => grp);
  const scope = g.scope.length ? `（文件限 ${g.scope.length} 个文件夹）` : "";
  return `${g.devs} · ${on.length ? on.join("、") : "没有权限"}${scope}`;
}
function presetOf(g) {
  if (g.locked) return "full";
  return Object.keys(PRESETS).find(k => PERM_DEFS.every(([, , items]) => items.every(([x]) => !!g.p[x] === !!PRESETS[k].p[x])) && g.scope.join() === PRESETS[k].scope.join()) || "custom";
}
function presetName(k) { return k === "custom" ? "自定义" : PRESETS[k].name; }

function viewAccess() {
  const row = g => {
    const pk = presetOf(g);
    return `<div class="li" data-a="grantee" data-id="${g.id}"><span class="avatar ${g.avc}">${g.av}</span>
      <div class="b"><div class="t"><b>${g.name}</b></div><div class="d">${permSummary(g)}</div></div>
      <div class="r">${g.exp ? `<span class="hide-m">到 ${g.exp}</span>` : ""}<span class="pill ${pk === "full" ? "line" : pk === "custom" ? "info" : "line"}">${presetName(pk)}</span>${ic("chevRight", "sm")}</div></div>`;
  };
  const mine = S.grantees.filter(g => g.kind !== "ext");
  const ext = S.grantees.filter(g => g.kind === "ext");
  return `<div class="v2-page" data-sk="access"><div class="v2-in" style="max-width:820px">
    ${pageHead("访问权限", "谁能在你的设备上做什么。权限保存在被控设备上，由被控设备执行", isPhone() ? `<button class="btn primary icon" data-a="invite">${ic("share")}</button>` : `<button class="btn" data-a="acceptInvite">${ic("link")}输入邀请链接</button><button class="btn primary" data-a="invite">${ic("share")}共享给别人</button>`)}
    <section><div class="sec-h"><h2>我自己</h2></div><div class="list2">${mine.map(row).join("")}</div></section>
    <section><div class="sec-h"><h2>共享给别人</h2><span class="count">${ext.length}</span></div>
      ${ext.length ? `<div class="list2">${ext.map(row).join("")}</div>` : `<div class="muted small">还没有共享给别人。</div>`}</section>
    <section><div class="sec-h"><h2>别人共享给我</h2><span class="count">${SHARED_IN.length}</span></div>
      <div class="list2">${SHARED_IN.map(s => `<div class="li" data-go="#/devices/${s.dev}/overview"><span class="fname">${ic("monitor", "lg")}</span>
        <div class="b"><div class="t"><b>${s.dev}</b> <span class="muted">来自 ${s.owner}</span></div><div class="d">${s.perms.join("、")} · 有效期 ${s.exp}</div></div><div class="r">${ic("chevRight", "sm")}</div></div>`).join("")}</div></section>
    ${expander("howConfirm", "info", "危险操作在哪里确认？", "发起端确认，被控端只按权限执行", `<div class="muted" style="line-height:1.75">
      被控设备不会弹窗等人批准。你的 AI 想删除文件、改注册表等操作时，确认请求发给<b>发起的一方</b>：你自己的 DeskBridge 窗口、手机或 AI 客户端。规则在 <a data-go="#/settings/confirm" style="cursor:pointer;text-decoration:underline">设置 → 操作确认</a> 中修改。<br>
      别人的 AI 由对方自己确认；你只需要决定给对方哪些权限。本地用户仍然可以随时暂停或终止所有远程操作。</div>`)}
  </div></div>`;
}

// 正在编辑的权限：邀请对话框或权限抽屉
const ed = () => (S.modal && S.modal.type === "invite" ? S.modal : S.drawer);
A.grantee = el => { const g = S.grantees.find(x => x.id === el.dataset.id); S.drawer = { type: "grantee", id: g.id, p: { ...g.p }, scope: [...g.scope], allFiles: !g.scope.length }; render(); };
A.gPerm = el => { const e = ed(), k = el.dataset.k; e.p[k] = e.p[k] ? 0 : 1; render(); };
A.gPreset = el => { const e = ed(), pr = PRESETS[el.dataset.v]; e.p = { ...pr.p }; e.scope = [...pr.scope]; e.allFiles = !pr.scope.length; render(); };
A.gScope = el => { const e = ed(); e.allFiles = el.dataset.v === "all"; if (!e.allFiles && !e.scope.length) e.scope = ["D:\\share"]; render(); };
A.gScopeDel = el => { ed().scope.splice(+el.dataset.i, 1); render(); };
A.gScopeAdd = () => { ed().scope.push("D:\\data\\notes"); render(); };
A.gSave = () => {
  const dr = S.drawer, g = S.grantees.find(x => x.id === dr.id);
  g.p = dr.p; g.scope = dr.allFiles ? [] : dr.scope; S.drawer = null;
  toast("ok", `已保存 ${g.name} 的权限`, "已同步到相关设备，下一次操作开始生效"); render();
};
A.gRevoke = () => { const g = S.grantees.find(x => x.id === S.drawer.id); S.grantees = S.grantees.filter(x => x !== g); S.drawer = null; toast("ok", `已撤销对 ${g.name} 的共享`, "对方正在进行的操作会被中断"); render(); };

// 权限编辑器：抽屉和邀请对话框共用。st = { p, scope, allFiles }
function permEditor(st) {
  const pk = presetOf({ p: st.p, scope: st.allFiles ? [] : st.scope });
  const groups = PERM_DEFS.map(([grp, icon, items]) => `<div class="pgroup"><div class="pg-h">${ic(icon, "sm")}${grp}</div>
    ${items.map(([k, l, d]) => `<label class="prow" data-a="gPerm" data-k="${k}"><div><div>${l}</div>${d ? `<div class="d">${d}</div>` : ""}</div><button class="switch ${st.p[k] ? "on" : ""}"></button></label>`).join("")}
    ${grp === "文件" && (st.p.read || st.p.write || st.p.del) ? `<div class="pscope">
      <div class="segmented"><button class="${st.allFiles ? "on" : ""}" data-a="gScope" data-v="all">全部文件</button><button class="${st.allFiles ? "" : "on"}" data-a="gScope" data-v="some">只限这些文件夹</button></div>
      ${st.allFiles ? "" : `<div class="chips">${st.scope.map((x, i) => `<span class="tag mono">${ic("folder", "sm")}${esc(x)}<a data-a="gScopeDel" data-i="${i}" title="移除">${ic("x", "sm")}</a></span>`).join("")}<button class="btn ghost sm" data-a="gScopeAdd">${ic("plus")}添加文件夹</button></div>`}
    </div>` : ""}</div>`).join("");
  const leak = (st.p.exec || st.p.kbd) && (!st.allFiles || !st.p.del);
  return `<div class="field"><label>预设</label><div class="segmented">${["full", "standard", "view"].map(k => `<button class="${pk === k ? "on" : ""}" data-a="gPreset" data-v="${k}">${PRESETS[k].name}</button>`).join("")}<button class="${pk === "custom" ? "on" : ""}" style="pointer-events:none">自定义</button></div></div>
    ${leak ? `<div class="banner warn">${ic("alert")}<div class="t" style="white-space:normal;line-height:1.6">允许“执行命令”或“键盘”后，对方可以用命令绕过文件范围和删除限制。需要严格限制时，请同时关闭这两项。</div></div>` : ""}
    ${groups}`;
}

DRAWERS.grantee = () => {
  const dr = S.drawer, g = S.grantees.find(x => x.id === dr.id);
  if (g.locked) {
    return drawer({
      title: g.name, sub: g.sub,
      body: `<div class="banner info">${ic("info")}<div class="t" style="white-space:normal;line-height:1.6">你本人的人工操作不受限制。即使这样，被控设备的本地用户仍然优先，可以随时暂停或终止远程操作。</div></div>
        <dl class="kv"><dt>适用设备</dt><dd>全部设备</dd><dt>权限</dt><dd>完全访问</dd></dl>`,
    });
  }
  return drawer({
    title: g.name, sub: g.sub,
    body: `<div class="field"><label>适用设备</label><select class="select"><option>${g.devs}</option>${g.kind === "ext" ? "" : `<option>只选部分设备…</option>`}</select></div>
      ${g.override ? expander("override", "monitor", "按设备单独设置", `${g.override.length} 台设备与上面不同`, `<div class="list2">${g.override.map(o => `<div class="li" style="cursor:default"><div class="b"><div class="t">${o.dev}</div><div class="d">${o.t}</div></div><button class="btn ghost sm">修改</button></div>`).join("")}</div>`) : ""}
      ${permEditor(dr)}
      ${g.exp ? `<div class="field"><label>有效期</label><select class="select"><option>到 ${g.exp}</option><option>延长 30 天</option><option>长期</option></select></div>` : ""}`,
    foot: `<button class="btn primary" data-a="gSave">保存</button><button class="btn" data-a="closeDrawer">取消</button><span class="spacer"></span>${g.kind === "ext" ? `<button class="btn ghost danger-soft" data-a="gRevoke">撤销共享</button>` : ""}`,
  });
};

// 发起端确认对话框
A.openConfirm = () => { if (!S.confirm.items.length) return; S.modal = { type: "confirm" }; render(); };
A.confirmDo = el => {
  const a = S.confirm.items.shift(), r = el.dataset.r;
  S.modal = null;
  toast(r === "deny" ? "warn" : "ok", { deny: "已拒绝", once: "已允许一次", session: "本次会话内都允许" }[r], `${ACTORS[a.actor].name} · ${a.what.slice(0, 40)}`);
  render();
};
MODALS.confirm = () => {
  const a = S.confirm.items[0], actor = ACTORS[a.actor];
  const verb = a.tool === "proc.exec" ? "执行命令" : a.tool;
  return modal({
    icon: `<span class="avatar ${actor.avc}" style="width:36px;height:36px">${actor.av}</span>`,
    title: `${actor.name} 想在 ${a.dev} 上${verb}`, sub: `你在 ${actor.from} 上的 AI 会话 · 命中规则“${a.rule}”`,
    body: `<div class="codebox" style="font-size:13px;padding:10px 12px">${esc(a.what)}</div>
      <div class="muted" style="line-height:1.7">${a.reason}</div>
      <div class="small muted row">${ic("clock", "sm")}<span data-tmr="${a.id}">${Math.floor(a.wait / 60)}:${String(a.wait % 60).padStart(2, "0")} 后自动拒绝</span><span class="spacer"></span><a data-go="#/settings/confirm" style="cursor:pointer;text-decoration:underline">修改规则</a></div>`,
    foot: `<button class="btn ghost" data-a="confirmDo" data-r="session">本次会话都允许</button><span class="spacer"></span><button class="btn" data-a="confirmDo" data-r="deny">拒绝</button><button class="btn primary" data-a="confirmDo" data-r="once">允许一次</button>`,
  });
};

// ---------- 共享给别人 ----------
A.invite = () => { S.modal = { type: "invite", step: 1, dev: "win-pc", p: { ...PRESETS.view.p }, scope: [...PRESETS.view.scope], allFiles: false }; render(); };
A.acceptInvite = () => { S.modal = { type: "accept" }; render(); };
A.inviteNext = () => { S.modal.step = 2; render(); };

MODALS.invite = () => {
  const m = S.modal;
  if (m.step === 2) {
    return modal({
      title: "邀请链接已生成", sub: "把链接发给对方，对方用自己的 DeskBridge 打开",
      body: `<div class="snippet"><div class="codebox">deskbridge://invite/9f2c-7a1e-example</div><button class="btn sm" data-a="noop" data-msg="已复制邀请链接">${ic("copy")}复制</button></div>
        <div class="muted small" style="line-height:1.6">链接 24 小时内有效，只能使用一次。对方接受后你会收到通知，可以随时在“访问权限”中修改或撤销。</div>`,
      foot: `<button class="btn primary" data-a="closeModal">完成</button>`,
    });
  }
  const editor = permEditor(m);
  return modal({
    title: "共享给别人", sub: "对方只能使用你打开的权限，他们的 AI 操作由他们自己确认",
    body: `<div class="field"><label>设备</label><select class="select"><option>win-pc</option><option>media-pc</option><option>linux-server</option></select></div>
      ${expander("invitePerm", "shield", "权限", permSummary({ p: m.p, scope: m.allFiles ? [] : m.scope, devs: "win-pc" }), editor)}
      <div class="field"><label>有效期</label><select class="select"><option>7 天</option><option>30 天</option><option>长期</option></select></div>`,
    foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn primary" data-a="inviteNext">生成邀请链接</button>`,
  });
};
MODALS.accept = () => modal({
  title: "输入邀请链接", sub: "别人共享给你的设备会出现在设备列表的“外部设备”分组",
  body: `<input class="input mono" placeholder="deskbridge://invite/…" id="inv">`,
  foot: `<button class="btn" data-a="closeModal">取消</button><button class="btn primary" data-a="noop" data-msg="邀请无效或已过期（示例）">接受</button>`,
});

// ---------- 设置 ----------
const SET_SECS = [
  ["general", "sliders", "通用"],
  ["appearance", "palette", "外观"],
  ["thisdev", "laptop", "本机"],
  ["control", "hand", "桌面控制权"],
  ["confirm", "shieldAlert", "操作确认"],
  ["network", "globe", "连接方式"],
  ["ai", "plug", "AI 接入"],
  ["about", "info", "关于"],
];

A.cruleToggle = el => { const r = S.crules[+el.dataset.i]; r.on = !r.on; render(); };
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
      sgroup("远程画面", [sitem("打开方式", "独立窗口可以同时操作多台设备，每台一个窗口", sel("remoteMode", ["独立窗口", "在主窗口中打开"]))]) +
      sgroup("语言和通知", [sitem("界面语言", "", sel("lang", ["简体中文", "English"])), sitem("确认请求通知", "你的 AI 有操作等待确认时弹出系统通知", `<button class="switch on"></button>`), sitem("AI 操作桌面时通知我", "AI 开始在任何设备上使用鼠标键盘时通知", `<button class="switch on"></button>`)]);
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
      sgroup("设备", [sitem("设备名称", "其他设备和 AI 用这个名字找到本机", `<input class="input" value="my-laptop">`), sitem("地址", "Tailscale", `<span class="mono">100.64.0.1</span>`)]) +
      sgroup("谁能访问本机", [sitem("访问权限", "我本人：完全访问 · 我的 AI：标准 · bob：未共享本机", `<button class="btn sm" data-go="#/access">查看${ic("chevRight", "sm")}</button>`)]);
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
        sitem("暂停所有远程操作", "中断远程的鼠标键盘，拒绝新的远程操作，后台命令继续", `<span class="kbd" style="margin:0">${s.hkPause}</span><button class="btn sm ghost">修改</button>`),
        sitem("终止", "暂停，并结束远程一方启动的所有进程", `<span class="kbd" style="margin:0">${s.hkStop}</span><button class="btn sm ghost">修改</button>`),
        sitem("已紧急停止的 AI", "被你在设备页停止的 AI 会话", `<span class="muted small">无</span>`),
      ]);
    case "confirm": return `<h2>操作确认</h2><div class="sub">你的 AI 发起这些操作前，先请你确认。确认在发起的一方完成，被控设备不会弹窗等人</div>` +
      sgroup("需要确认的操作", S.crules.map((r, i) => sitem(r.t, r.d, `<button class="switch ${r.on ? "on" : ""}" data-a="cruleToggle" data-i="${i}"></button>`))) +
      sgroup("确认方式", [
        sitem("在哪里确认", "优先在 AI 客户端里询问（需要客户端支持 MCP 的确认请求，待验证）；否则发到你已登录的 DeskBridge 窗口和手机", `<span class="muted small">自动</span>`),
        sitem("等待多久", "没人处理时自动拒绝", sel("confirmWait", ["1 分钟", "5 分钟", "15 分钟"])),
      ]) +
      `<div class="small muted" style="line-height:1.7">别人的 AI 操作你的设备时，由对方自己确认；你在 <a data-go="#/access" style="cursor:pointer;text-decoration:underline">访问权限</a> 中决定给对方哪些权限。</div>`;
    case "network": {
      const ts = CONNECTORS[0];
      return `<h2>连接方式</h2><div class="sub">DeskBridge 通过连接层和其他设备通信。目前支持 Tailscale，以后会增加更多方式</div>` +
      sgroup(`<span class="row">${ic("globe", "sm")}Tailscale<span class="pill ${netDown() ? "bad" : "ok"}">${netDown() ? "已断开" : "已连接"}</span></span>`, [
        sitem(`<span class="row">${netDown() ? `<span class="pulse bad"></span>正在重连` : `<span class="pulse"></span>本机 <span class="mono">${ts.ip}</span>`}</span>`, netDown() ? "最近错误：无法连接控制服务器（超时）" : `${ts.devs} 台设备 · 最近一次同步 10 秒前`, `<button class="btn sm" data-a="${netDown() ? "netRetry" : "noop"}" data-msg="诊断完成：连接正常">${ic("refresh")}${netDown() ? "重试" : "诊断"}</button>`),
        sitem("账号", ts.acct, `<button class="btn sm ghost danger-soft" data-a="noop" data-msg="退出登录需要确认（原型未实现）">${ic("logout")}退出登录</button>`),
        sitem("控制服务器", "可以改成自建的 Headscale", sel("server", ["默认（Tailscale）", "自定义…"])),
        sitem("使用系统已安装的 Tailscale", "检测到时直接使用它，不再单独加入网络，同一台设备不会出现两次", `<button class="switch"></button>`),
        sitem("授权密钥", "给没有浏览器的服务器登录用", `<button class="btn sm">生成密钥</button>`),
      ]) +
      sgroup(`<span class="row">${ic("link", "sm")}WireGuard 直连<span class="soon">规划中</span></span>`, [
        sitem("两台设备配对后直接连接", "不需要账号。适合局域网内、或不想使用 Tailscale 的情况（任务 19）", `<button class="btn sm" disabled style="opacity:.5">配对设备</button>`),
      ]) +
      sgroup("选路", [
        sitem("多种方式都能连通时", "自动选择最快的路径：局域网直连 > 跨网直连 > 中继", `<span class="muted small">自动</span>`),
        sitem("自建中继", "暂时搁置（ADR-0006）", `<span class="soon">规划中</span>`),
      ]);
    }
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
</div>`;
    }
    return `<div class="mtop"><button class="btn ghost icon" data-go="#/settings">${ic("chevLeft", "lg")}</button><h2>${SET_SECS.find(s => s[0] === sec)[2]}</h2></div><div class="sbody">${settingsSec(sec).replace(/<h2>.*?<\/h2>/, "")}</div>`;
  }
  return `<div class="settings">
    <nav class="snav"><div class="small muted" style="padding:6px 10px 10px;font-weight:600">设置</div>${SET_SECS.map(([k, i, l]) => `<a class="${sec === k ? "on" : ""}" data-go="#/settings/${k}">${ic(i)}<span class="label">${l}</span></a>`).join("")}</nav>
    <div class="sbody" data-sk="set">${settingsSec(sec)}</div>
  </div>`;
}

// ---------- 首次登录 ----------
// 欢迎 → 选择连接方式 → 登录 → 设置本机 → 完成
A.onbNext = () => {
  const o = S.onb;
  o.step += 1;
  render();
  if (o.step === 2 && S.scenario !== "error") setTimeout(() => { if (S.onb.step === 2) { S.onb.step = 3; render(); } }, 2600);
};
A.onbBack = () => { S.onb.step = Math.max(0, S.onb.step - 1); render(); };
A.onbConn = el => { S.onb.conn = el.dataset.v; render(); };
A.onbToggle = el => { S.onb[el.dataset.k] = !S.onb[el.dataset.k]; render(); };
A.onbDone = () => { S.onb.step = 0; go("#/devices/win-pc/overview"); toast("ok", "欢迎使用 DeskBridge", "已连接到 Tailscale 网络，发现 6 台设备"); };

function viewWelcome() {
  const o = S.onb;
  const steps = `<div class="onb-steps">${[0, 1, 2, 3, 4].map(i => `<i class="${i <= o.step ? "on" : ""}"></i>`).join("")}</div>`;
  let c;
  if (o.step === 0) {
    c = `<span class="logo big">${ic("bridge")}</span>
      <div><h1>欢迎使用 DeskBridge</h1><p class="lead" style="margin-top:6px">让你和 AI 都能操作自己的所有设备：命令、文件、截图和桌面。</p></div>
      <div class="col" style="gap:12px">
        <div class="feature"><div class="fi">${ic("globe")}</div><div><div class="t">设备直接互连</div><div class="d">不需要公网 IP，设备之间加密连接</div></div></div>
        <div class="feature"><div class="fi">${ic("bot")}</div><div><div class="t">给 AI 用</div><div class="d">Claude Code 等 Agent 通过 MCP 操作你的设备</div></div></div>
        <div class="feature"><div class="fi">${ic("shield")}</div><div><div class="t">看得见、停得住</div><div class="d">每次操作都有记录；你动一下鼠标，AI 就让出桌面</div></div></div>
      </div>
      <button class="btn primary lg block" data-a="onbNext">开始使用</button>`;
  } else if (o.step === 1) {
    const opt = (v, icon, t, d, dis) => `<div class="opt ${o.conn === v ? "on" : ""} ${dis ? "dis" : ""}" ${dis ? "" : `data-a="onbConn" data-v="${v}"`}><span class="radio ${o.conn === v ? "on" : ""}"></span><div style="flex:1"><div class="t">${t}${dis ? `<span class="soon">规划中</span>` : ""}</div><div class="d">${d}</div></div>${ic(icon)}</div>`;
    c = `<div><h1>选择连接方式</h1><p class="lead" style="margin-top:6px">设备之间怎样互相找到、建立加密连接。以后可以在设置中增加其他方式。</p></div>
      <div class="col" style="gap:8px">
        ${opt("tailscale", "globe", "Tailscale 账号（推荐）", "用 Google、GitHub、Microsoft 等账号登录，同一账号的设备自动发现。不需要另装 Tailscale")}
        ${opt("headscale", "server", "自建 Headscale 服务器", "和 Tailscale 兼容，登录服务器由你自己部署")}
        ${opt("wireguard", "link", "WireGuard 直连", "不需要账号，两台设备配对后直接连接", true)}
      </div>
      ${o.conn === "headscale" ? `<div class="field"><label>服务器地址</label><input class="input mono" placeholder="https://headscale.example.com"></div>` : ""}
      <div class="row"><button class="btn" data-a="onbBack">返回</button><span class="spacer"></span><button class="btn primary" data-a="onbNext">${ic("external")}用浏览器登录</button></div>`;
  } else if (o.step === 2) {
    c = S.scenario === "error"
      ? `<div class="empty" style="padding:10px 0"><div class="art bad">${ic("wifiOff", "xl")}</div><h3>无法连接登录服务器</h3><p>请检查网络。如果你在中国大陆，部分网络可能无法访问默认服务器，可以稍后重试或改用自建的 Headscale。</p><p><span class="errcode">control_unreachable · 超时 15 秒</span></p></div>
        <div class="row"><button class="btn" data-a="onbBack">换一种方式</button><span class="spacer"></span><button class="btn primary" data-a="onbRetry">${ic("refresh")}重试</button></div>`
      : `<div class="empty" style="padding:16px 0"><span class="spin lg"></span><h3 style="margin-top:8px">请在浏览器中完成登录</h3><p>已在默认浏览器中打开 ${o.conn === "headscale" ? "Headscale" : "Tailscale"} 的登录页面。登录完成后会自动回到这里。</p></div>
        <div class="row" style="justify-content:center"><button class="btn sm" data-a="noop" data-msg="已复制登录链接">${ic("copy")}复制登录链接</button><button class="btn sm ghost" data-a="noop" data-msg="已重新打开浏览器">重新打开浏览器</button></div>
        <button class="btn ghost" data-a="onbBack">取消</button>`;
  } else if (o.step === 3) {
    const t = (k, l, d) => `<div class="opt ${o[k] ? "on" : ""}" data-a="onbToggle" data-k="${k}" style="padding:10px 12px"><span class="check ${o[k] ? "on" : ""}"></span><div><div class="t">${l}</div><div class="d">${d}</div></div></div>`;
    c = `<div><h1>设置这台电脑</h1><p class="lead" style="margin-top:6px">已登录 user@example.com。</p></div>
      <div class="field"><label>设备名称</label><input class="input" value="${esc(o.name)}" data-in="onb.name"><div class="hint">其他设备和 AI 用这个名字找到它</div></div>
      <div class="field"><label>允许你的 AI 在这台电脑上</label><div class="col" style="gap:6px">
        ${t("allowCmd", "执行命令、读写文件", "")}
        ${t("allowDesk", "截图、读取界面元素", "")}
        ${t("allowInput", "使用鼠标键盘", "AI 操作时会有明显提示，你动鼠标就能抢回")}
      </div><div class="hint">你本人始终可以操作。以后可以在“访问权限”中细调。</div></div>
      <div class="row"><button class="switch ${o.autostart ? "on" : ""}" data-a="onbToggle" data-k="autostart"></button><span>开机时自动启动</span></div>
      <button class="btn primary lg block" data-a="onbNext">继续</button>`;
  } else {
    c = `<div class="empty" style="padding:6px 0"><div class="art accent" style="background:var(--ok-soft);color:var(--ok)">${ic("check", "xl")}</div><h3>已连接</h3><p>在你的 Tailscale 网络中发现 6 台设备，其中 5 台运行着 DeskBridge。</p></div>
      <div class="card">${DEVICES.filter(d => !d.self && d.group === "mine").slice(0, 4).map(d => `<div class="dev" style="margin:0;cursor:default"><div class="dic">${ic(kindIcon(d))}${dot(d)}</div><div class="main"><div class="n">${d.name}</div><div class="m">${d.os}</div></div></div>`).join("")}</div>
      <div class="banner info">${ic("plug")}<div class="t">要让 AI 使用这些设备，可以在“设置 → AI 接入”复制 MCP 配置。</div></div>
      <button class="btn primary lg block" data-a="onbDone">进入 DeskBridge</button>`;
  }
  const inner = `<div class="onb"><div class="card onb-card">${steps}${c}</div></div>`;
  return isPhone() ? `<div class="mshell">${inner}</div>` : inner;
}
A.onbRetry = () => { S.scenario = "normal"; S.onb.step = 1; A.onbNext(); };
