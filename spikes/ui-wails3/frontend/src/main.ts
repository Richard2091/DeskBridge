import {Window} from "@wailsio/runtime";
import {DeskService} from "../bindings/deskbridge-spikes/ui-wails3";
import "./style.css";

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;

let base = "";
let framesBase = "";
let streamAbort: AbortController | null = null;

const CONN: Record<string, string> = {lan: "局域网直连", direct: "直连", offline: "离线"};

function connText(d: {conn: string; relay?: string}): string {
  if (d.conn === "relay") return `中继（${d.relay ?? "?"}）`;
  return CONN[d.conn] ?? d.conn;
}

async function loadDevices() {
  const res = await fetch(`${base}/api/devices`);
  const list = await res.json();
  const tbody = $("devices");
  tbody.innerHTML = "";
  for (const d of list) {
    const tr = document.createElement("tr");
    const cells = [
      d.name,
      d.os,
      d.online ? "在线" : "离线",
      connText(d),
      d.online ? `${d.latency_ms} ms` : "-",
    ];
    cells.forEach((text, i) => {
      const td = document.createElement("td");
      td.textContent = text;
      if (i === 2) td.className = d.online ? "on" : "off";
      tr.appendChild(td);
    });
    tbody.appendChild(tr);
  }
}

async function runExec() {
  const out = $("exec-out");
  out.textContent = "执行中…";
  try {
    const res = await fetch(`${base}/api/exec`, {
      method: "POST",
      headers: {"Content-Type": "application/json"},
      body: JSON.stringify({command: $<HTMLInputElement>("cmd").value}),
    });
    const r = await res.json();
    out.textContent = `退出码：${r.exit_code}（${r.ms} ms）\n标准输出：\n${r.stdout}\n标准错误：\n${r.stderr}`;
  } catch (e) {
    out.textContent = `出错：${e}`;
  }
}

function setTitle(fps: number) {
  const t = `DeskBridge 实测 fps=${fps.toFixed(1)}`;
  document.title = t;
  Window.SetTitle(t);
}

async function startStream() {
  if (streamAbort) return;
  const ctrl = new AbortController();
  streamAbort = ctrl;
  $<HTMLButtonElement>("btn-start").disabled = true;
  $<HTMLButtonElement>("btn-stop").disabled = false;

  const canvas = $<HTMLCanvasElement>("screen");
  const ctx = canvas.getContext("2d")!;
  let frames = 0;
  let last = performance.now();
  const timer = setInterval(() => {
    const now = performance.now();
    const fps = (frames * 1000) / (now - last);
    frames = 0;
    last = now;
    $("fps").textContent = fps.toFixed(1);
    setTitle(fps);
  }, 1000);

  try {
    const res = await fetch(`${framesBase}/api/frames?fps=10&width=1280&q=70`, {signal: ctrl.signal});
    const reader = res.body!.getReader();
    let buf = new Uint8Array(1 << 20);
    let len = 0;
    for (;;) {
      const {done, value} = await reader.read();
      if (done) break;
      if (len + value.length > buf.length) {
        const bigger = new Uint8Array(Math.max(buf.length * 2, len + value.length));
        bigger.set(buf.subarray(0, len));
        buf = bigger;
      }
      buf.set(value, len);
      len += value.length;
      let off = 0;
      while (len - off >= 4) {
        const n = new DataView(buf.buffer, off, 4).getUint32(0, false);
        if (len - off - 4 < n) break;
        const jpeg = buf.slice(off + 4, off + 4 + n);
        off += 4 + n;
        const bmp = await createImageBitmap(new Blob([jpeg], {type: "image/jpeg"}));
        if (canvas.width !== bmp.width || canvas.height !== bmp.height) {
          canvas.width = bmp.width;
          canvas.height = bmp.height;
        }
        ctx.drawImage(bmp, 0, 0);
        bmp.close();
        frames++;
      }
      if (off > 0) {
        buf.copyWithin(0, off, len);
        len -= off;
      }
    }
  } catch (e) {
    if ((e as Error).name !== "AbortError") console.error(e);
  } finally {
    clearInterval(timer);
    streamAbort = null;
    $<HTMLButtonElement>("btn-start").disabled = false;
    $<HTMLButtonElement>("btn-stop").disabled = true;
  }
}

function stopStream() {
  streamAbort?.abort();
}

async function login() {
  $("login").classList.add("hidden");
  $("home").classList.remove("hidden");
  await loadDevices();
}

async function init() {
  const cfg = await DeskService.Config();
  base = cfg.apiBase;
  framesBase = cfg.framesBase || cfg.apiBase;
  $("mode").textContent = `（接口通道：${cfg.mode}）`;
  $("binding").textContent = await DeskService.Ping("DeskBridge");

  $("btn-login").addEventListener("click", login);
  $("btn-exec").addEventListener("click", runExec);
  $("btn-start").addEventListener("click", startStream);
  $("btn-stop").addEventListener("click", stopStream);

  if (cfg.autoStream) {
    await login();
    await runExec();
    await startStream();
  }
}

init();