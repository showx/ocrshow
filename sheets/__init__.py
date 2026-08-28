#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""Sheet module registry.

Open-source core only ships `generic` + auto-detect.
Company-specific parsers live in `sheets/private/` and are not committed.
"""
from __future__ import annotations

import importlib
import pkgutil
from pathlib import Path
from typing import Any, Iterable, Protocol

ROOT = Path(__file__).resolve().parent.parent

_REGISTRY: list["SheetModule"] = []
_loaded = False


class SheetModule(Protocol):
    id: str
    name: str
    desc: str
    aliases: tuple[str, ...]
    priority: int
    fields: dict[str, str]
    csv_fields: list[str]
    output_stem: str
    columns: tuple[dict[str, str], ...] | list[dict[str, str]]

    def match(self, payload: Any) -> bool: ...
    def reconstruct(self, payload: Any, force: bool = False) -> list[dict[str, Any]]: ...


class BaseSheet:
    id = ""
    name = ""
    desc = ""
    aliases: tuple[str, ...] = ()
    priority = 10
    fields: dict[str, str] = {}
    csv_fields: list[str] = ["date", "image", "rank", "app_name", "source"]
    output_stem = ""
    columns: tuple[dict[str, str], ...] = (
        {"key": "rank", "label": "序号"},
        {"key": "app_name", "label": "名称"},
        {"key": "image", "label": "图片"},
    )

    def match(self, payload: Any) -> bool:
        return False

    def reconstruct(self, payload: Any, force: bool = False) -> list[dict[str, Any]]:
        return []

    def refine(self, image_path: Path, records: list[dict[str, Any]]) -> None:
        return None

    def fallback_from_tables(self, tables: list[list[list[str]]]) -> list[dict[str, Any]]:
        return []

    def compact_ocr(self, records: list[dict[str, Any]], markdown: str, limit: int = 400) -> str:
        lines = []
        for rec in records[:limit]:
            name = rec.get("app_name") or ""
            lines.append(f"{rec.get('rank')}\t{name}")
        body = "\n".join(lines)
        if not body:
            body = (markdown or "")[:8000]
        return body

    def convert_vl(self, rec: dict[str, Any], index: int) -> dict[str, Any] | None:
        name = str(rec.get("app_name") or rec.get("name") or "").strip()
        if not name:
            return None
        out = dict(rec)
        out["app_name"] = name
        out["rank"] = rec.get("rank") or index
        out["source"] = "qwen3-vl"
        return out

    def merge_hit(self, ocr: dict[str, Any], converted: dict[str, Any]) -> None:
        return None

    def vl_prompt(self, image_name: str, date: str, ocr_text: str) -> str:
        return f"""你是表格结构化专家。请对照原图，把 OCR 初稿整理成 JSON。

OCR 参考：
{ocr_text}

只输出 JSON：
{{
  "image": "{image_name}",
  "date": "{date}",
  "summary": "一句话",
  "fields": {{}},
  "records": [{{"rank": 1, "app_name": "名称"}}],
  "corrections": [],
  "notes": ""
}}"""


def register(mod: SheetModule) -> SheetModule:
    if not any(m.id == mod.id for m in _REGISTRY):
        _REGISTRY.append(mod)
    return mod


def _import_pack(package: str) -> None:
    try:
        pkg = importlib.import_module(package)
    except ImportError:
        return
    if not hasattr(pkg, "__path__"):
        return
    for info in pkgutil.iter_modules(pkg.__path__):
        if info.name.startswith("_"):
            continue
        importlib.import_module(f"{package}.{info.name}")


def load_modules() -> list[SheetModule]:
    global _loaded
    if not _loaded:
        importlib.import_module("sheets.generic")
        _import_pack("sheets.private")
        _loaded = True
    return list(_REGISTRY)


def by_id(sheet_id: str | None) -> SheetModule | None:
    if not sheet_id:
        return None
    key = str(sheet_id).strip().lower()
    for mod in load_modules():
        if mod.id == key or key in {a.lower() for a in (mod.aliases or ())}:
            return mod
    return None


def normalize_sheet_type(value: str | None) -> str | None:
    if value is None:
        return None
    key = str(value).strip().lower()
    if key in {"", "auto", "none"}:
        return None
    mod = by_id(key)
    if mod:
        return mod.id
    raise ValueError(f"未知类别: {value}。可用: auto / " + " / ".join(sorted(m.id for m in load_modules())))


def reconstruct(payload: Any, sheet_type: str | None = None) -> list[dict[str, Any]]:
    forced = normalize_sheet_type(sheet_type)
    if forced:
        mod = by_id(forced)
        if mod is None:
            return []
        recs = mod.reconstruct(payload, force=True)
        for rec in recs:
            rec["sheet_type"] = forced
        return recs
    ranked = sorted((m for m in load_modules() if m.id != "generic"), key=lambda m: -int(m.priority or 0))
    for mod in ranked:
        if mod.match(payload):
            recs = mod.reconstruct(payload, force=False)
            if recs:
                return recs
    gen = by_id("generic")
    return gen.reconstruct(payload) if gen else []


def refine(image_path: Path, records: list[dict[str, Any]]) -> None:
    if not records:
        return
    mod = by_id(str(records[0].get("sheet_type") or ""))
    if mod is not None:
        mod.refine(image_path, records)


def fallback_from_tables(tables: list[list[list[str]]], sheet_type: str | None) -> list[dict[str, Any]]:
    mod = by_id(normalize_sheet_type(sheet_type) or "") or by_id("generic")
    if mod is None:
        return []
    return mod.fallback_from_tables(tables)


def categories() -> list[dict[str, Any]]:
    items = [
        {
            "id": "auto",
            "name": "自动识别",
            "desc": "按表头匹配已安装的版式模块；没有模块时走通用表格",
            "columns": [
                {"key": "rank", "label": "序号"},
                {"key": "app_name", "label": "名称"},
                {"key": "image", "label": "图片"},
            ],
        }
    ]
    for mod in sorted(load_modules(), key=lambda m: (-int(m.priority or 0), m.id)):
        items.append(
            {
                "id": mod.id,
                "name": mod.name,
                "desc": mod.desc,
                "columns": list(mod.columns),
            }
        )
    return items


def iter_output_modules() -> Iterable[SheetModule]:
    return sorted(load_modules(), key=lambda m: m.id)
