#!/usr/bin/env python3
"""在 Windows 上按档位批量运行 capenc，结果保存到本地 spikes/stream/out/（不提交）。

用法：
  AGENTDESK_MACHINE=<机器> STREAM_WIN_DIR=<Windows 上的 spikes\\stream 目录> \\
  python3 matrix.py --label A [--only 名称,...] [--seconds 20] [--warmup 5] [--verify]

依赖 agentdesk 命令（需支持 --poll-timeout）。每个测试先 run，抽查的几项再用 verify 检查码流和时间戳，
之后删除 Windows 上的码流文件。
"""
import argparse
import json
import os
import subprocess
import sys
import time

# 名称、画面、宽度（0 表示原始分辨率）、帧率、码率 kbps、编码器
TESTS = [
    ("ctl30-hw", "motion", 1920, 30, 4000, "hw"),
    ("ctl30-sw", "motion", 1920, 30, 4000, "sw"),
    ("hi60-hw", "motion", 1920, 60, 8000, "hw"),
    ("hi60-sw", "motion", 1920, 60, 8000, "sw"),
    ("native30-hw", "motion", 0, 30, 12000, "hw"),
    ("native30-sw", "motion", 0, 30, 12000, "sw"),
    ("native60-hw", "motion", 0, 60, 12000, "hw"),
    ("native60-sw", "motion", 0, 60, 12000, "sw"),
    ("text30-hw", "text", 1920, 30, 4000, "hw"),
    ("text30-sw", "text", 1920, 30, 4000, "sw"),
    ("static30-hw", "static", 1920, 30, 4000, "hw"),
    ("watch5-hw", "text", 0, 5, 4000, "hw"),
    ("watch5-sw", "text", 0, 5, 4000, "sw"),
    ("watch5-static-hw", "static", 0, 5, 4000, "hw"),
]


def win(cmd, cwd, timeout_s):
    """在 Windows 上执行命令，等待结束并把标准输出解析为 JSON。

    有的执行器不接受 cwd 参数，统一在命令里切换目录。请求级超时（--timeout-ms）和客户端等待时间
    （--poll-timeout）都要放宽，否则长命令会在默认的 30 秒处中断。
    """
    args = json.dumps({"command": f"cd /d {cwd} && {cmd}", "timeout_ms": timeout_s * 1000})
    r = subprocess.run(
        ["agentdesk", "call", "--tool", "windows.process.execute", "--args", args,
         "--timeout-ms", str(timeout_s * 1000 + 5000), "--poll-timeout", str(timeout_s + 30)],
        capture_output=True,
        text=True,
    )
    try:
        d = json.loads(r.stdout)
    except json.JSONDecodeError:
        return {"error": f"agentdesk 无法解析的输出：{r.stdout[-300:]} {r.stderr[-300:]}"}
    res = d.get("result") or {}
    out = res.get("stdout") or ""
    try:
        return json.loads(out)
    except json.JSONDecodeError:
        return {"error": out[-500:] + (res.get("stderr") or "") + json.dumps(d.get("error"), ensure_ascii=False)}


EXE = "rs\\target\\release\\capenc.exe"
# 默认只抽查这几项的码流和时间戳，--verify 时全部检查
VERIFY = {"ctl30-hw", "hi60-hw", "native60-hw"}


def row(name, s, v):
    if "error" in s:
        return f"{name:18} 错误：{s['error'][:200]}"
    f, lat, cpu = s["frames"], s["latency_ms"], s["cpu_pct_of_machine"]
    enc = s["gpu_self_pct"].get("VideoEncode", {}).get("avg", 0)
    p50 = lambda k: (lat[k] or {}).get("p50", "-")
    st = v.get("stamp", {})
    return (
        f"{name:18} {s['config']['out'][0]}x{s['config']['out'][1]} fps={f['out_fps']:6} "
        f"kbps={s['bitrate_kbps']['avg']:9} enc={p50('encoder_input_to_output'):6}ms "
        f"draw→enc={p50('draw_to_encoded'):6}ms cpu={cpu['avg']:5}% venc={enc:5}% "
        f"mem={s['private_mb']['peak']:7}MB 时间戳={st.get('match')}/{v.get('frames_decoded')}"
    )


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--label", required=True, help="设备代号，如 A、B")
    ap.add_argument("--only", default="")
    ap.add_argument("--seconds", type=int, default=20)
    ap.add_argument("--warmup", type=int, default=5)
    ap.add_argument("--verify", action="store_true", help="每项都检查码流和时间戳")
    a = ap.parse_args()
    cwd = os.environ.get("STREAM_WIN_DIR")
    if not cwd:
        sys.exit("需要设置 STREAM_WIN_DIR")
    out_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), "out")
    os.makedirs(out_dir, exist_ok=True)
    only = set(filter(None, a.only.split(",")))
    for name, scene, width, fps, kbps, enc in TESTS:
        if only and name not in only:
            continue
        base = f"out\\{a.label}-{name}"
        cmd = (
            f"run --pattern {scene} --fps {fps} --kbps {kbps} "
            f"--encoder {enc} --seconds {a.seconds} --warmup {a.warmup} --out {base}.h264 --stats {base}.jsonl"
        )
        if width:
            cmd += f" --width {width}"
        s = win(f"{EXE} {cmd}", cwd, a.seconds + a.warmup + 60)
        v = {}
        if "error" not in s and (a.verify or name in VERIFY):
            v = win(f"{EXE} verify --in {base}.h264 --stats {base}.jsonl", cwd, 110)
        win(f"del {base}.h264", cwd, 20)
        with open(os.path.join(out_dir, f"{a.label}-{name}.json"), "w", encoding="utf-8") as f:
            json.dump({"run": s, "verify": v, "time": time.strftime("%Y-%m-%d %H:%M:%S")}, f, ensure_ascii=False, indent=1)
        print(row(name, s, v), flush=True)


if __name__ == "__main__":
    main()
