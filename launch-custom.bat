@echo off
chcp 65001 >/dev/null
title Plexus Desktop (8081)
cd /d "D:\deepseek\reasonix_1\src\DeepSeek-Reasonix"
start "" http://localhost:8081
echo Engine starting at http://localhost:8081
echo Close this window to stop the engine.
echo.
reasonix-custom.exe serve --addr localhost:8081 --auth none
echo.
echo Engine stopped.
pause
