@echo off
chcp 65001 >nul
title Reasonix Custom Desktop (8081)

echo ========================================
echo   Reasonix Custom Desktop
echo   Port: 8081 (no conflict with official)
echo ========================================
echo.
echo [1/2] 启动引擎...
cd /d "D:\deepseek\reasonix_1\src\DeepSeek-Reasonix"

start "Reasonix Custom Engine" /MIN reasonix-custom.exe serve --addr localhost:8081 --auth none

echo [2/2] 正在打开浏览器...
timeout /t 5 /nobreak >nul
start "" http://localhost:8081
echo.
echo 引擎运行在 http://localhost:8081
echo 提示：可用 launch-custom.vbs（无控制台窗口）
echo.
