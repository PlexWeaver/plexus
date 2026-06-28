# Reasonix 环境回退脚本
# 用法: PowerShell -ExecutionPolicy Bypass .\rollback.ps1

param(
    [string]$BackupDir = "",
    [switch]$Force = $false
)

$ErrorActionPreference = "Stop"

$reasonixDir = "D:\deepseek\Reasonix"
$configPath  = "C:\Users\19922\AppData\Roaming\reasonix\config.toml"
$workDir     = "D:\deepseek\work"

function Color { param($c,$t) Write-Host $t -ForegroundColor $c }

if (-not $BackupDir) {
    $bk = Get-ChildItem "$reasonixDir\.backup-*" -Directory | Sort-Object Name -Descending
    if (-not $bk) { Color Red "[ERR] 未找到备份目录"; exit 1 }
    $BackupDir = $bk[0].FullName
}

if (-not (Test-Path $BackupDir)) { Color Red "[ERR] 备份目录不存在"; exit 1 }

Color Cyan "=== Reasonix 环境回退 ==="
Color Gray "备份: $BackupDir"

$missing = @()
if (-not (Test-Path "$BackupDir\config.toml"))          { $missing += "config.toml" }
if (-not (Test-Path "$BackupDir\reasonix-desktop.sha256")) { $missing += "reasonix-desktop.sha256" }
if ($missing.Count -gt 0) { Color Red "[ERR] 备份不完整: $($missing -join ', ')"; exit 1 }

Color Yellow "待恢复:"
Get-ChildItem $BackupDir | Where-Object { -not $_.PSIsContainer } | ForEach-Object {
    $n = $_.Name; $s = $_.Length
    Color Gray "  $n ($s bytes)"
}

if (-not $Force) {
    $r = Read-Host "继续? (y/N)"
    if ($r -ne "y" -and $r -ne "Y") { Color Cyan "已取消"; exit 0 }
}

$errs = @()

try { Copy-Item "$BackupDir\config.toml" $configPath -Force; Color Green "[OK] config.toml" }
catch { Color Red "[ERR] config.toml: $_"; $errs += "config.toml" }

if (Test-Path "$BackupDir\交接.md") {
    try {
        Copy-Item "$BackupDir\交接.md" "$workDir\交接.md" -Force
        if (Test-Path "$reasonixDir\交接.md") { Copy-Item "$BackupDir\交接.md" "$reasonixDir\交接.md" -Force }
        Color Green "[OK] 交接.md"
    } catch { Color Red "[ERR] 交接.md: $_"; $errs += "交接.md" }
}

$origHash = (Get-Content "$BackupDir\reasonix-desktop.sha256" -Raw).Trim()
$currHash = (Get-FileHash "$reasonixDir\reasonix-desktop.exe" -Algorithm SHA256).Hash
if ($currHash -eq $origHash) { Color Green "[OK] desktop 未修改" }
else { Color Yellow "[WARN] desktop 已修改!" }

if ($errs.Count -eq 0) { Color Green "[OK] 全部恢复成功！请重启 Reasonix Desktop" }
else { Color Red "[ERR] 失败: $($errs -join ', ')" }
