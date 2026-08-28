# ocrshow

把表格截图识别成结构化记录。开源核心是 **PaddleOCR PP-StructureV3** 做表格 OCR，再按已安装的版式模块拆字段；可选 **Qwen3-VL** 对照原图纠错。

默认只有 **自动识别** 和 **通用表格**。公司内部版式做成独立模块，丢进 `sheets/private/` 即可拼接，不会进公开仓库。

## 开源与私有

本仓库只提交**通用代码和配置模板**。截图、识别结果、密钥、内部版式解析都留在本地。

| 可开源（提交） | 不要提交 |
|---|---|
| `pipeline.py`、`ocrutil.py`、`sheets/` 注册表与 `generic` | `sheets/private/` 内部版式模块 |
| 前后端、安装脚本 | `samples/` 里的真实截图 |
| `config.example.toml`、`.env.example` | `config.toml`、`.env` |
| README、依赖清单 | `data/`、`output/`、模型、xlsx/csv 结果 |

使用前：

```bash
copy config.example.toml config.toml   # Linux: cp config.example.toml config.toml
copy .env.example .env                 # 密钥写在 .env，不要写进 toml
```

把待识别截图放到 `samples/`（该目录会进 git，**里面的图片不会**）。命令行不传路径时默认扫这个目录。Web 上传的文件写在 `data/uploads/`，同样不会入库。

配置优先级：**代码默认 → `config.example.toml` → `config.toml` → `.env` / 环境变量 → 命令行参数**。

内部版式：把 `*.py` 和同名 `*.toml` 放到 `sheets/private/`，启动后 Web 类别和 `--sheet-type` 会自动出现。写法见 [`sheets/README.md`](sheets/README.md)。

## Web 系统

上传图片、选择类别后自动识别。后端 Go + SQLite，前端 Vue，识别仍走本仓库的 `pipeline.py`。

```
浏览器  →  Vue（:5173）  →  Go API（:8080）  →  SQLite / 上传目录
                                    ↓
                              pipeline.py（PaddleOCR，可选 Qwen3-VL）
                                    ↓
                              sheets 模块（generic + 可选 private）
```

1. 一键安装环境（见下方「环境」）
2. 启动：

```bash
# 终端 1：后端（go run . 或 go run main.go 均可）
cd backend
go run .

# 终端 2：前端
cd frontend
npm run dev
```

浏览器打开 http://127.0.0.1:5173 。默认选「自动识别」；装了内部模块后会出现对应按钮。任务排队执行（GPU 上一次只跑一个，避免抢卡）。

生产环境可先 `npm run build`，再只启动 Go；它会托管 `frontend/dist`，默认监听 `:8080`。

常用环境变量（也可写在 `.env`，完整项见 `.env.example`）：

| 变量 | 说明 |
|---|---|
| `OCR_PYTHON` | Python 解释器，默认项目里的 `.venv` |
| `OCR_ADDR` | 监听地址，默认 `:8080` |
| `OCR_DEVICE` | `auto` / `gpu` / `cpu` |
| `OCR_DATA` | SQLite 与上传目录，默认 `data/` |
| `OCR_IMAGES` | 命令行默认截图目录，默认 `samples/` |
| `OCR_VL_HOST` / `OCR_VL_MODEL` | Ollama 地址与模型名 |
| `OCR_VL_API_KEY` | 云端视觉模型密钥（若使用） |

## 处理流程

1. PP-StructureV3 识别图片，得到文字框和表格结果
2. **自动识别**：按模块 `priority` 从高到低做表头匹配（跳过 `generic`），第一个命中且能拆出记录的生效
3. 没有命中时走 **通用表格**：把识别框导出为文本行
4. 也可 `--sheet-type <模块 id>` 强制指定
5. 可选 Qwen3-VL：对照原图纠错；提示词和字段合并由当前模块提供

文件名会用来推断日期，例如 `81.jpg` → 8 月 1 日，`d-20260822-1.jpg` → 2026-08-22。

## 环境

一键安装 Python 3.11 虚拟环境、PaddleOCR、Go 模块、前端 npm：

```powershell
# Windows（也可双击 setup.bat）
.\setup.ps1
```

```bash
# Linux
chmod +x setup.sh
./setup.sh
```

无 NVIDIA GPU 时加 `-Cpu` / `--cpu`；只要命令行 OCR、不要 Web 时加 `-SkipWeb` / `--skip-web`；同时拉取 Qwen3-VL 时加 `-WithVL` / `--with-vl`。

脚本会用 uv 自动准备 Python 3.11。Web 系统还需要 Go 1.21+、Node.js 18+：Windows 缺省时尝试 winget；Linux 缺省时下载到 `~/.local/go` 和 `~/.local/node`（无需 root）。Linux 上 PaddleOCR 还依赖 `libGL` 等系统库，有免密 sudo 时会尝试 `apt-get install`。

也可手动安装。需要 Python 3.11、NVIDIA GPU（推荐），以及本机 [Ollama](https://ollama.com/)（仅在启用 Qwen3-VL 时需要）。

```bash
uv venv .venv --python 3.11
source .venv/bin/activate          # Windows: .\.venv\Scripts\activate

# PaddlePaddle GPU（CUDA 12.6 wheel，驱动需支持）
uv pip install paddlepaddle-gpu==3.2.2 -i https://www.paddlepaddle.org.cn/packages/stable/cu126/

# 国内镜像安装其余依赖
uv pip install "paddleocr[doc-parser]" openpyxl requests openai pillow -i https://pypi.tuna.tsinghua.edu.cn/simple
```

第一次跑 PP-StructureV3 会自动下载模型到 `~/.paddlex/official_models`（Windows 为 `%USERPROFILE%\.paddlex/official_models`）。

Qwen3-VL（可选）：

```bash
ollama pull qwen3-vl:8b
```

## 用法

默认处理 `samples/`（或 `config.toml` 里 `paths.images`）下的图片。真实截图不要放进 git。

解释器：Linux 用 `.venv/bin/python`，Windows 用 `.venv\Scripts\python.exe`。

```bash
# 只跑 OCR + 字段拆分（不调用视觉模型）
.venv/bin/python pipeline.py --skip-vl

# 复用已有 OCR JSON，只重新拆字段
.venv/bin/python pipeline.py --from-ocr --skip-vl

# OCR 之后用 Qwen3-VL 纠错整理（需本机 Ollama）
.venv/bin/python pipeline.py --with-vl

# 指定图片（路径随意，不必在 samples/）
.venv/bin/python pipeline.py path/to/a.jpg path/to/b.jpg --skip-vl

# 强制某个已安装模块（未安装则报错）
.venv/bin/python pipeline.py --sheet-type generic --skip-vl
```

常用参数：

| 参数 | 说明 |
|---|---|
| `--skip-vl` | 只跑 OCR，不调用 Qwen3-VL |
| `--with-vl` | 强制调用 Qwen3-VL（覆盖配置里的 `vl.skip`） |
| `--config` | 额外 TOML，覆盖 `config.toml` |
| `--images-dir` | 截图目录，默认 `samples/` |
| `--from-ocr` | 复用 `output/<图名>/` 里已有的 PP-StructureV3 JSON |
| `--device auto\|gpu\|cpu` | PP-StructureV3 推理设备，默认自动选 GPU |
| `--vl-model` | Ollama 模型名，默认 `qwen3-vl:8b` |
| `--vl-host` | Ollama 地址，默认 `http://127.0.0.1:11434` |
| `--out` | 输出目录，默认 `output` |
| `--sheet-type` | 强制版式：`auto` 或已安装模块 id |

## 输出

```
output/
  records.json         # 全部记录
  generic.csv          # 未匹配内部模块时的通用表格
  generic.json
  records.xlsx         # 每个已安装模块一个 sheet，另加 corrections
  summary.json
  result.json          # Web 任务读取
  81/
    81_res.json        # PP-StructureV3 原始结果
    81.md
    ocr_flatten.json   # 拆字段后的记录
    qwen3vl.json       # 仅在启用 VL 时生成
```

装了内部模块后，还会按模块的 `output_stem` 写出对应 csv/json。个别汉字、链接仍可能认错；需要更高准确率时去掉 `--skip-vl`，让 Qwen3-VL 对照原图修正。
