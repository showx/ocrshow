#!/usr/bin/env bash
# ocrshow 一键安装（Linux）：Python 虚拟环境、PaddleOCR、Go 依赖、前端 npm。
# 用法:
#   ./setup.sh
#   ./setup.sh --cpu          # 强制 CPU 版 PaddlePaddle
#   ./setup.sh --skip-web     # 只装 OCR，不装 Go / 前端
#   ./setup.sh --with-vl      # 额外拉取 qwen3-vl:8b
#   ./setup.sh --skip-system  # 缺 Go/Node 时不自动下载到 ~/.local

set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

CPU=0
SKIP_WEB=0
WITH_VL=0
SKIP_SYSTEM=0

for arg in "$@"; do
  case "$arg" in
    --cpu) CPU=1 ;;
    --skip-web) SKIP_WEB=1 ;;
    --with-vl) WITH_VL=1 ;;
    --skip-system) SKIP_SYSTEM=1 ;;
    -h|--help)
      sed -n '2,8p' "$0"
      exit 0
      ;;
    *)
      echo "未知参数: $arg  （可用 --cpu --skip-web --with-vl --skip-system）" >&2
      exit 2
      ;;
  esac
done

PY_INDEX="https://pypi.tuna.tsinghua.edu.cn/simple"
PADDLE_GPU_INDEX="https://www.paddlepaddle.org.cn/packages/stable/cu126/"
PADDLE_CPU_INDEX="https://www.paddlepaddle.org.cn/packages/stable/cpu/"
PADDLE_VER="3.2.2"
NPM_REGISTRY="https://registry.npmmirror.com"
GOPROXY_URL="https://goproxy.cn,direct"
GO_VER="1.22.12"
NODE_VER="v20.19.4"

FAILED=()
WARNINGS=()

c_cyan=$'\033[36m'
c_green=$'\033[32m'
c_yellow=$'\033[33m'
c_red=$'\033[31m'
c_gray=$'\033[90m'
c_reset=$'\033[0m'

step() { printf '\n%s==> %s%s\n' "$c_cyan" "$1" "$c_reset"; }
ok()   { printf '    %sOK  %s%s\n' "$c_green" "$1" "$c_reset"; }
warn() { printf '    %sWARN  %s%s\n' "$c_yellow" "$1" "$c_reset"; WARNINGS+=("$1"); }
fail() { printf '    %sFAIL  %s%s\n' "$c_red" "$1" "$c_reset"; FAILED+=("$1"); }

have() { command -v "$1" >/dev/null 2>&1; }

refresh_path() {
  export PATH="$HOME/.local/bin:$HOME/.local/go/bin:$HOME/.local/node/bin:$PATH"
  if [[ -n "${HOME:-}" && -f "$HOME/.local/bin/env" ]]; then
    # shellcheck disable=SC1091
    source "$HOME/.local/bin/env" 2>/dev/null || true
  fi
  if [[ -n "${CARGO_HOME:-}" && -f "$CARGO_HOME/env" ]]; then
    # shellcheck disable=SC1091
    source "$CARGO_HOME/env" 2>/dev/null || true
  fi
}

arch_triple() {
  local m
  m="$(uname -m)"
  case "$m" in
    x86_64|amd64) echo "amd64 x64" ;;
    aarch64|arm64) echo "arm64 arm64" ;;
    *) echo "" ;;
  esac
}

download() {
  local url="$1" dest="$2"
  if have curl; then
    curl -fL --retry 3 -o "$dest" "$url"
  elif have wget; then
    wget -O "$dest" "$url"
  else
    return 1
  fi
}

# ---------------------------------------------------------------------------
printf '%socrshow 环境安装 (Linux)%s\n' "$c_cyan" "$c_reset"
echo "目录: $ROOT"

if [[ "$(uname -s)" != "Linux" ]]; then
  warn "当前系统是 $(uname -s)，本脚本按 Linux 编写。Windows 请用 setup.ps1 / setup.bat"
fi

refresh_path

# ---- uv + Python 3.11 ------------------------------------------------------
step "Python 3.11 虚拟环境"

if ! have uv; then
  echo "    未找到 uv，正在安装..."
  if have curl; then
    if curl -LsSf https://astral.sh/uv/install.sh | sh; then
      refresh_path
    else
      fail "安装 uv 失败"
    fi
  else
    fail "未找到 curl，无法安装 uv。请先安装 curl，或见 https://docs.astral.sh/uv/"
  fi
fi

if ! have uv; then
  fail "仍找不到 uv。请重新打开终端后重跑，或手动安装: https://docs.astral.sh/uv/"
else
  ok "uv $(uv --version)"
  echo "    确保 Python 3.11 可用..."
  if uv python install 3.11 && {
       if [[ -d .venv ]]; then
         echo "    已有 .venv，跳过创建"
         true
       else
         uv venv .venv --python 3.11
       fi
     }; then
    ok ".venv 就绪"
  else
    fail "创建虚拟环境失败"
  fi
fi

VENV_PY="$ROOT/.venv/bin/python"

# ---- 可选系统库（无密码 sudo 时才装，避免卡住）------------------------------
if have apt-get && sudo -n true 2>/dev/null; then
  step "系统库（libGL / OpenMP）"
  if sudo -n apt-get update -qq && sudo -n apt-get install -y -qq libgl1 libglib2.0-0 libgomp1; then
    ok "已安装 libgl1 libglib2.0-0 libgomp1"
  else
    warn "系统库安装失败，PaddleOCR 若报缺 libGL，请手动: sudo apt-get install -y libgl1 libglib2.0-0 libgomp1"
  fi
fi

# ---- PaddlePaddle + OCR ----------------------------------------------------
if [[ -x "$VENV_PY" ]] && have uv; then
  step "PaddlePaddle / PaddleOCR"

  USE_GPU=1
  if [[ "$CPU" -eq 1 ]]; then
    USE_GPU=0
  fi

  if [[ "$USE_GPU" -eq 1 ]]; then
    GPU_OK=0
    if have nvidia-smi && nvidia-smi -L; then
      GPU_OK=1
    fi
    ARCH="$(uname -m)"
    if [[ "$ARCH" != "x86_64" && "$ARCH" != "amd64" ]]; then
      warn "当前架构 $ARCH，PaddlePaddle GPU wheel 通常仅 x86_64，改装 CPU"
      GPU_OK=0
    fi
    if [[ "$GPU_OK" -eq 0 ]]; then
      if [[ "$CPU" -eq 0 ]]; then
        warn "未检测到 NVIDIA GPU / nvidia-smi，改装 CPU 版 PaddlePaddle"
      fi
      USE_GPU=0
    fi
  fi

  paddle_ok=1
  if [[ "$USE_GPU" -eq 1 ]]; then
    echo "    安装 paddlepaddle-gpu==${PADDLE_VER} (CUDA 12.6 wheel)..."
    if ! uv pip install "paddlepaddle-gpu==${PADDLE_VER}" -i "$PADDLE_GPU_INDEX" --python "$VENV_PY"; then
      fail "paddlepaddle-gpu 安装失败"
      paddle_ok=0
    fi
  else
    echo "    安装 paddlepaddle==${PADDLE_VER} (CPU)..."
    if ! uv pip install "paddlepaddle==${PADDLE_VER}" -i "$PADDLE_CPU_INDEX" --python "$VENV_PY"; then
      fail "paddlepaddle 安装失败"
      paddle_ok=0
    fi
  fi

  if [[ "$paddle_ok" -eq 1 ]]; then
    echo "    安装 paddleocr / openpyxl / requests ..."
    if ! uv pip install "paddleocr[doc-parser]" openpyxl requests openai pillow -i "$PY_INDEX" --python "$VENV_PY"; then
      fail "PaddleOCR 依赖安装失败"
      paddle_ok=0
    fi
  fi

  if [[ "$paddle_ok" -eq 1 && "$USE_GPU" -eq 1 ]]; then
    if ! uv pip install "paddlepaddle-gpu==${PADDLE_VER}" -i "$PADDLE_GPU_INDEX" --python "$VENV_PY"; then
      fail "重新固定 paddlepaddle-gpu 失败"
      paddle_ok=0
    fi
  fi

  if [[ "$paddle_ok" -eq 1 ]]; then
    echo "    校验 import paddle ..."
    if "$VENV_PY" -c "import paddle, paddleocr; print('paddle', paddle.__version__, 'cuda', paddle.is_compiled_with_cuda())"; then
      ok "Python OCR 依赖已安装"
    else
      fail "import paddle / paddleocr 失败。若提示缺 libGL，请执行: sudo apt-get install -y libgl1 libglib2.0-0 libgomp1"
    fi
  fi
fi

# ---- Go / Node 用户目录安装 -------------------------------------------------
install_go_local() {
  local triple go_arch
  triple="$(arch_triple)"
  if [[ -z "$triple" ]]; then
    warn "无法识别 CPU 架构 $(uname -m)，请手动安装 Go: https://go.dev/dl/"
    return 1
  fi
  go_arch="${triple%% *}"
  local tar="go${GO_VER}.linux-${go_arch}.tar.gz"
  local url="https://golang.google.cn/dl/${tar}"
  local tmp
  tmp="$(mktemp -d)"
  echo "    下载 Go ${GO_VER} -> ~/.local/go ..."
  if ! download "$url" "$tmp/$tar"; then
    url="https://go.dev/dl/${tar}"
    download "$url" "$tmp/$tar" || { rm -rf "$tmp"; return 1; }
  fi
  mkdir -p "$HOME/.local"
  rm -rf "$HOME/.local/go"
  tar -C "$HOME/.local" -xzf "$tmp/$tar"
  rm -rf "$tmp"
  refresh_path
  have go
}

install_node_local() {
  local triple node_arch
  triple="$(arch_triple)"
  if [[ -z "$triple" ]]; then
    warn "无法识别 CPU 架构 $(uname -m)，请手动安装 Node.js: https://nodejs.org/"
    return 1
  fi
  node_arch="${triple##* }"
  local name="node-${NODE_VER}-linux-${node_arch}"
  local tar="${name}.tar.gz"
  local url="https://npmmirror.com/mirrors/node/${NODE_VER}/${tar}"
  local tmp
  tmp="$(mktemp -d)"
  echo "    下载 Node.js ${NODE_VER} -> ~/.local/node ..."
  if ! download "$url" "$tmp/$tar"; then
    url="https://nodejs.org/dist/${NODE_VER}/${tar}"
    download "$url" "$tmp/$tar" || { rm -rf "$tmp"; return 1; }
  fi
  mkdir -p "$HOME/.local"
  rm -rf "$HOME/.local/node"
  tar -C "$tmp" -xzf "$tmp/$tar"
  mv "$tmp/$name" "$HOME/.local/node"
  rm -rf "$tmp"
  refresh_path
  have npm
}

# ---- Go --------------------------------------------------------------------
if [[ "$SKIP_WEB" -eq 0 ]]; then
  step "Go 后端依赖"

  if ! have go && [[ "$SKIP_SYSTEM" -eq 0 ]]; then
    if install_go_local; then
      ok "已安装到 ~/.local/go"
    else
      warn "自动安装 Go 失败。请安装 Go 1.21+ 后重跑: https://go.dev/dl/"
    fi
  fi

  if have go; then
    ok "$(go version)"
    export GOPROXY="$GOPROXY_URL"
    if (cd "$ROOT/backend" && go mod tidy); then
      ok "go mod tidy 完成"
    else
      fail "go mod tidy 失败"
    fi
  else
    fail "跳过 Go 依赖（未安装 Go）"
  fi
fi

# ---- Node / 前端 ------------------------------------------------------------
if [[ "$SKIP_WEB" -eq 0 ]]; then
  step "前端 npm 依赖"

  if ! have npm && [[ "$SKIP_SYSTEM" -eq 0 ]]; then
    if install_node_local; then
      ok "已安装到 ~/.local/node"
    else
      warn "自动安装 Node.js 失败。请安装 Node.js 18+ 后重跑: https://nodejs.org/"
    fi
  fi

  if have npm; then
    ok "node $(node -v) / npm $(npm -v)"
    if (cd "$ROOT/frontend" && npm install --registry "$NPM_REGISTRY"); then
      ok "frontend/node_modules 就绪"
    else
      fail "npm install 失败"
    fi
  else
    fail "跳过前端依赖（未安装 Node.js）"
  fi
fi

# ---- 可选 VL ----------------------------------------------------------------
if [[ "$WITH_VL" -eq 1 ]]; then
  step "Ollama Qwen3-VL"
  if have ollama; then
    if ollama pull qwen3-vl:8b; then
      ok "已拉取 qwen3-vl:8b"
    else
      fail "ollama pull 失败"
    fi
  else
    warn "未找到 ollama。请先安装 https://ollama.com/ 再执行: ollama pull qwen3-vl:8b"
  fi
else
  printf '\n    %s提示: 需要 Qwen3-VL 纠错时，安装 Ollama 后执行  ollama pull qwen3-vl:8b%s\n' "$c_gray" "$c_reset"
  printf '          %s或重跑  ./setup.sh --with-vl%s\n' "$c_gray" "$c_reset"
fi

# ---- 本机配置（不入库） ----------------------------------------------------
if [[ ! -f "$ROOT/config.toml" && -f "$ROOT/config.example.toml" ]]; then
  cp "$ROOT/config.example.toml" "$ROOT/config.toml"
  ok "已生成 config.toml（不会提交到 git）"
fi
if [[ ! -f "$ROOT/.env" && -f "$ROOT/.env.example" ]]; then
  cp "$ROOT/.env.example" "$ROOT/.env"
  ok "已生成 .env（密钥写这里，不要提交）"
fi
mkdir -p "$ROOT/samples"

# ---- 总结 -------------------------------------------------------------------
printf '\n%s========================================%s\n' "$c_cyan" "$c_reset"
if [[ ${#FAILED[@]} -eq 0 ]]; then
  printf '%s安装完成%s\n\n' "$c_green" "$c_reset"
  echo "命令行 OCR:"
  echo "  把截图放到 samples/ ，然后"
  echo "  .venv/bin/python pipeline.py --skip-vl"
  if [[ "$SKIP_WEB" -eq 0 ]]; then
    echo ""
    echo "Web 系统（两个终端）:"
    echo "  cd backend && go run ."
    echo "  cd frontend && npm run dev"
    echo "  浏览器 http://127.0.0.1:5173"
    if [[ "$SKIP_SYSTEM" -eq 0 ]]; then
      echo ""
      echo "若本会话找不到 go / node，把下面两行写入 ~/.bashrc 后重新打开终端:"
      echo "  export PATH=\"\$HOME/.local/bin:\$HOME/.local/go/bin:\$HOME/.local/node/bin:\$PATH\""
    fi
  fi
  if [[ ${#WARNINGS[@]} -gt 0 ]]; then
    printf '\n%s注意:%s\n' "$c_yellow" "$c_reset"
    for w in "${WARNINGS[@]}"; do printf '  - %s%s%s\n' "$c_yellow" "$w" "$c_reset"; done
  fi
  exit 0
fi

printf '%s安装未完全成功:%s\n' "$c_red" "$c_reset"
for f in "${FAILED[@]}"; do printf '  - %s%s%s\n' "$c_red" "$f" "$c_reset"; done
if [[ ${#WARNINGS[@]} -gt 0 ]]; then
  printf '%s注意:%s\n' "$c_yellow" "$c_reset"
  for w in "${WARNINGS[@]}"; do printf '  - %s%s%s\n' "$c_yellow" "$w" "$c_reset"; done
fi
exit 1
