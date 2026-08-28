#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""Generic OCR helpers used by the pipeline and optional sheet modules."""
from __future__ import annotations

import json
import re
from datetime import datetime
from html.parser import HTMLParser
from pathlib import Path
from typing import Any

LEADING_NUM_NAME = re.compile(
    r"^\s*(-?\d+(?:\.\d+)?)[\s:：]*([\u4e00-\u9fffA-Za-z].+)$"
)


TRAILING_NUM = re.compile(r"^(.+?)[\s:：]*(-?\d+(?:\.\d+)?)\s*$")


EXCEL_COL = re.compile(r"^[A-P]$")


ROW_NAME = re.compile(r"^\s*(\d{1,2})[\s:：]*([\u4e00-\u9fffA-Za-z].+)$")


class HTMLTableParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.tables: list[list[list[str]]] = []
        self._table: list[list[str]] | None = None
        self._row: list[str] | None = None
        self._cell: list[str] | None = None
        self._in_cell = False

    def handle_starttag(self, tag: str, attrs) -> None:
        if tag == "table":
            self._table = []
        elif tag == "tr" and self._table is not None:
            self._row = []
        elif tag in {"td", "th"} and self._row is not None:
            self._cell = []
            self._in_cell = True

    def handle_endtag(self, tag: str) -> None:
        if tag in {"td", "th"} and self._row is not None and self._cell is not None:
            self._row.append(re.sub(r"\s+", " ", "".join(self._cell)).strip())
            self._cell = None
            self._in_cell = False
        elif tag == "tr" and self._table is not None and self._row is not None:
            if any(cell for cell in self._row):
                self._table.append(self._row)
            self._row = None
        elif tag == "table" and self._table is not None:
            if self._table:
                self.tables.append(self._table)
            self._table = None

    def handle_data(self, data: str) -> None:
        if self._in_cell and self._cell is not None:
            self._cell.append(data)


def parse_date_from_stem(stem: str) -> str | None:
    m = re.search(r"(?:^|[^0-9])((?:19|20)\d{2})(\d{2})(\d{2})(?:[^0-9]|$)", stem)
    if m:
        try:
            return datetime(int(m.group(1)), int(m.group(2)), int(m.group(3))).strftime("%Y-%m-%d")
        except ValueError:
            pass
    # 7 位日期少写一位时（例如 2060822 → 20260822）尝试补全
    m = re.search(r"(?i)d-(\d{7})(?:-|$)", stem)
    if m:
        raw = m.group(1)
        if raw.startswith("20"):
            guess = raw[:2] + "2" + raw[2:]
            try:
                return datetime(int(guess[:4]), int(guess[4:6]), int(guess[6:8])).strftime("%Y-%m-%d")
            except ValueError:
                pass
    if re.fullmatch(r"\d{2}", stem):
        month, day = int(stem[0]), int(stem[1])
    elif re.fullmatch(r"\d{3}", stem):
        month, day = int(stem[0]), int(stem[1:])
    elif re.fullmatch(r"\d{4}", stem):
        month, day = int(stem[:2]), int(stem[2:])
    else:
        return None
    year = datetime.now().year
    try:
        return datetime(year, month, day).strftime("%Y-%m-%d")
    except ValueError:
        return None


def parse_number(text: str) -> float | None:
    if text is None:
        return None
    cleaned = str(text).strip().replace(",", "").replace("，", "")
    cleaned = cleaned.replace(" ", "")
    m = re.search(r"-?\d+(?:\.\d+)?", cleaned)
    if not m:
        return None
    try:
        return float(m.group(0))
    except ValueError:
        return None


def split_num_name(text: str) -> tuple[float | None, str]:
    text = re.sub(r"\s+", " ", (text or "")).strip()
    if not text:
        return None, ""
    m = LEADING_NUM_NAME.match(text)
    if m:
        return parse_number(m.group(1)), m.group(2).strip()
    return parse_number(text), "" if parse_number(text) is not None and re.fullmatch(r"-?\d+(?:\.\d+)?", text.replace(" ", "")) else text


def box_center(box: list[int] | list[float]) -> tuple[float, float]:
    if len(box) >= 4 and not isinstance(box[0], (list, tuple)):
        x1, y1, x2, y2 = (float(box[0]), float(box[1]), float(box[2]), float(box[3]))
        return (x1 + x2) / 2.0, (y1 + y2) / 2.0
    xs = [float(p[0]) for p in box]
    ys = [float(p[1]) for p in box]
    return sum(xs) / len(xs), sum(ys) / len(ys)


def box_width(box: list[int] | list[float]) -> float:
    if len(box) >= 4 and not isinstance(box[0], (list, tuple)):
        return abs(float(box[2]) - float(box[0]))
    xs = [float(p[0]) for p in box]
    return max(xs) - min(xs) if xs else 0.0


def cluster_rows(items: list[dict[str, Any]], y_tol: float = 12.0) -> list[list[dict[str, Any]]]:
    ordered = sorted(items, key=lambda it: (it["y"], it["x"]))
    rows: list[list[dict[str, Any]]] = []
    for item in ordered:
        if rows:
            ys = [c["y"] for c in rows[-1]]
            row_y = sum(ys) / len(ys)
            if abs(item["y"] - row_y) <= y_tol:
                rows[-1].append(item)
                continue
        rows.append([item])
    for row in rows:
        row.sort(key=lambda it: it["x"])
    return rows


def cluster_columns(items: list[dict[str, Any]], x_tol: float = 90.0) -> list[list[dict[str, Any]]]:
    if not items:
        return []
    ordered = sorted(items, key=lambda it: it["x"])
    cols: list[list[dict[str, Any]]] = [[ordered[0]]]
    for item in ordered[1:]:
        mean_x = sum(c["x"] for c in cols[-1]) / len(cols[-1])
        if item["x"] - mean_x <= x_tol:
            cols[-1].append(item)
        else:
            cols.append([item])
    for col in cols:
        col.sort(key=lambda it: it["y"])
    return cols


def rows_from_columns(cols: list[list[dict[str, Any]]], min_col_size: int = 8) -> list[list[dict[str, Any]]]:
    if not cols:
        return []
    main = [col for col in cols if len(col) >= min_col_size]
    extra = [item for col in cols if len(col) < min_col_size for item in col]
    if not main:
        return cluster_rows(extra) if extra else []
    n_rows = max(len(col) for col in main)
    rows: list[list[dict[str, Any]]] = []
    for i in range(n_rows):
        cells = [col[i] for col in main if i < len(col)]
        cells.sort(key=lambda it: it["x"])
        rows.append(cells)
    for item in extra:
        if not rows:
            rows.append([item])
            continue
        best = min(rows, key=lambda row: abs(sum(c["y"] for c in row) / len(row) - item["y"]))
        best.append(item)
        best.sort(key=lambda it: it["x"])
    return rows


def strip_excel_row_prefix(text: str) -> str:
    """最左列行号（1-40 的整数）粘在名称前面时去掉。

    带小数的数值前缀不算行号，例如 270.4名称 里的 27 不应被剥掉。
    """
    text = re.sub(r"\s+", " ", (text or "")).strip()
    m = re.match(r"^(\d{1,2})(?!\d)(?!\.\d)[\s:：._-]*(.+)$", text)
    if not m:
        return text
    row_no = int(m.group(1))
    rest = m.group(2).strip()
    if 1 <= row_no <= 40 and re.match(r"[\u4e00-\u9fffA-Za-z]", rest):
        return rest
    return text


def overall_ocr(payload: Any) -> dict[str, Any]:
    ocr: Any = {}
    if isinstance(payload, dict):
        ocr = payload.get("overall_ocr_res") or {}
        if not ocr and "res" in payload:
            ocr = (payload.get("res") or {}).get("overall_ocr_res") or {}
    return ocr if isinstance(ocr, dict) else {}


def ocr_items_from_payload(payload: Any) -> list[dict[str, Any]]:
    ocr = overall_ocr(payload)
    items: list[dict[str, Any]] = []
    for text, box in zip(ocr.get("rec_texts") or [], ocr.get("rec_boxes") or []):
        x, y = box_center(box)
        box_xy: list[int] | None = None
        try:
            vals = list(box)[:4]
            if len(vals) == 4:
                box_xy = [int(v) for v in vals]
        except (TypeError, ValueError):
            box_xy = None
        items.append({"text": str(text).strip(), "x": x, "y": y, "w": box_width(box), "box": box_xy})
    return items


def longest_digit_run(text: str) -> str:
    runs = re.findall(r"\d+", text or "")
    return max(runs, key=len) if runs else ""


def split_row_and_name(text: str) -> tuple[int | None, str]:
    text = re.sub(r"\s+", " ", (text or "")).strip()
    m = re.match(r"^(\d{1,2})\s*([\u4e00-\u9fffA-Za-z].+)$", text)
    if m:
        return int(m.group(1)), m.group(2).strip()
    if re.fullmatch(r"\d{1,3}", text):
        return int(text), ""
    return None, text


def take_nearest(
    items: list[dict[str, Any]],
    y: float,
    used: set[int],
    max_dy: float = 20.0,
    min_dy: float = -6.0,
    max_below: float | None = None,
    below_penalty: float = 0.0,
) -> dict[str, Any] | None:
    if max_below is None:
        max_below = max_dy
    best_i = None
    best_score = None
    for i, item in enumerate(items):
        if i in used:
            continue
        dy = item["y"] - y
        if dy < min_dy or dy > max_below or abs(dy) > max_dy:
            continue
        score = abs(dy) + (below_penalty if dy > 0 else 0.0)
        if best_i is None or score < best_score:
            best_i = i
            best_score = score
    if best_i is None:
        return None
    used.add(best_i)
    return items[best_i]


def extract_html_tables(payload: Any) -> list[str]:
    htmls: list[str] = []
    if isinstance(payload, dict):
        for key in ("table_res_list", "table_result_list", "parsing_res_list"):
            items = payload.get(key) or []
            if isinstance(items, list):
                for item in items:
                    if not isinstance(item, dict):
                        continue
                    for hk in ("pred_html", "html", "table_html"):
                        html = item.get(hk)
                        if isinstance(html, str) and "<table" in html.lower():
                            htmls.append(html)
        rec = payload.get("overall_ocr_res") or payload.get("ocr_result") or {}
        if isinstance(rec, dict):
            html = rec.get("pred_html") or rec.get("html")
            if isinstance(html, str) and "<table" in html.lower():
                htmls.append(html)
        for value in payload.values():
            htmls.extend(extract_html_tables(value))
    elif isinstance(payload, list):
        for item in payload:
            htmls.extend(extract_html_tables(item))
    return htmls


def unique_keep_order(items: list[str]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for item in items:
        if item not in seen:
            seen.add(item)
            out.append(item)
    return out


def markdown_from_result(res: Any) -> str:
    md = getattr(res, "markdown", None)
    if isinstance(md, dict):
        text = md.get("markdown_texts") or md.get("text") or ""
        if isinstance(text, list):
            return "\n\n".join(str(x) for x in text)
        return str(text)
    if isinstance(md, str):
        return md
    return ""


def json_from_result(res: Any) -> Any:
    if hasattr(res, "json"):
        data = res.json
        if callable(data):
            try:
                data = data()
            except TypeError:
                pass
        if isinstance(data, dict) and "res" in data:
            return data["res"]
        return data
    return {}


def load_saved_structure_json(image_out: Path, stem: str) -> Any | None:
    candidates = [
        image_out / f"{stem}_res.json",
        image_out / f"{stem}.json",
    ]
    candidates.extend(sorted(image_out.glob("*_res.json")))
    seen: set[Path] = set()
    for path in candidates:
        if path in seen or not path.exists():
            continue
        seen.add(path)
        return json.loads(path.read_text(encoding="utf-8"))
    return None


def save_flatten(image_out: Path, item: dict[str, Any]) -> None:
    records = item.get("ocr_records") or []
    image_out.mkdir(parents=True, exist_ok=True)
    (image_out / "ocr_flatten.json").write_text(
        json.dumps(
            {
                "image": item["image"],
                "sheet_type": item.get("sheet_type"),
                "date": item.get("date"),
                "record_count": len(records),
                "records": records,
                "markdown": item.get("markdown") or "",
            },
            ensure_ascii=False,
            indent=2,
        ),
        encoding="utf-8",
    )


def extract_json_object(text: str) -> dict[str, Any]:
    if not text:
        raise ValueError("空响应")
    text = text.strip()
    fence = re.search(r"```(?:json)?\s*([\s\S]*?)```", text)
    if fence:
        text = fence.group(1).strip()
    try:
        data = json.loads(text)
        if isinstance(data, dict):
            return data
    except json.JSONDecodeError:
        pass
    start = text.find("{")
    end = text.rfind("}")
    if start >= 0 and end > start:
        try:
            data = json.loads(text[start : end + 1])
            if isinstance(data, dict):
                return data
        except json.JSONDecodeError:
            pass
    salvaged = salvage_truncated_records(text)
    if salvaged:
        return salvaged
    raise ValueError("未能从模型输出中解析 JSON")


def salvage_truncated_records(text: str) -> dict[str, Any] | None:
    m = re.search(r'"records"\s*:\s*\[', text)
    if not m:
        return None
    rest = text[m.end() :]
    decoder = json.JSONDecoder()
    records: list[Any] = []
    i = 0
    while i < len(rest):
        while i < len(rest) and rest[i] in " \n\r\t,":
            i += 1
        if i >= len(rest) or rest[i] in "]":
            break
        try:
            obj, n = decoder.raw_decode(rest, i)
        except json.JSONDecodeError:
            break
        if isinstance(obj, dict):
            records.append(obj)
        i += n
    if not records:
        return None
    return {
        "records": records,
        "notes": f"输出被截断，已抢救 {len(records)} 条",
    }

