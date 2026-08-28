#Requires -Version 5.1
<#
.SYNOPSIS
  ocrshow 一键安装：Python 虚拟环境、PaddleOCR、Go 依赖、前端 npm。
.PARAMETER Cpu
  不装 GPU 版 PaddlePaddle，改用 CPU。
.PARAMETER SkipWeb
  只装 Python/OCR，不装 Go / Node 前端。
.PARAMETER WithVL
  额外拉取 Ollama 模型 qwen3-vl:8b（需本机已装 Ollama）。
.PARAMETER SkipWinget
  缺 Go/Node 时不自动用 winget 安装。
#>
param(
    [switch]$Cpu,
    [switch]$SkipWeb,
    [switch]$WithVL,
    [switch]$SkipWinget
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$PyIndex = "https://pypi.tuna.tsinghua.edu.cn/simple"
$PaddleGpuIndex = "https://www.paddlepaddle.org.cn/packages/stable/cu126/"
$PaddleCpuIndex = "https://www.paddlepaddle.org.cn/packages/stable/cpu/"
$PaddleVer = "3.2.2"
$NpmRegistry = "https://registry.npmmirror.com"
$GoProxy = "https://goproxy.cn,direct"

$script:Failed = @()
$script:Warnings = @()

function Write-Step([string]$msg) {
    Write-Host ""
    Write-Host "==> $msg" -ForegroundColor Cyan
}

function Write-Ok([string]$msg) {
    Write-Host "    OK  $msg" -ForegroundColor Green
}

function Write-Warn([string]$msg) {
    Write-Host "    WARN  $msg" -ForegroundColor Yellow
    $script:Warnings += $msg
}

function Write-Fail([string]$msg) {
    Write-Host "    FAIL  $msg" -ForegroundColor Red
    $script:Failed += $msg
}

function Refresh-Path {
    $machine = [System.Environment]::GetEnvironmentVariable("Path", "Machine")
    $user = [System.Environment]::GetEnvironmentVariable("Path", "User")
    $env:Path = "$machine;$user"
    $uvBin = Join-Path $env:USERPROFILE ".local\bin"
    if (Test-Path $uvBin) {
        $env:Path = "$uvBin;$env:Path"
    }
}

function Test-Cmd([string]$name) {
    return [bool](Get-Command $name -ErrorAction SilentlyContinue)
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)]
        [scriptblock]$Command,
        [string]$FailMessage
    )
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw $FailMessage
    }
}

function Install-WithWinget {
    param(
        [string]$Id,
        [string]$DisplayName
    )
    if ($SkipWinget) { return $false }
    if (-not (Test-Cmd "winget")) { return $false }
    Write-Host "    正在用 winget 安装 $DisplayName ..."
    winget install --id $Id -e --accept-package-agreements --accept-source-agreements
    Refresh-Path
    return $true
}

# ---------------------------------------------------------------------------
Write-Host "ocrshow 环境安装" -ForegroundColor Cyan
Write-Host "目录: $PSScriptRoot"

Refresh-Path

# ---- uv + Python 3.11 ------------------------------------------------------
Write-Step "Python 3.11 虚拟环境"

if (-not (Test-Cmd "uv")) {
    Write-Host "    未找到 uv，正在安装..."
    try {
        irm https://astral.sh/uv/install.ps1 | iex
        Refresh-Path
    } catch {
        Write-Fail "安装 uv 失败: $_"
    }
}

if (-not (Test-Cmd "uv")) {
    Write-Fail "仍找不到 uv。请打开新终端后重跑，或手动安装: https://docs.astral.sh/uv/"
} else {
    Write-Ok "uv $(uv --version)"
    try {
        Write-Host "    确保 Python 3.11 可用..."
        uv python install 3.11
        if (-not (Test-Path ".\.venv")) {
            uv venv .venv --python 3.11
        } else {
            Write-Host "    已有 .venv，跳过创建"
        }
        Write-Ok ".venv 就绪"
    } catch {
        Write-Fail "创建虚拟环境失败: $_"
    }
}

# ---- PaddlePaddle + OCR ----------------------------------------------------
$venvPython = Join-Path $PSScriptRoot ".venv\Scripts\python.exe"
$venvUv = $null
if (Test-Cmd "uv") { $venvUv = "uv" }

if ((Test-Path $venvPython) -and $venvUv) {
    Write-Step "PaddlePaddle / PaddleOCR"

    $useGpu = -not $Cpu
    if ($useGpu) {
        $gpuOk = $false
        if (Test-Cmd "nvidia-smi") {
            nvidia-smi -L | Out-Host
            if ($LASTEXITCODE -eq 0) { $gpuOk = $true }
        }
        if (-not $gpuOk) {
            Write-Warn "未检测到 NVIDIA GPU / nvidia-smi，改装 CPU 版 PaddlePaddle"
            $useGpu = $false
        }
    }

    try {
        if ($useGpu) {
            Write-Host "    安装 paddlepaddle-gpu==$PaddleVer (CUDA 12.6 wheel)..."
            uv pip install "paddlepaddle-gpu==$PaddleVer" -i $PaddleGpuIndex --python $venvPython
            if ($LASTEXITCODE -ne 0) { throw "paddlepaddle-gpu 安装失败" }
        } else {
            Write-Host "    安装 paddlepaddle==$PaddleVer (CPU)..."
            uv pip install "paddlepaddle==$PaddleVer" -i $PaddleCpuIndex --python $venvPython
            if ($LASTEXITCODE -ne 0) { throw "paddlepaddle 安装失败" }
        }

        Write-Host "    安装 paddleocr / openpyxl / requests ..."
        uv pip install "paddleocr[doc-parser]" openpyxl requests openai pillow -i $PyIndex --python $venvPython
        if ($LASTEXITCODE -ne 0) { throw "PaddleOCR 依赖安装失败" }

        # paddleocr 可能把 GPU 包换成 PyPI 上的 CPU 包，再钉一次
        if ($useGpu) {
            uv pip install "paddlepaddle-gpu==$PaddleVer" -i $PaddleGpuIndex --python $venvPython
            if ($LASTEXITCODE -ne 0) { throw "重新固定 paddlepaddle-gpu 失败" }
        }

        Write-Host "    校验 import paddle ..."
        & $venvPython -c "import paddle, paddleocr; print('paddle', paddle.__version__, 'cuda', paddle.is_compiled_with_cuda())"
        if ($LASTEXITCODE -ne 0) { throw "import paddle / paddleocr 失败" }
        Write-Ok "Python OCR 依赖已安装"
    } catch {
        Write-Fail $_
    }
}

# ---- Go --------------------------------------------------------------------
if (-not $SkipWeb) {
    Write-Step "Go 后端依赖"

    if (-not (Test-Cmd "go")) {
        $installed = Install-WithWinget -Id "GoLang.Go" -DisplayName "Go"
        Refresh-Path
        if (-not (Test-Cmd "go") -and (Test-Path "C:\Program Files\Go\bin\go.exe")) {
            $env:Path = "C:\Program Files\Go\bin;$env:Path"
        }
        if (-not $installed -and -not (Test-Cmd "go")) {
            Write-Warn "未找到 Go。请安装 Go 1.21+ 后重跑: https://go.dev/dl/"
        }
    }

    if (Test-Cmd "go") {
        Write-Ok "$(go version)"
        try {
            $env:GOPROXY = $GoProxy
            Push-Location (Join-Path $PSScriptRoot "backend")
            go mod tidy
            if ($LASTEXITCODE -ne 0) { throw "go mod tidy 失败" }
            Write-Ok "go mod tidy 完成"
        } catch {
            Write-Fail $_
        } finally {
            Pop-Location
        }
    } else {
        Write-Fail "跳过 Go 依赖（未安装 Go）"
    }
}

# ---- Node / 前端 ------------------------------------------------------------
if (-not $SkipWeb) {
    Write-Step "前端 npm 依赖"

    if (-not (Test-Cmd "npm")) {
        $installed = Install-WithWinget -Id "OpenJS.NodeJS.LTS" -DisplayName "Node.js LTS"
        Refresh-Path
        if (-not (Test-Cmd "npm") -and (Test-Path "C:\Program Files\nodejs\npm.cmd")) {
            $env:Path = "C:\Program Files\nodejs;$env:Path"
        }
        if (-not $installed -and -not (Test-Cmd "npm")) {
            Write-Warn "未找到 Node.js。请安装 Node.js 18+ 后重跑: https://nodejs.org/"
        }
    }

    if (Test-Cmd "npm") {
        Write-Ok "node $(node -v) / npm $(npm -v)"
        try {
            Push-Location (Join-Path $PSScriptRoot "frontend")
            npm install --registry $NpmRegistry
            if ($LASTEXITCODE -ne 0) { throw "npm install 失败" }
            Write-Ok "frontend/node_modules 就绪"
        } catch {
            Write-Fail $_
        } finally {
            Pop-Location
        }
    } else {
        Write-Fail "跳过前端依赖（未安装 Node.js）"
    }
}

# ---- 可选 VL ----------------------------------------------------------------
if ($WithVL) {
    Write-Step "Ollama Qwen3-VL"
    if (Test-Cmd "ollama") {
        try {
            ollama pull qwen3-vl:8b
            if ($LASTEXITCODE -ne 0) { throw "ollama pull 失败" }
            Write-Ok "已拉取 qwen3-vl:8b"
        } catch {
            Write-Fail $_
        }
    } else {
        Write-Warn "未找到 ollama。请先安装 https://ollama.com/ 再执行: ollama pull qwen3-vl:8b"
    }
} else {
    Write-Host ""
    Write-Host "    提示: 需要 Qwen3-VL 纠错时，安装 Ollama 后执行  ollama pull qwen3-vl:8b" -ForegroundColor DarkGray
    Write-Host "          或重跑  .\setup.ps1 -WithVL" -ForegroundColor DarkGray
}

# ---- 本机配置（不入库） ----------------------------------------------------
if (-not (Test-Path (Join-Path $PSScriptRoot "config.toml")) -and (Test-Path (Join-Path $PSScriptRoot "config.example.toml"))) {
    Copy-Item (Join-Path $PSScriptRoot "config.example.toml") (Join-Path $PSScriptRoot "config.toml")
    Write-Ok "已生成 config.toml（不会提交到 git）"
}
if (-not (Test-Path (Join-Path $PSScriptRoot ".env")) -and (Test-Path (Join-Path $PSScriptRoot ".env.example"))) {
    Copy-Item (Join-Path $PSScriptRoot ".env.example") (Join-Path $PSScriptRoot ".env")
    Write-Ok "已生成 .env（密钥写这里，不要提交）"
}
$samples = Join-Path $PSScriptRoot "samples"
if (-not (Test-Path $samples)) {
    New-Item -ItemType Directory -Path $samples | Out-Null
}

# ---- 总结 -------------------------------------------------------------------
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
if ($script:Failed.Count -eq 0) {
    Write-Host "安装完成" -ForegroundColor Green
    Write-Host ""
    Write-Host "命令行 OCR:"
    Write-Host "  把截图放到 samples\ ，然后"
    Write-Host "  .\.venv\Scripts\python.exe pipeline.py --skip-vl"
    if (-not $SkipWeb) {
        Write-Host ""
        Write-Host "Web 系统（两个终端）:"
        Write-Host "  cd backend; go run ."
        Write-Host "  cd frontend; npm run dev"
        Write-Host "  浏览器 http://127.0.0.1:5173"
    }
    if ($script:Warnings.Count -gt 0) {
        Write-Host ""
        Write-Host "注意:" -ForegroundColor Yellow
        $script:Warnings | ForEach-Object { Write-Host "  - $_" -ForegroundColor Yellow }
    }
    exit 0
}

Write-Host "安装未完全成功:" -ForegroundColor Red
$script:Failed | ForEach-Object { Write-Host "  - $_" -ForegroundColor Red }
if ($script:Warnings.Count -gt 0) {
    Write-Host "注意:" -ForegroundColor Yellow
    $script:Warnings | ForEach-Object { Write-Host "  - $_" -ForegroundColor Yellow }
}
exit 1
