#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""Open-source fallback: dump OCR cells as generic rows."""
from __future__ import annotations

from typing import Any

from ocrutil import ocr_items_from_payload
from sheets import BaseSheet, register


class GenericSheet(BaseSheet):
    id = "generic"
    name = "通用表格"
    desc = "不套内部版式，按识别框导出文本"
    aliases = ("table",)
    priority = 0
    fields = {"app_name": "文本"}
    csv_fields = ["date", "image", "rank", "excel_row", "app_name", "source"]
    output_stem = "generic"
    columns = (
        {"key": "rank", "label": "序号"},
        {"key": "excel_row", "label": "行"},
        {"key": "app_name", "label": "文本"},
        {"key": "image", "label": "图片"},
    )

    def match(self, payload: Any) -> bool:
        return True

    def reconstruct(self, payload: Any, force: bool = False) -> list[dict[str, Any]]:
        records: list[dict[str, Any]] = []
        for i, item in enumerate(ocr_items_from_payload(payload), start=1):
            text = str(item.get("text") or "").strip()
            if not text:
                continue
            records.append(
                {
                    "sheet_type": self.id,
                    "rank": len(records) + 1,
                    "excel_row": i,
                    "app_name": text,
                    "source": "ocr_boxes",
                    "x": item.get("x"),
                    "y": item.get("y"),
                }
            )
        return records

    def fallback_from_tables(self, tables: list[list[list[str]]]) -> list[dict[str, Any]]:
        records: list[dict[str, Any]] = []
        for table in tables:
            for r_i, row in enumerate(table):
                text = " | ".join(str(c).strip() for c in row if str(c).strip())
                if not text:
                    continue
                records.append(
                    {
                        "sheet_type": self.id,
                        "rank": len(records) + 1,
                        "excel_row": r_i,
                        "app_name": text,
                        "source": "ocr_html",
                    }
                )
        return records


register(GenericSheet())
