#!/usr/bin/env python3
"""Import AWS Claude application inference profiles as aggregated channels.

Edit the global configuration below, then run:

    python3 scripts/import_aws_claude_channels/import_channels.py /path/to/channels.xlsx

The input must contain the columns: model, arena, arn. CSV and TSV files are
also supported. Reading .xlsx files requires openpyxl (`pip install openpyxl`).
"""

from __future__ import annotations

import csv
import json
import re
import sys
from collections import OrderedDict, defaultdict
from pathlib import Path
from typing import Any, Iterable
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


# Global configuration
API_URL = "http://localhost:3000/api/channel/"
COOKIE = ""
AUTHORIZATION = ""
AWS_ACCESS_KEY_ID = ""
AWS_SECRET_ACCESS_KEY = ""

CHANNEL_NAME_PREFIX = "AWS Claude"
CHANNEL_GROUP = "default"
REQUEST_TIMEOUT_SECONDS = 30
DRY_RUN = False

AWS_CHANNEL_TYPE = 33
REQUIRED_COLUMNS = ("model", "arena", "arn")
MODEL_PREFIX_PATTERN = re.compile(r"^(?:[a-z0-9-]+\.)?anthropic\.", re.IGNORECASE)
MODEL_VERSION_SUFFIX_PATTERN = re.compile(r"-v\d+(?::\d+)?$", re.IGNORECASE)


def normalize_model(model: str) -> str:
    normalized = MODEL_PREFIX_PATTERN.sub("", model.strip(), count=1)
    return MODEL_VERSION_SUFFIX_PATTERN.sub("", normalized, count=1)


def read_rows(path: Path) -> list[dict[str, str]]:
    suffix = path.suffix.lower()
    if suffix == ".xlsx":
        try:
            from openpyxl import load_workbook
        except ImportError as exc:
            raise RuntimeError(
                "读取 .xlsx 需要 openpyxl，请先执行: pip install openpyxl"
            ) from exc

        workbook = load_workbook(path, read_only=True, data_only=True)
        try:
            sheet = workbook.active
            values = sheet.iter_rows(values_only=True)
            try:
                raw_headers = next(values)
            except StopIteration as exc:
                raise ValueError("Excel 文件为空") from exc
            headers = [str(value or "").strip().lower() for value in raw_headers]
            return _rows_from_values(headers, values)
        finally:
            workbook.close()

    if suffix not in {".csv", ".tsv", ".txt"}:
        raise ValueError("仅支持 .xlsx、.csv、.tsv 或 .txt 文件")

    delimiter = "\t" if suffix in {".tsv", ".txt"} else ","
    with path.open("r", encoding="utf-8-sig", newline="") as file:
        reader = csv.reader(file, delimiter=delimiter)
        try:
            headers = [value.strip().lower() for value in next(reader)]
        except StopIteration as exc:
            raise ValueError("输入文件为空") from exc
        return _rows_from_values(headers, reader)


def _rows_from_values(
    headers: list[str], values: Iterable[Iterable[Any]]
) -> list[dict[str, str]]:
    missing = [column for column in REQUIRED_COLUMNS if column not in headers]
    if missing:
        raise ValueError(f"缺少必需列: {', '.join(missing)}")

    indexes = {column: headers.index(column) for column in REQUIRED_COLUMNS}
    rows: list[dict[str, str]] = []
    for row_number, raw_row in enumerate(values, start=2):
        row = list(raw_row)
        if not any(value is not None and str(value).strip() for value in row):
            continue

        parsed = {
            column: str(row[index] if index < len(row) and row[index] is not None else "").strip()
            for column, index in indexes.items()
        }
        empty = [column for column, value in parsed.items() if not value]
        if empty:
            raise ValueError(f"第 {row_number} 行存在空字段: {', '.join(empty)}")
        parsed["model"] = normalize_model(parsed["model"])
        if not parsed["model"]:
            raise ValueError(f"第 {row_number} 行的模型名称处理后为空")
        rows.append(parsed)

    if not rows:
        raise ValueError("输入文件没有数据行")
    return rows


def aggregate_rows(rows: Iterable[dict[str, str]]) -> list[dict[str, Any]]:
    """Put the Nth occurrence of a model in the Nth channel for its arena."""
    arenas: OrderedDict[str, list[OrderedDict[str, str]]] = OrderedDict()
    occurrence_counts: dict[str, dict[str, int]] = defaultdict(lambda: defaultdict(int))

    for row in rows:
        arena = row["arena"]
        model = row["model"]
        channel_index = occurrence_counts[arena][model]
        occurrence_counts[arena][model] += 1

        channels = arenas.setdefault(arena, [])
        while len(channels) <= channel_index:
            channels.append(OrderedDict())
        channels[channel_index][model] = row["arn"]

    result: list[dict[str, Any]] = []
    for arena, mappings in arenas.items():
        multiple = len(mappings) > 1
        for index, mapping in enumerate(mappings, start=1):
            name = f"{CHANNEL_NAME_PREFIX} {arena}"
            if multiple:
                name += f" #{index}"
            result.append(build_payload(name, arena, mapping))
    return result


def build_payload(
    name: str, arena: str, model_mapping: OrderedDict[str, str]
) -> dict[str, Any]:
    return {
        "mode": "single",
        "channel": {
            "name": name,
            "type": AWS_CHANNEL_TYPE,
            "key": f"{AWS_ACCESS_KEY_ID}|{AWS_SECRET_ACCESS_KEY}|{arena}",
            "models": ",".join(model_mapping.keys()),
            "group": CHANNEL_GROUP,
            "model_mapping": json.dumps(
                model_mapping, ensure_ascii=False, separators=(",", ":")
            ),
            "status": 1,
            "auto_ban": 1,
            "settings": "{}",
        },
    }


def create_channel(payload: dict[str, Any]) -> None:
    headers = {"Content-Type": "application/json"}
    if COOKIE:
        headers["Cookie"] = COOKIE
    if AUTHORIZATION:
        headers["Authorization"] = AUTHORIZATION

    request = Request(
        API_URL,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers=headers,
        method="POST",
    )
    try:
        with urlopen(request, timeout=REQUEST_TIMEOUT_SECONDS) as response:
            body = response.read().decode("utf-8")
    except HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"HTTP {exc.code}: {body}") from exc
    except URLError as exc:
        raise RuntimeError(f"请求 API 失败: {exc.reason}") from exc

    try:
        result = json.loads(body)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"API 返回的不是有效 JSON: {body}") from exc
    if not result.get("success"):
        raise RuntimeError(result.get("message") or f"创建渠道失败: {body}")


def validate_config() -> None:
    if not API_URL.strip():
        raise ValueError("API_URL 不能为空")
    if not DRY_RUN:
        missing = []
        if not AWS_ACCESS_KEY_ID:
            missing.append("AWS_ACCESS_KEY_ID")
        if not AWS_SECRET_ACCESS_KEY:
            missing.append("AWS_SECRET_ACCESS_KEY")
        if not COOKIE and not AUTHORIZATION:
            missing.append("COOKIE 或 AUTHORIZATION")
        if missing:
            raise ValueError(f"请先设置全局变量: {', '.join(missing)}")


def main() -> int:
    if len(sys.argv) != 2:
        print(f"用法: {Path(sys.argv[0]).name} <channels.xlsx|csv|tsv>", file=sys.stderr)
        return 2

    try:
        validate_config()
        rows = read_rows(Path(sys.argv[1]))
        payloads = aggregate_rows(rows)
        print(f"读取 {len(rows)} 条记录，整理为 {len(payloads)} 个渠道")

        for index, payload in enumerate(payloads, start=1):
            channel = payload["channel"]
            if DRY_RUN:
                preview = json.loads(json.dumps(payload, ensure_ascii=False))
                preview["channel"]["key"] = "***|***|" + channel["key"].rsplit("|", 1)[-1]
                print(json.dumps(preview, ensure_ascii=False, indent=2))
                continue

            create_channel(payload)
            print(f"[{index}/{len(payloads)}] 已创建: {channel['name']} ({channel['models']})")
        return 0
    except (OSError, ValueError, RuntimeError) as exc:
        print(f"错误: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
