#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""Load local config without putting secrets or sample images in git.

Precedence: code defaults → config.example.toml → config.toml → .env / env vars.
"""
from __future__ import annotations

import os
import tomllib
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent
IMAGE_EXTS = {".jpg", ".jpeg", ".png", ".webp", ".bmp", ".tif", ".tiff"}


def _deep_merge(base: dict[str, Any], override: dict[str, Any]) -> dict[str, Any]:
    out = dict(base)
    for key, value in override.items():
        if isinstance(value, dict) and isinstance(out.get(key), dict):
            out[key] = _deep_merge(out[key], value)
        elif value is not None:
            out[key] = value
    return out


def load_env_file(path: Path) -> None:
    if not path.is_file():
        return
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.lower().startswith("export "):
            line = line[7:].strip()
        if "=" not in line:
            continue
        key, _, val = line.partition("=")
        key = key.strip()
        val = val.strip()
        if len(val) >= 2 and val[0] == val[-1] and val[0] in {"'", '"'}:
            val = val[1:-1]
        if key and key not in os.environ:
            os.environ[key] = val


def load_toml_file(path: Path) -> dict[str, Any]:
    if not path.is_file():
        return {}
    with path.open("rb") as fh:
        data = tomllib.load(fh)
    return data if isinstance(data, dict) else {}


def default_app_config() -> dict[str, Any]:
    return {
        "device": "auto",
        "addr": ":8080",
        "paths": {"images": "samples", "output": "output", "data": "data"},
        "vl": {
            "skip": False,
            "host": "http://127.0.0.1:11434",
            "model": "qwen3-vl:8b",
            "timeout": 600,
            "api_key": "",
        },
    }


def apply_env_overrides(cfg: dict[str, Any]) -> dict[str, Any]:
    if v := os.environ.get("OCR_DEVICE"):
        cfg["device"] = v
    if v := os.environ.get("OCR_ADDR"):
        cfg["addr"] = v
    paths = cfg.setdefault("paths", {})
    if v := os.environ.get("OCR_IMAGES"):
        paths["images"] = v
    if v := os.environ.get("OCR_OUT") or os.environ.get("OCR_OUTPUT"):
        paths["output"] = v
    if v := os.environ.get("OCR_DATA"):
        paths["data"] = v
    vl = cfg.setdefault("vl", {})
    if v := os.environ.get("OCR_VL_HOST"):
        vl["host"] = v
    if v := os.environ.get("OCR_VL_MODEL"):
        vl["model"] = v
    if v := os.environ.get("OCR_VL_API_KEY"):
        vl["api_key"] = v
    if v := os.environ.get("OCR_VL_TIMEOUT"):
        try:
            vl["timeout"] = int(v)
        except ValueError:
            pass
    if v := os.environ.get("OCR_SKIP_VL"):
        vl["skip"] = v.strip().lower() in {"1", "true", "yes", "on"}
    return cfg


def load_app_config(extra: Path | None = None) -> dict[str, Any]:
    load_env_file(ROOT / ".env")
    cfg = default_app_config()
    cfg = _deep_merge(cfg, load_toml_file(ROOT / "config.example.toml"))
    cfg = _deep_merge(cfg, load_toml_file(ROOT / "config.toml"))
    if extra:
        cfg = _deep_merge(cfg, load_toml_file(Path(extra)))
    cfg = apply_env_overrides(cfg)
    key = str((cfg.get("vl") or {}).get("api_key") or "").strip()
    if key and "OCR_VL_API_KEY" not in os.environ:
        os.environ["OCR_VL_API_KEY"] = key
    return cfg


def resolve_under_root(path_value: str | Path | None, fallback: str) -> Path:
    raw = str(path_value or fallback).strip() or fallback
    path = Path(raw)
    return path if path.is_absolute() else ROOT / path
