$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$mysqlBin = "C:\Program Files\MySQL\MySQL Server 8.4\bin"
$mysqlBase = "C:\Program Files\MySQL\MySQL Server 8.4"
$mysqlData = "C:\ProgramData\MySQL\MySQL Server 8.4\Data"
$memuraiDir = Join-Path $projectRoot ".local\memurai\Memurai"
$memuraiConfig = Join-Path $projectRoot ".local\memurai\memurai.conf"

if (-not (Test-Path (Join-Path $mysqlBin "mysqld.exe"))) {
    throw "MySQL 8.4 was not found."
}
if (-not (Test-Path (Join-Path $memuraiDir "memurai.exe"))) {
    throw "Redis/Memurai was not found under .local\memurai."
}

if (-not (Get-NetTCPConnection -LocalPort 3306 -State Listen -ErrorAction SilentlyContinue)) {
    $mysqlArgs = "--basedir=`"$mysqlBase`" --datadir=`"$mysqlData`" --port=3306 --bind-address=127.0.0.1"
    Start-Process -FilePath (Join-Path $mysqlBin "mysqld.exe") -ArgumentList $mysqlArgs -WindowStyle Hidden
}

if (-not (Get-NetTCPConnection -LocalPort 6379 -State Listen -ErrorAction SilentlyContinue)) {
    Start-Process -FilePath (Join-Path $memuraiDir "memurai.exe") -ArgumentList "`"$memuraiConfig`"" -WindowStyle Hidden
}

$ready = $false
for ($i = 0; $i -lt 20; $i++) {
    $mysqlReady = [bool](Get-NetTCPConnection -LocalPort 3306 -State Listen -ErrorAction SilentlyContinue)
    $redisReady = [bool](Get-NetTCPConnection -LocalPort 6379 -State Listen -ErrorAction SilentlyContinue)
    if ($mysqlReady -and $redisReady) {
        $ready = $true
        break
    }
    Start-Sleep -Milliseconds 500
}
if (-not $ready) {
    throw "MySQL or Redis did not become ready on ports 3306/6379."
}

$env:MYSQL_DSN = "dianping:dianping@tcp(127.0.0.1:3306)/dianping?charset=utf8mb4&parseTime=True&loc=Local"
$env:REDIS_ADDR = "127.0.0.1:6379"
$env:REDIS_PASSWORD = ""

Set-Location $projectRoot
go run .\cmd\service
