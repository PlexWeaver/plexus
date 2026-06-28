#!/usr/bin/env python
# -*- coding: utf-8 -*-
# Reasonix Custom Desktop

import subprocess
import time
import urllib.request
import sys
import os
from pathlib import Path

BASE_DIR = Path(r"D:\deepseek\reasonix_1")
EXE_PATH = BASE_DIR / "src" / "DeepSeek-Reasonix" / "reasonix-custom.exe"
SERVER_URL = "http://localhost:8081"
WINDOW_TITLE = "Reasonix Custom"
BG_COLOR = "#1a1a2e"

process = None

def start_server():
    global process
    if not EXE_PATH.exists():
        print("[ERR] 引擎不存在:", EXE_PATH)
        return False
    process = subprocess.Popen(
        [str(EXE_PATH), "serve", "--addr", "localhost:8081", "--auth", "none"],
        cwd=str(EXE_PATH.parent),
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        creationflags=subprocess.CREATE_NO_WINDOW,
    )
    for _ in range(30):
        try:
            urllib.request.urlopen(SERVER_URL, timeout=1)
            print("[OK]", SERVER_URL)
            return True
        except:
            time.sleep(0.5)
    # Read engine output to show why it failed
    try:
        out = process.stdout.read(1024).decode("utf-8", errors="replace")
        if out.strip():
            print("[ERR] 引擎输出:", out.strip()[:300])
    except:
        pass
    return False

def stop_server():
    global process
    if process and process.poll() is None:
        process.terminate()
        try: process.wait(timeout=5)
        except: pass

class Api:
    def close(self):
        stop_server()

if __name__ == "__main__":
    try:
        import webview
    except ImportError:
        os.system("pip install pywebview")
        import webview

    print("[INFO] 启动引擎...")
    if not start_server():
        print("[ERR] 启动失败")
        stop_server()
        input("按回车退出...")
        sys.exit(1)
    print("[OK]", SERVER_URL)

    try:
        api = Api()
        window = webview.create_window(
            WINDOW_TITLE, SERVER_URL,
            width=1200, height=800,
            resizable=True, min_size=(800, 600),
            background_color=BG_COLOR,
        )
        webview.start(api.close, private_mode=False)
    except Exception as e:
        print("[ERR]", e)
        import webbrowser
        webbrowser.open(SERVER_URL)

    stop_server()
