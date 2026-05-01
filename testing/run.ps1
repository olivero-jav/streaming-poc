# Uso: .\testing\run.ps1 <script>
# Ejemplos:
#   .\testing\run.ps1 api-smoke
#   .\testing\run.ps1 hls-vod
#   .\testing\run.ps1 hls-live   <- crea stream + publisher RTMP automaticamente

param(
    [Parameter(Mandatory)][ValidateSet('api-smoke','hls-vod','hls-live')]
    [string]$Script
)

$envFile = "$PSScriptRoot\.env"
if (-not (Test-Path $envFile)) {
    Write-Error "No se encontro $envFile. Copia .env.example a .env y completa los valores."
    exit 1
}

# Cargar variables del .env
Get-Content $envFile | ForEach-Object {
    if ($_ -match '^\s*([^#][^=]+)=(.*)$') {
        $key = $Matches[1].Trim()
        $val = $Matches[2].Trim()
        [System.Environment]::SetEnvironmentVariable($key, $val, 'Process')
    }
}

# Validaciones
if ($Script -eq 'hls-vod' -and -not $env:VIDEO_ID) {
    Write-Error "VIDEO_ID no esta definido en .env"
    exit 1
}

$scriptPath = "$PSScriptRoot\$Script.js"
New-Item -ItemType Directory -Force -Path "$PSScriptRoot\reports" | Out-Null
if (-not $env:REPORT_DIR) {
    $env:REPORT_DIR = (Join-Path $PSScriptRoot 'reports').Replace('\', '/')
}

# Contexto de la corrida (queda en setup_data del JSON de cada reporte)
$env:GIT_COMMIT = ''
$gitOut = & git -C "$PSScriptRoot\.." rev-parse --short HEAD 2>$null
if ($LASTEXITCODE -eq 0 -and $gitOut) {
    $env:GIT_COMMIT = $gitOut.Trim()
}
$env:HOSTNAME = $env:COMPUTERNAME

# ---------------------------------------------------------------------------
# hls-live: crea stream + publisher RTMP con FFmpeg, espera live, corre k6
# ---------------------------------------------------------------------------
if ($Script -eq 'hls-live') {

    # 1. Crear stream via API
    Write-Host "Creando stream de prueba..." -ForegroundColor Yellow
    $body = "{`"title`":`"k6-live-$(Get-Date -Format 'HHmm')`"}"
    try {
        $stream = Invoke-RestMethod -Uri "$env:BASE_URL/streams" -Method POST -Body $body -ContentType 'application/json' -ErrorAction Stop
    } catch {
        Write-Error "No se pudo crear el stream en $env:BASE_URL/streams : $_"
        exit 1
    }
    $streamId  = $stream.id
    $streamKey = $stream.stream_key
    Write-Host "Stream creado: $streamId  key: $streamKey" -ForegroundColor Green

    # 2. Lanzar FFmpeg como publisher RTMP con senal de prueba
    $rtmpUrl = "rtmp://localhost:1935/live/$streamKey"
    $ffmpegArgs = @(
        '-re',
        '-f', 'lavfi', '-i', 'testsrc2=size=1280x720:rate=30',
        '-f', 'lavfi', '-i', 'sine=frequency=440:sample_rate=44100',
        '-c:v', 'libx264', '-preset', 'ultrafast', '-b:v', '1500k',
        '-c:a', 'aac', '-b:a', '128k',
        '-f', 'flv', $rtmpUrl
    )
    $ffProc = Start-Process ffmpeg -ArgumentList $ffmpegArgs -PassThru -WindowStyle Minimized
    Write-Host "FFmpeg publisher iniciado (PID $($ffProc.Id)) -> $rtmpUrl" -ForegroundColor Yellow

    # 3. Esperar a que el stream sea live (max 30s)
    Write-Host "Esperando que el stream pase a live..." -ForegroundColor Yellow
    $deadline = (Get-Date).AddSeconds(30)
    $isLive = $false
    while ((Get-Date) -lt $deadline) {
        Start-Sleep -Seconds 2
        try {
            $s = Invoke-RestMethod -Uri "$env:BASE_URL/streams/$streamId" -ErrorAction Stop
            if ($s.status -eq 'live') { $isLive = $true; break }
        } catch {}
    }

    if (-not $isLive) {
        Stop-Process -Id $ffProc.Id -Force -ErrorAction SilentlyContinue
        Write-Error "El stream no llego a estado live en 30s. Verifica que MediaMTX este corriendo."
        exit 1
    }

    Write-Host "Stream live. Iniciando k6 (VUS=$env:VUS, DURATION=$env:DURATION)..." -ForegroundColor Cyan
    $env:STREAM_ID = $streamId

    # 4. Correr k6; FFmpeg se detiene pase lo que pase
    try {
        k6 run $scriptPath
    } finally {
        Write-Host "Deteniendo FFmpeg publisher (PID $($ffProc.Id))..." -ForegroundColor Yellow
        Stop-Process -Id $ffProc.Id -Force -ErrorAction SilentlyContinue
        Write-Host "Listo." -ForegroundColor Green
    }
    exit
}

# ---------------------------------------------------------------------------
# api-smoke / hls-vod
# ---------------------------------------------------------------------------
Write-Host "Corriendo $Script (VUS=$env:VUS, DURATION=$env:DURATION, BASE_URL=$env:BASE_URL)" -ForegroundColor Cyan
k6 run $scriptPath
