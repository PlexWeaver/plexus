# Reasonix Custom Desktop Launcher
# Close this window = stop engine.

$enginePath = "D:\deepseek\reasonix_1\src\DeepSeek-Reasonix\reasonix-custom.exe"
$url = "http://localhost:8081"

if (-not (Test-Path $enginePath)) {
    Write-Host "[ERR] Engine not found: $enginePath" -ForegroundColor Red
    Read-Host "Press Enter"
    exit 1
}

$old = netstat -ano | findstr ":8081 " | findstr LISTENING
if ($old) {
    $oldPid = ($old -split '\s+')[-1]
    if ($oldPid -match '^\d+$') { Stop-Process -Id $oldPid -Force -ErrorAction SilentlyContinue }
}

Write-Host "[1/3] Starting engine..." -ForegroundColor Cyan
$engineDir = Split-Path $enginePath -Parent
$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = $enginePath
$psi.Arguments = "serve --addr localhost:8081 --auth none"
$psi.WorkingDirectory = $engineDir
$psi.UseShellExecute = $false
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true
$psi.CreateNoWindow = $true
$engine = [System.Diagnostics.Process]::Start($psi)

Write-Host "[2/3] Waiting..." -ForegroundColor Cyan
$setupPage = $false
$ready = $false
for ($i = 0; $i -lt 50; $i++) {
    Start-Sleep -Milliseconds 300
    try {
        $req = [System.Net.WebRequest]::Create($url)
        $req.Timeout = 1000
        $resp = $req.GetResponse()
        $reader = New-Object System.IO.StreamReader($resp.GetResponseStream())
        $buf = New-Object char[] 200
        $reader.Read($buf, 0, 200) | Out-Null
        $reader.Close()
        $resp.Close()
        $ready = $true
        $prefix = -join $buf
        if ($prefix -match "API Key Required|未配置 API") {
            $setupPage = $true
        }
        break
    } catch {
    }
}

if (-not $ready) {
    Write-Host "[ERR] Engine failed to start on $url" -ForegroundColor Red
    $engine.Kill()
    Read-Host "Press Enter"
    exit 1
}

if ($setupPage) {
    Write-Host "[!] DEEPSEEK_API_KEY not set" -ForegroundColor Yellow
} else {
    Write-Host "[OK] $url" -ForegroundColor Green
}

Write-Host "[3/3] Opening browser..." -ForegroundColor Cyan
Start-Process $url
Write-Host ""
Write-Host "Engine running at $url" -ForegroundColor White
Write-Host "Close this window to stop the engine." -ForegroundColor Gray
Write-Host ""

try { $engine.WaitForExit() } catch {}
if (-not $engine.HasExited) { $engine.Kill() }
