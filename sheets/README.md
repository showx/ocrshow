# 版式模块

开源仓库只带 **自动识别骨架** 和 **generic（通用表格）**。

公司内部解析放到 `private/`，这个目录已被 `.gitignore`，克隆公开仓库的人看不到。把模块文件拷进来即可拼接，不必改 `pipeline.py`。

```
sheets/
  __init__.py       # 注册表：load / match / reconstruct
  generic.py        # 开源兜底
  generic.toml      # 给 Go 前端用的目录信息
  private/          # 不入库；启动时自动 import
    __init__.py     # 可空
    my_sheet.py     # register(MySheet())
    my_sheet.toml   # 与 py 的 id 一致，Web 才会出现按钮
```

## 自动识别

1. 加载 `sheets.generic`，再扫描 `sheets.private` 下非 `_` 开头的包
2. 指定 `--sheet-type` / Web 类别时，直接走该模块
3. 否则按 `priority` 从高到低调用 `match()`（跳过 `generic`）
4. 第一个 `match` 成功且 `reconstruct` 有记录的生效
5. 都没有则 `generic`

`generic` 的 `priority` 是 0，内部模块请用更大的数。

## 一个模块要提供什么

Python 类（继承 `sheets.BaseSheet` 或实现同样接口）：

| 属性 / 方法 | 作用 |
|---|---|
| `id` / `name` / `desc` | 标识与界面文案 |
| `priority` | 自动识别时的优先级，越大越先 |
| `aliases` | `--sheet-type` 别名 |
| `fields` / `csv_fields` / `output_stem` / `columns` | 导出与表格列 |
| `match(payload)` | 是否像这张表（看表头等） |
| `reconstruct(payload, force=False)` | 从 OCR JSON 拆记录 |
| `refine` / `fallback_from_tables` | 可选后处理、HTML 表兜底 |
| `vl_prompt` / `convert_vl` / `merge_hit` / `compact_ocr` | 可选 VL 纠错 |

文件末尾调用 `register(MySheet())`。

同名 toml 给 Web/Go 用（Go 不 import Python，只扫 `sheets/**/*.toml`）：

```toml
id = "my_sheet"
name = "我的清单"
desc = "一句话说明"
priority = 50

[[columns]]
key = "rank"
label = "序号"

[[columns]]
key = "app_name"
label = "名称"
```

## 最小例子

```python
from sheets import BaseSheet, register

class MySheet(BaseSheet):
    id = "my_sheet"
    name = "我的清单"
    desc = "示例"
    priority = 50
    output_stem = "my_sheet"
    csv_fields = ["date", "image", "rank", "app_name", "source"]

    def match(self, payload):
        # 看 OCR 文本里有没有自家表头
        return False

    def reconstruct(self, payload, force=False):
        if not force and not self.match(payload):
            return []
        return []

register(MySheet())
```

通用 OCR 工具（聚类、HTML 表、JSON 解析）在仓库根目录的 `ocrutil.py`，模块里按需 import。
