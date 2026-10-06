import { getCurrentWindow } from "@tauri-apps/api/window";
import { invoke } from "@tauri-apps/api/core";

const CORE = "http://127.0.0.1:47800";
const app = document.getElementById("app") as HTMLElement;

interface Device {
  name: string;
  os: string;
  online: boolean;
  conn: string;
  relay?: string;
  latency_ms?: number;
}

const connText = (d: Device): string => {
  switch (d.conn) {
    case "lan": return "局域网直连";
    case "direct": return "直连";
    case "relay": return `中继（${d.relay ?? "?"}）`;
    default: return "离线";
  }
};

async function setTitle(t: string) {
  document.title = t;
  try {
    await getCurrentWindow().setTitle(t);
  } catch (e) {
    console.error("setTitle 失败", e);
  }
}

function showLogin() {
  app.innerHTML = `
    <div class="login">
      <h1>DeskBridge 登录</h1>
      <input id="user" placeholder="用户名" value="demo" />
      <input id="pass" type="password" placeholder="密码" value="demo" />
      <button id="login">登录</button>
    </div>`;
  document.getElementById("login")!.addEventListener("click", showMain);
}

async function waitCore(): Promise<Response> {
  let last: unknown;
  for (let i = 0; i < 50; i++) {
    try {
      return await fetch(`${CORE}/api/devices`);
    } catch (e) {
      last = e;
      await new Promise((r) => setTimeout(r, 200));
    }
  }
  throw last;
}

async function showMain() {
  app.innerHTML = `
    <div class="main">
      <section>
        <h2>设备列表</h2>
        <table><thead><tr><th>名称</th><th>系统</th><th>在线</th><th>连接方式</th><th>延迟</th></tr></thead>
        <tbody id="devices"><tr><td colspan="5">加载中…</td></tr></tbody></table>
      </section>
      <section>
        <h2>执行命令</h2>
        <div class="row"><input id="cmd" value="Write-Output 中文测试" /><button id="run">执行</button></div>
        <pre id="out"></pre>
      </section>
      <section>
        <h2>远程画面</h2>
        <div class="row">
          <button id="start">开始</button><button id="stop">停止</button>
          <span>实际帧率：<b id="fps">0.0</b></span>
        </div>
        <canvas id="cv" width="1280" height="720"></canvas>
      </section>
    </div>`;

  const tbody = document.getElementById("devices")!;
  try {
    const res = await waitCore();
    const devs: Device[] = await res.json();
    tbody.innerHTML = devs
      .map(
        (d) =>
          `<tr><td>${d.name}</td><td>${d.os}</td><td class="${d.online ? "on" : "off"}">${d.online ? "在线" : "离线"}</td><td>${connText(d)}</td><td>${d.online && d.latency_ms != null ? d.latency_ms + " ms" : "-"}</td></tr>`,
      )
      .join("");
  } catch (e) {
    tbody.innerHTML = `<tr><td colspan="5">无法连接核心：${e}</td></tr>`;
  }

  const cmd = document.getElementById("cmd") as HTMLInputElement;
  const out = document.getElementById("out")!;
  document.getElementById("run")!.addEventListener("click", async () => {
    out.textContent = "执行中…";
    try {
      const r = await fetch(`${CORE}/api/exec`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: cmd.value }),
      });
      const j = await r.json();
      out.textContent = `退出码: ${j.exit_code}\n--- stdout ---\n${j.stdout}\n--- stderr ---\n${j.stderr}\n(${j.ms} ms)`;
    } catch (e) {
      out.textContent = `请求失败：${e}`;
    }
  });

  setupStream();
}

function setupStream() {
  const cv = document.getElementById("cv") as HTMLCanvasElement;
  const ctx = cv.getContext("2d")!;
  const fpsEl = document.getElementById("fps")!;
  let ctrl: AbortController | null = null;

  async function start() {
    if (ctrl) return;
    ctrl = new AbortController();
    const my = ctrl;
    let frames = 0;
    let t0 = performance.now();
    try {
      const res = await fetch(`${CORE}/api/frames?fps=10&width=1280&q=70`, { signal: my.signal });
      const reader = res.body!.getReader();
      let buf = new Uint8Array(0);
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        const nb = new Uint8Array(buf.length + value.length);
        nb.set(buf);
        nb.set(value, buf.length);
        buf = nb;
        while (buf.length >= 4) {
          const len = new DataView(buf.buffer, buf.byteOffset).getUint32(0, false);
          if (buf.length < 4 + len) break;
          const jpeg = buf.slice(4, 4 + len);
          buf = buf.slice(4 + len);
          const bmp = await createImageBitmap(new Blob([jpeg], { type: "image/jpeg" }));
          if (cv.width !== bmp.width || cv.height !== bmp.height) {
            cv.width = bmp.width;
            cv.height = bmp.height;
          }
          ctx.drawImage(bmp, 0, 0);
          bmp.close();
          frames++;
          const now = performance.now();
          if (now - t0 >= 1000) {
            const fps = (frames * 1000) / (now - t0);
            fpsEl.textContent = fps.toFixed(1);
            setTitle(`DeskBridge 实测 fps=${fps.toFixed(1)}`);
            frames = 0;
            t0 = now;
          }
        }
      }
    } catch (e) {
      if ((e as Error).name !== "AbortError") console.error(e);
    } finally {
      if (ctrl === my) ctrl = null;
    }
  }

  document.getElementById("start")!.addEventListener("click", start);
  document.getElementById("stop")!.addEventListener("click", () => {
    ctrl?.abort();
    ctrl = null;
  });
  (window as unknown as { __startStream: () => void }).__startStream = start;
}

showLogin();
setTitle("DeskBridge 实测");

// 无人测量：环境变量 DESKBRIDGE_AUTOSTREAM=1 时由 Rust 端告知
invoke<boolean>("autostream").then(async (on) => {
  if (on) {
    await showMain();
    (window as unknown as { __startStream: () => void }).__startStream();
  }
});
