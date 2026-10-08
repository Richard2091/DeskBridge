// 示例数据：全部是虚构值（设备名、地址、账号），只用于原型展示。初版范围：Tailscale 和局域网两种连接方式。

const DEVICES = [
  {
    id: "my-laptop", name: "my-laptop", kind: "laptop", os: "Windows 11 专业版", self: true,
    online: true, via: "tailscale", path: "self", latency: 0, ip: "100.64.0.1", version: "0.3.0",
    note: "本机",
  },
  {
    id: "win-pc", name: "win-pc", kind: "monitor", os: "Windows 11 专业版",
    via: "tailscale", online: true, path: "lan", latency: 4, ip: "100.64.0.2", version: "0.3.0",
    session: "已登录 · 活跃", displays: 2, runMode: "用户模式（计划任务自启）",
    spark: [5, 4, 4, 6, 5, 4, 3, 4, 5, 4, 4, 3, 4, 5, 4, 4, 6, 4, 4, 4],
  },
  {
    id: "linux-server", name: "linux-server", kind: "server", os: "Ubuntu 24.04",
    via: "tailscale", online: true, path: "direct", latency: 38, ip: "100.64.0.3", version: "0.3.0",
    session: "无桌面", displays: 0, runMode: "systemd 服务",
    spark: [36, 40, 38, 37, 41, 39, 38, 36, 38, 44, 39, 38, 37, 38, 40, 38, 37, 39, 38, 38],
  },
  {
    id: "old-laptop", name: "old-laptop", kind: "laptop", os: "Windows 10 家庭版",
    via: "tailscale", online: true, path: "relay", relay: "东京", latency: 186, ip: "100.64.0.4", version: "0.2.4",
    session: "已登录 · 空闲 12 分钟", displays: 1, runMode: "用户模式（计划任务自启）", outdated: true,
    spark: [170, 190, 182, 210, 186, 175, 240, 188, 180, 186, 195, 178, 186, 220, 190, 184, 186, 181, 199, 186],
  },
  {
    id: "media-pc", name: "media-pc", kind: "monitor", os: "Windows 10 专业版",
    via: "tailscale", online: true, path: "lan", latency: 3, ip: "100.64.0.7", version: "0.3.0",
    session: "已登录 · 空闲 40 分钟", displays: 1, runMode: "用户模式（计划任务自启）",
    spark: [3, 3, 4, 3, 3, 5, 3, 3, 3, 4, 3, 3, 3, 3, 4, 3, 3, 3, 3, 3],
  },
  {
    id: "redmi-phone", name: "redmi-phone", kind: "phone", os: "Android 15",
    via: "tailscale", online: false, lastSeen: "2 小时前", path: "none", ip: "100.64.0.5", version: "0.3.0", controllerOnly: true,
  },
  {
    id: "nas", name: "nas", kind: "hdd", os: "Linux",
    via: "tailscale", online: true, noNode: true, path: "direct", latency: 6, ip: "100.64.0.6",
  },
  // 局域网设备：同一局域网中发现、但不在 Tailscale 里的 DeskBridge 设备。paired 表示是否已配对。
  {
    id: "living-room-pc", name: "living-room-pc", kind: "monitor", os: "Windows 11 家庭版",
    via: "lan", paired: true, online: true, path: "lan", latency: 2, ip: "192.168.1.31", version: "0.3.0",
    session: "已登录 · 空闲 5 分钟", displays: 1, runMode: "用户模式（计划任务自启）",
    spark: [2, 2, 3, 2, 2, 2, 3, 2, 2, 2, 2, 3, 2, 2, 2, 2, 2, 3, 2, 2],
  },
  {
    id: "study-pc", name: "study-pc", kind: "monitor", os: "Windows 10 专业版",
    via: "lan", paired: false, online: true, path: "lan", latency: 3, ip: "192.168.1.45", version: "0.3.0",
  },
];

const VIA_LABEL = { tailscale: "Tailscale", lan: "局域网" };
const PATH_LABEL = {
  self: "本机", lan: "局域网直连", direct: "跨网直连", relay: "中继", none: "未连接",
};

const ACTORS = {
  claude: { type: "ai", name: "Claude Code", from: "linux-server", av: "C", avc: "ai" },
  codex: { type: "ai", name: "Codex", from: "win-pc", av: "X", avc: "ai2" },
  me: { type: "human", name: "我", from: "my-laptop", av: "我", avc: "" },
  local: { type: "local", name: "本地用户", from: "win-pc", av: "本", avc: "" },
};

const ACTIVITY = [
  { day: "今天", t: "14:02", actor: "claude", dev: "win-pc", tool: "proc.exec", sum: "Get-ChildItem D:\\data -Recurse | Measure-Object", st: "run", du: "进行中",
    chain: ["claude", "win-pc:proc.exec"], args: 'shell: "pwsh"\ncommand: "Get-ChildItem D:\\\\data -Recurse | Measure-Object -Sum Length"\ntimeout: 120s', out: "已扫描 1,284 个文件…" },
  { day: "今天", t: "14:01", actor: "claude", dev: "win-pc", tool: "input.click", sum: "点击“保存”按钮（记事本）", st: "ok", du: "0.2 秒",
    chain: ["claude", "win-pc:input.click"], args: 'window: "无标题 - 记事本"\ntarget: ui://Button[name="保存"]\n控制权: AI 持有（租约 15 秒）', out: "成功" },
  { day: "今天", t: "14:01", actor: "claude", dev: "win-pc", tool: "screen.capture", sum: "截取显示器 1", st: "ok", du: "0.3 秒",
    chain: ["claude", "win-pc:screen.capture"], args: 'monitor: 1\nformat: "png"', out: "1920 × 1080 · 286 KB" },
  { day: "今天", t: "13:58", actor: "local", dev: "win-pc", tool: "control.preempt", sum: "本地用户移动鼠标，AI 的前台输入被中断", st: "pre", du: "—",
    chain: ["local"], args: "被抢占的请求: input.type（输入 42 个字符中的第 17 个）", out: "preempted，3 秒无操作后自动释放" },
  { day: "今天", t: "13:41", actor: "me", dev: "win-pc", tool: "fs.write", sum: "上传 设计草稿.pdf → D:\\data\\projects", st: "ok", du: "1.8 秒",
    chain: ["me", "win-pc:fs.write"], args: 'path: "D:\\\\data\\\\projects\\\\设计草稿.pdf"\nsize: 200 KB', out: "成功" },
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

// 连接方式：连接层的各个实现。Tailscale 和局域网是初版支持的方式，其余标“后续支持”。
const CONNECTORS = [
  { id: "tailscale", name: "Tailscale", icon: "globe", st: "on", acct: "user@example.com", ip: "100.64.0.1",
    d: "内置，不需要另装 Tailscale。同一账号下的设备自动发现、互相信任" },
  { id: "lan", name: "局域网", icon: "wifi", st: "on", ip: "192.168.1.20",
    d: "在同一局域网中发现运行着 DeskBridge 的设备，输入配对码后直接连接" },
  { id: "wireguard", name: "WireGuard 直连", icon: "link", st: "plan", d: "不依赖账号，两台设备配对后直接连接" },
  { id: "relay", name: "自建中继", icon: "route", st: "plan", d: "自己部署中继服务器，转发无法直连的流量" },
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
