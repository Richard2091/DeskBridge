// 示例数据：全部是虚构值（设备名、地址、账号），只用于原型展示。

const DEVICES = [
  {
    id: "my-laptop", name: "my-laptop", kind: "laptop", os: "Windows 11 专业版", group: "mine", self: true,
    online: true, path: "self", latency: 0, ip: "100.64.0.1", version: "0.3.0",
    note: "本机",
  },
  {
    id: "win-pc", name: "win-pc", kind: "monitor", os: "Windows 11 专业版", group: "mine",
    online: true, path: "lan", latency: 4, ip: "100.64.0.2", version: "0.3.0",
    session: "已登录 · 活跃", displays: 2, runMode: "用户模式（计划任务自启）",
    merged: "这台设备上还装有 Tailscale 客户端，内网中的两个节点已合并显示",
    spark: [5, 4, 4, 6, 5, 4, 3, 4, 5, 4, 4, 3, 4, 5, 4, 4, 6, 4, 4, 4],
  },
  {
    id: "linux-server", name: "linux-server", kind: "server", os: "Ubuntu 24.04", group: "mine",
    online: true, path: "direct", latency: 38, ip: "100.64.0.3", version: "0.3.0",
    session: "无桌面", displays: 0, runMode: "systemd 服务",
    spark: [36, 40, 38, 37, 41, 39, 38, 36, 38, 44, 39, 38, 37, 38, 40, 38, 37, 39, 38, 38],
  },
  {
    id: "old-laptop", name: "old-laptop", kind: "laptop", os: "Windows 10 家庭版", group: "mine",
    online: true, path: "relay", relay: "东京", latency: 186, ip: "100.64.0.4", version: "0.2.4",
    session: "已登录 · 空闲 12 分钟", displays: 1, runMode: "用户模式（计划任务自启）", outdated: true,
    spark: [170, 190, 182, 210, 186, 175, 240, 188, 180, 186, 195, 178, 186, 220, 190, 184, 186, 181, 199, 186],
  },
  {
    id: "redmi-phone", name: "redmi-phone", kind: "phone", os: "Android 15", group: "mine",
    online: false, lastSeen: "2 小时前", path: "none", ip: "100.64.0.5", version: "0.3.0", controllerOnly: true,
  },
  {
    id: "nas", name: "nas", kind: "hdd", os: "Linux", group: "mine",
    online: true, noNode: true, path: "direct", latency: 6, ip: "100.64.0.6",
  },
  {
    id: "office-pc", name: "office-pc", kind: "monitor", os: "Windows 11", group: "external", owner: "alice",
    online: true, path: "direct", latency: 24, ip: "100.64.1.8", version: "0.3.0",
    session: "已登录 · 活跃", displays: 1, runMode: "用户模式", perms: ["命令", "读文件"],
    spark: [22, 25, 24, 23, 26, 24, 24, 25, 23, 24, 24, 22, 25, 24, 26, 24, 23, 24, 25, 24],
  },
];

const PATH_LABEL = {
  self: "本机", lan: "局域网直连", direct: "跨网直连", relay: "中继", none: "未连接",
};

const ACTORS = {
  claude: { type: "ai", name: "Claude Code", from: "linux-server", av: "C", avc: "ai" },
  codex: { type: "ai", name: "Codex", from: "win-pc", av: "X", avc: "ai2" },
  me: { type: "human", name: "示例用户", from: "my-laptop", av: "示", avc: "" },
  local: { type: "local", name: "本地用户", from: "win-pc", av: "本", avc: "" },
  bob: { type: "ext", name: "bob 的 Claude Code", from: "bob-pc", av: "B", avc: "ext" },
};

const ACTIVITY = [
  { day: "今天", t: "14:02", actor: "claude", dev: "win-pc", tool: "proc.exec", sum: "Get-ChildItem D:\\data -Recurse | Measure-Object", st: "run", du: "进行中",
    chain: ["claude", "win-pc:proc.exec"], args: 'shell: "pwsh"\ncommand: "Get-ChildItem D:\\\\data -Recurse | Measure-Object -Sum Length"\ntimeout: 120s', out: "已扫描 1,284 个文件…" },
  { day: "今天", t: "14:01", actor: "claude", dev: "win-pc", tool: "input.click", sum: "点击“保存”按钮（记事本）", st: "ok", du: "0.2 秒",
    chain: ["claude", "win-pc:input.click"], args: 'window: "无标题 - 记事本"\ntarget: ui://Button[name="保存"]\n控制权: AI 持有（租约 15 秒）', out: "成功" },
  { day: "今天", t: "13:58", actor: "local", dev: "win-pc", tool: "control.preempt", sum: "本地用户移动鼠标，AI 的前台输入被中断", st: "pre", du: "—",
    chain: ["local"], args: "被抢占的请求: input.type（输入 42 个字符中的第 17 个）", out: "preempted，3 秒无操作后自动释放" },
  { day: "今天", t: "13:41", actor: "me", dev: "win-pc", tool: "fs.write", sum: "上传 设计草稿.pdf → D:\\data\\projects", st: "ok", du: "1.8 秒",
    chain: ["me", "win-pc:fs.write"], args: 'path: "D:\\\\data\\\\projects\\\\设计草稿.pdf"\nsize: 200 KB', out: "成功" },
  { day: "今天", t: "13:20", actor: "claude", dev: "win-pc", tool: "agent.run", sum: "交给 Codex：编译并修复 deskbridge 的报错", st: "ok", du: "6 分 12 秒",
    chain: ["claude", "win-pc:agent.run(Codex)", "win-pc:proc.exec", "win-pc:mcp.playwright.navigate"], args: 'agent: "codex"\ncwd: "D:\\\\data\\\\projects\\\\deskbridge"\nprompt: "编译并修复报错"', out: "修改 3 个文件，编译通过" },
  { day: "今天", t: "11:05", actor: "bob", dev: "win-pc", tool: "screen.capture", sum: "截图（显示器 1）", st: "deny", du: "—",
    chain: ["bob", "win-pc:screen.capture"], args: "monitor: 1", out: "被拒绝：示例用户在审批中拒绝" },
  { day: "今天", t: "10:47", actor: "claude", dev: "old-laptop", tool: "proc.exec", sum: "winget upgrade --all", st: "err", du: "2 分 0 秒",
    chain: ["claude", "old-laptop:proc.exec"], args: 'command: "winget upgrade --all"\ntimeout: 120s', out: "超时（120 秒），已结束进程" },
  { day: "今天", t: "10:30", actor: "me", dev: "linux-server", tool: "proc.exec", sum: "systemctl status deskbridge", st: "ok", du: "0.1 秒",
    chain: ["me", "linux-server:proc.exec"], args: 'command: "systemctl status deskbridge"', out: "退出码 0" },
  { day: "昨天", t: "22:14", actor: "me", dev: "win-pc", tool: "proc.exec", sum: "Remove-Item -Recurse D:\\tmp\\cache", st: "cancel", du: "4.2 秒",
    chain: ["me", "win-pc:proc.exec"], args: 'command: "Remove-Item -Recurse D:\\\\tmp\\\\cache"', out: "已取消（用户）" },
  { day: "昨天", t: "21:03", actor: "claude", dev: "win-pc", tool: "ui.tree", sum: "读取“设置”窗口的界面元素", st: "ok", du: "0.6 秒",
    chain: ["claude", "win-pc:ui.tree"], args: 'window: "设置"\ndepth: 4', out: "126 个元素" },
  { day: "昨天", t: "20:55", actor: "codex", dev: "linux-server", tool: "fs.read", sum: "读取 /etc/nginx/nginx.conf", st: "ok", du: "0.1 秒",
    chain: ["codex", "linux-server:fs.read"], args: 'path: "/etc/nginx/nginx.conf"', out: "3.2 KB" },
];

const ST_LABEL = {
  ok: ["成功", "ok"], err: ["失败", "bad"], run: ["进行中", "info"], cancel: ["已取消", "off"],
  pre: ["被抢占", "warn"], deny: ["已拒绝", "bad"],
};

const APPROVALS = [
  {
    id: "a1", actor: "claude", dev: "win-pc", tool: "proc.exec", risk: "high",
    what: "Remove-Item -Recurse -Force D:\\data\\build\\*", why: "规则“删除文件需要审批”", wait: 272,
    reason: "AI 说明：清理旧的编译产物，然后重新编译。",
  },
  {
    id: "a2", actor: "bob", dev: "win-pc", tool: "screen.capture", risk: "mid",
    what: "截图（显示器 1）", why: "外部设备首次请求截图", wait: 845,
    reason: "bob 共享过来的 AI 请求查看屏幕。",
  },
];

const RULES = [
  { t: "删除文件需要审批", d: "fs.delete，以及包含 Remove-Item、rm -rf、del 的命令", on: true, scope: "所有设备" },
  { t: "外部设备的桌面操作需要审批", d: "截图、界面元素、鼠标键盘", on: true, scope: "外部访问" },
  { t: "AI 前台输入前先询问", d: "AI 模拟鼠标键盘前，被控端弹出确认", on: false, scope: "win-pc" },
  { t: "调用被控端的 Agent 需要审批", d: "agent.run，每个会话第一次调用时询问", on: true, scope: "所有设备" },
  { t: "注册表和系统设置修改需要审批", d: "reg add、Set-ItemProperty HKLM:\\ 等", on: true, scope: "Windows 设备" },
];

const SHARED_IN = [
  { dev: "office-pc", owner: "alice", perms: ["命令", "读文件"], since: "2026-09-12", exp: "长期", online: true },
];
const SHARED_OUT = [
  { dev: "win-pc", to: "bob", perms: ["命令", "读文件", "截图（需审批）"], since: "2026-10-01", exp: "2026-10-31", online: true },
];

const FILES = {
  "D:\\": [
    { n: "data", d: true, m: "2026/10/07 13:58" },
    { n: "tmp", d: true, m: "2026/10/06 22:14" },
    { n: "Program Files", d: true, m: "2026/09/30 08:12" },
  ],
  "D:\\data": [
    { n: "projects", d: true, m: "2026/10/07 13:58" },
    { n: "notes", d: true, m: "2026/10/05 09:12" },
    { n: "build", d: true, m: "2026/10/07 11:40" },
    { n: "backup.zip", s: "1.2 GB", m: "2026/09/28 03:00" },
  ],
  "D:\\data\\projects": [
    { n: "deskbridge", d: true, m: "2026/10/07 13:58" },
    { n: "readme.md", s: "18 KB", m: "2026/10/06 21:40", text: "# 项目笔记\n\n- DeskBridge 原型在 deskbridge 目录\n- 编译用 build.ps1\n- 设计稿见 设计草稿.pdf\n\n## 待办\n\n1. 补充界面设计\n2. 检查中文输出编码\n" },
    { n: "build.log", s: "246 KB", m: "2026/10/07 11:03", text: "[11:02:51] go build ./...\n[11:02:58] ok  deskbridge/cmd\n[11:03:02] cargo build --release\n[11:03:40]    Compiling desk-helper v0.1.0\n[11:03:44]     Finished release [optimized]\n[11:03:44] 编译完成，用时 53 秒\n" },
    { n: "设计草稿.pdf", s: "200 KB", m: "2026/10/07 13:41" },
  ],
};

const UI_WINDOWS = [
  { id: "np", n: "无标题 - 记事本", app: "notepad.exe" },
  { id: "ex", n: "文件资源管理器", app: "explorer.exe" },
  { id: "st", n: "设置", app: "SystemSettings.exe" },
  { id: "vs", n: "deskbridge - VS Code", app: "Code.exe", elevated: false },
  { id: "tm", n: "任务管理器", app: "Taskmgr.exe", elevated: true },
];

const UI_TREE = {
  id: "w", ty: "Window", nm: "无标题 - 记事本", open: true, kids: [
    { id: "tb", ty: "TitleBar", nm: "", kids: [
      { id: "min", ty: "Button", nm: "最小化" }, { id: "max", ty: "Button", nm: "最大化" }, { id: "cls", ty: "Button", nm: "关闭" },
    ] },
    { id: "mb", ty: "MenuBar", nm: "应用程序", open: true, kids: [
      { id: "mf", ty: "MenuItem", nm: "文件" }, { id: "me", ty: "MenuItem", nm: "编辑" }, { id: "mv", ty: "MenuItem", nm: "查看" },
    ] },
    { id: "doc", ty: "Document", nm: "文本编辑器" },
    { id: "sb", ty: "StatusBar", nm: "", kids: [
      { id: "s1", ty: "Text", nm: "第 1 行，第 1 列" }, { id: "s2", ty: "Text", nm: "UTF-8" },
    ] },
  ],
};

const UI_PROPS = {
  doc: { ty: "Document", nm: "文本编辑器", aid: "RichEditD2DPT", cls: "RichEditD2DPT", rect: "(312, 188) 960 × 540", pat: ["Text", "Value", "Scroll"], en: true },
  mf: { ty: "MenuItem", nm: "文件", aid: "File", cls: "MenuItem", rect: "(320, 150) 40 × 28", pat: ["Invoke", "ExpandCollapse"], en: true },
  cls: { ty: "Button", nm: "关闭", aid: "Close", cls: "Button", rect: "(1226, 112) 46 × 32", pat: ["Invoke"], en: true },
};

const AGENTS = [
  { n: "Claude Code", v: "2.4.1", st: "已登录", ok: true, av: "C", avc: "ai" },
  { n: "Codex", v: "0.48.0", st: "已登录", ok: true, av: "X", avc: "ai2" },
  { n: "Gemini CLI", v: "", st: "未安装", ok: false, av: "G", avc: "ext" },
];
const MCP_SERVERS = [
  { n: "playwright", tools: 21, st: "运行中", ok: true },
  { n: "sqlserver", tools: 8, st: "已停止", ok: false },
  { n: "office", tools: 14, st: "运行中", ok: true },
];
const SKILLS = ["build-windows", "release-notes", "excel-report"];

const CMD_OUTPUTS = {
  ping: [
    ["dim", "正在 Ping 100.64.0.3 具有 32 字节的数据:"],
    ["", "来自 100.64.0.3 的回复: 字节=32 时间=38ms TTL=64"],
    ["", "来自 100.64.0.3 的回复: 字节=32 时间=37ms TTL=64"],
    ["", "来自 100.64.0.3 的回复: 字节=32 时间=41ms TTL=64"],
    ["", "来自 100.64.0.3 的回复: 字节=32 时间=38ms TTL=64"],
    ["", ""],
    ["", "100.64.0.3 的 Ping 统计信息:"],
    ["", "    数据包: 已发送 = 4，已接收 = 4，丢失 = 0 (0% 丢失)"],
  ],
  default: [
    ["", ""],
    ["", "    目录: <hl>D:\\data\\projects</hl>"],
    ["", ""],
    ["", "Mode        LastWriteTime      Length Name"],
    ["", "----        -------------      ------ ----"],
    ["", "d----   2026/10/07  13:58             deskbridge"],
    ["", "d----   2026/10/05  09:12             notes"],
    ["", "-a---   2026/10/06  21:40      18342  readme.md"],
    ["", "-a---   2026/10/07  11:03     251904  build.log"],
    ["", "-a---   2026/10/07  13:41     204800  设计草稿.pdf"],
    ["err", "Get-ChildItem: 拒绝访问路径 'D:\\data\\private'。"],
    ["dim", "已扫描 1,284 个文件…"],
    ["", ""],
    ["", "Count    : 1284"],
    ["", "Sum      : 3418265904"],
  ],
};

const RECENT_RUNS = [
  { c: "systemctl status deskbridge", st: "ok", code: 0, w: "10:30", dev: "linux-server" },
  { c: "Get-Process | Sort-Object CPU -Desc | Select -First 5", st: "ok", code: 0, w: "09:12" },
  { c: "Remove-Item -Recurse D:\\tmp\\cache", st: "cancel", code: null, w: "昨天" },
  { c: "git -C D:\\data\\projects\\deskbridge pull --ff-only", st: "err", code: 1, w: "昨天" },
];
