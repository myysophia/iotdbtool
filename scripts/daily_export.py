#!/usr/bin/env python3
"""IoTDB 每日导出并上传 OSS 的自动化脚本"""
from __future__ import annotations

import argparse
import csv
import shutil
import json
import logging
import os
import subprocess
import sys
import tarfile
from dataclasses import dataclass
from datetime import date, datetime, time, timedelta, timezone
from pathlib import Path
from typing import Dict, Iterable, List, Optional
from urllib import error, request

try:
    from zoneinfo import ZoneInfo
except ImportError:  # Python < 3.9 兼容
    from backports.zoneinfo import ZoneInfo  # type: ignore


DEFAULT_MEASUREMENTS: List[str] = [
    "STATISTICS",
    "INVERTER_INFO",
    "DEVICE_BASE_INFO",
    "DATA_COLLECTOR_BASE",
    "BATTERY",
    "BATTERY2",
    "BATTERY3",
    "BATTERY4",
]


@dataclass
class RuntimeConfig:
    """脚本运行时所需的全部配置"""

    iotdb_export_bin: str
    iotdb_host: str
    iotdb_port: str
    iotdb_username: str
    iotdb_password: str
    devices: List[str]
    measurements: List[str]
    query_timeout_ms: int
    workdir: Path
    output_dir: Path
    ossutil_bin: str
    oss_bucket: str
    oss_prefix: str
    oss_base_url: str
    wechat_webhook: Optional[str]
    timezone: ZoneInfo


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="导出 IoTDB 数据为 CSV，压缩后上传 OSS 并推送企微通知"
    )
    parser.add_argument("--iotdb-export-bin", default=os.getenv("IOTDB_EXPORT_BIN"))
    parser.add_argument("--iotdb-host", default=os.getenv("IOTDB_HOST"))
    parser.add_argument("--iotdb-port", default=os.getenv("IOTDB_PORT", "6667"))
    parser.add_argument("--iotdb-username", default=os.getenv("IOTDB_USERNAME", "root"))
    parser.add_argument("--iotdb-password", default=os.getenv("IOTDB_PASSWORD", "root"))
    parser.add_argument(
        "--devices",
        default=os.getenv(
            "IOTDB_DEVICES",
            "sn25627A000014608,sn25627A000014634",
        ),
        help="以逗号分隔的设备 SN 列表",
    )
    parser.add_argument(
        "--measurements",
        default=os.getenv("IOTDB_MEASUREMENTS"),
        help="以逗号分隔的查询节点列表，缺省使用默认配置",
    )
    parser.add_argument(
        "--query-timeout-ms",
        type=int,
        default=int(os.getenv("IOTDB_QUERY_TIMEOUT_MS", "600000")),
    )
    parser.add_argument(
        "--workdir",
        default=os.getenv("IOTDB_WORKDIR", "./tmp"),
        help="生成 SQL 与 CSV 的临时目录",
    )
    parser.add_argument(
        "--output-dir",
        default=os.getenv("IOTDB_OUTPUT_DIR", "./artifacts"),
        help="压缩包输出目录",
    )
    parser.add_argument(
        "--ossutil-bin",
        default=os.getenv("OSSUTIL_BIN", "ossutil64"),
    )
    parser.add_argument(
        "--oss-bucket",
        default=os.getenv("OSS_BUCKET", "ems-plus-demo"),
    )
    parser.add_argument(
        "--oss-prefix",
        default=os.getenv("OSS_PREFIX", "iotdbeu"),
    )
    parser.add_argument(
        "--oss-base-url",
        default=os.getenv("OSS_BASE_URL"),
        help="公开访问的 OSS 基础域名，例如 https://bucket.oss-cn-xx.aliyuncs.com",
    )
    parser.add_argument(
        "--oss-endpoint",
        default=os.getenv("OSS_ENDPOINT"),
        help="当未显式提供 base url 时可根据 endpoint 推导",
    )
    parser.add_argument(
        "--wechat-webhook",
        default=os.getenv("WECHAT_WEBHOOK"),
        help="企业微信机器人 webhook 地址",
    )
    parser.add_argument(
        "--target-date",
        default=None,
        help="YYYY-MM-DD 格式，缺省为当前日期的前一天",
    )
    parser.add_argument(
        "--log-level",
        default=os.getenv("LOG_LEVEL", "INFO"),
        help="日志等级，例如 INFO/DEBUG",
    )
    return parser.parse_args()


def load_config(args: argparse.Namespace) -> RuntimeConfig:
    tz = ZoneInfo("Asia/Shanghai")
    if not args.iotdb_export_bin:
        raise ValueError("必须通过 --iotdb-export-bin 或 IOTDB_EXPORT_BIN 指定 export-csv.sh 路径")
    if not Path(args.iotdb_export_bin).exists():
        raise FileNotFoundError(f"找不到 IoTDB 导出脚本 {args.iotdb_export_bin}")

    if not args.iotdb_host:
        raise ValueError("必须提供 IoTDB 主机名 --iotdb-host")

    devices = [d.strip() for d in args.devices.split("\n") if d.strip()] if "\n" in args.devices else [d.strip() for d in args.devices.split(",") if d.strip()]
    if not devices:
        raise ValueError("设备列表不能为空")

    if args.measurements:
        measurements = [m.strip() for m in args.measurements.split(",") if m.strip()]
    else:
        measurements = DEFAULT_MEASUREMENTS

    workdir = Path(args.workdir).resolve()
    output_dir = Path(args.output_dir).resolve()
    workdir.mkdir(parents=True, exist_ok=True)
    output_dir.mkdir(parents=True, exist_ok=True)

    oss_base_url = args.oss_base_url
    if not oss_base_url:
        if args.oss_endpoint:
            oss_base_url = f"https://{args.oss_bucket}.{args.oss_endpoint.strip('/')}"
        else:
            raise ValueError("必须提供 --oss-base-url 或 OSS_BASE_URL")

    if not oss_base_url.startswith("http"):
        raise ValueError("OSS 基础地址必须以 http/https 开头")

    return RuntimeConfig(
        iotdb_export_bin=args.iotdb_export_bin,
        iotdb_host=args.iotdb_host,
        iotdb_port=str(args.iotdb_port),
        iotdb_username=args.iotdb_username,
        iotdb_password=args.iotdb_password,
        devices=devices,
        measurements=measurements,
        query_timeout_ms=int(args.query_timeout_ms),
        workdir=workdir,
        output_dir=output_dir,
        ossutil_bin=args.ossutil_bin,
        oss_bucket=args.oss_bucket,
        oss_prefix=args.oss_prefix.strip("/"),
        oss_base_url=oss_base_url.rstrip("/"),
        wechat_webhook=args.wechat_webhook,
        timezone=tz,
    )


def determine_target_date(config: RuntimeConfig, date_str: Optional[str]) -> date:
    if date_str:
        return datetime.strptime(date_str, "%Y-%m-%d").date()
    now = datetime.now(config.timezone)
    return (now - timedelta(days=1)).date()


def calc_time_range(config: RuntimeConfig, target: date) -> Dict[str, int]:
    start_dt = datetime.combine(target, time.min, tzinfo=config.timezone)
    end_dt = start_dt + timedelta(days=1)
    start_ms = int(start_dt.timestamp() * 1000)
    end_ms = int(end_dt.timestamp() * 1000)
    return {
        "start": start_ms,
        "end": end_ms,
        "start_dt": start_dt,
        "end_dt": end_dt,
    }


def _format_local_datetime(dt: datetime, tz: ZoneInfo) -> str:
    local = dt.astimezone(tz)
    base = local.strftime("%Y-%m-%d %H:%M:%S")
    millis = local.microsecond // 1000
    if millis:
        return f"{base}.{millis:03d}"
    return base


def convert_time_to_timezone(raw_value: str, tz: ZoneInfo) -> Optional[str]:
    value = raw_value.strip()
    if not value:
        return None

    # 处理整数或浮点时间戳（默认毫秒）
    try:
        if value.startswith("-"):
            numeric_candidate = value[1:]
        else:
            numeric_candidate = value
        if numeric_candidate.isdigit():
            timestamp = int(value)
            if abs(timestamp) >= 1_000_000_000_000:  # 毫秒级
                seconds = timestamp / 1000
            else:  # 秒级
                seconds = float(timestamp)
            dt_utc = datetime.fromtimestamp(seconds, tz=timezone.utc)
            return _format_local_datetime(dt_utc, tz)
    except (ValueError, OverflowError):
        pass

    # 尝试处理浮点时间戳
    try:
        numeric_float = float(value)
        if abs(numeric_float) >= 1_000_000_000_000:  # 毫秒
            seconds = numeric_float / 1000
        else:
            seconds = numeric_float
        dt_utc = datetime.fromtimestamp(seconds, tz=timezone.utc)
        return _format_local_datetime(dt_utc, tz)
    except (ValueError, OverflowError):
        pass

    # 尝试解析 ISO 时间字符串
    candidate = value.replace("Z", "+00:00")
    if "T" not in candidate and " " in candidate:
        candidate = candidate.replace(" ", "T", 1)
    try:
        dt = datetime.fromisoformat(candidate)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        else:
            dt = dt.astimezone(timezone.utc)
        return _format_local_datetime(dt, tz)
    except ValueError:
        return None


def normalize_csv_timezone(csv_path: Path, tz: ZoneInfo) -> None:
    tmp_path = csv_path.with_suffix(csv_path.suffix + ".tmp")
    with csv_path.open("r", encoding="utf-8", newline="") as src:
        reader = csv.reader(src)
        try:
            header = next(reader)
        except StopIteration:
            return

        with tmp_path.open("w", encoding="utf-8", newline="") as dst:
            writer = csv.writer(dst)
            writer.writerow(header)
            time_idx = next(
                (idx for idx, name in enumerate(header) if name.strip().lower() == "time"),
                0,
            )
            for row in reader:
                if time_idx < len(row):
                    converted = convert_time_to_timezone(row[time_idx], tz)
                    if converted is not None:
                        row[time_idx] = converted
                writer.writerow(row)

    tmp_path.replace(csv_path)


def normalize_device_csv(device_dir: Path, tz: ZoneInfo) -> None:
    for csv_file in device_dir.glob("*.csv"):
        try:
            normalize_csv_timezone(csv_file, tz)
        except Exception as exc:  # noqa: BLE001
            logging.warning("CSV 时区转换失败 %s: %s", csv_file, exc)


def rename_device_csv(device_dir: Path, device: str, measurements: Iterable[str], target_date: date) -> None:
    day_suffix = target_date.strftime("%Y%m%d")
    measurements = list(measurements)
    for index, measurement in enumerate(measurements):
        canonical_prefix = f"root.energy.{device}.{measurement}"
        pattern = f"root.energy.{device}{index}_*.csv"
        matched_files = sorted(device_dir.glob(pattern))

        if not matched_files:
            # 若已存在符合规范的文件，跳过
            existing = sorted(device_dir.glob(f"{canonical_prefix}*.csv"))
            if existing:
                continue
            logging.warning("未找到测点 %s 对应的导出文件，匹配模式 %s", measurement, pattern)
            continue

        for seq, src in enumerate(matched_files):
            suffix = "" if seq == 0 else f"_{seq}"
            dst = device_dir / f"{canonical_prefix}{suffix}.{day_suffix}.csv"
            if dst.exists():
                dst.unlink()
            src.rename(dst)


def build_sql_content(device: str, config: RuntimeConfig, time_range: Dict[str, int]) -> str:
    start_ms = time_range["start"]
    end_ms = time_range["end"]
    prefix = f"root.energy.{device}"
    lines = []
    for measurement in config.measurements:
        lines.append(
            f"select * from {prefix}.{measurement} where time>={start_ms} and time<{end_ms};"
        )
    return "\n".join(lines) + "\n"


def run_command(cmd: List[str], env: Optional[Dict[str, str]] = None) -> None:
    logging.debug("执行命令: %s", " ".join(cmd))
    proc = subprocess.run(cmd, env=env, check=False, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    if proc.returncode != 0:
        logging.error("命令执行失败: %s", " ".join(cmd))
        logging.error("stdout: %s", proc.stdout)
        logging.error("stderr: %s", proc.stderr)
        raise RuntimeError(f"命令执行失败: {' '.join(cmd)}")
    if proc.stdout:
        logging.debug("stdout: %s", proc.stdout.strip())
    if proc.stderr:
        logging.debug("stderr: %s", proc.stderr.strip())


def export_device(device: str, config: RuntimeConfig, time_range: Dict[str, int], target_date: date) -> Path:
    logging.info("开始导出设备 %s", device)
    device_dir = config.workdir / device
    if device_dir.exists():
        shutil.rmtree(device_dir)
    device_dir.mkdir(parents=True, exist_ok=True)
    sql_file = device_dir / f"{device}.sql"
    sql_content = build_sql_content(device, config, time_range)
    sql_file.write_text(sql_content, encoding="utf-8")

    cmd = [
        config.iotdb_export_bin,
        "-h",
        config.iotdb_host,
        "-p",
        config.iotdb_port,
        "-u",
        config.iotdb_username,
        "-pw",
        config.iotdb_password,
        "-td",
        str(device_dir),
        "-s",
        str(sql_file),
        "-f",
        f"root.energy.{device}",
        "-t",
        str(config.query_timeout_ms),
    ]
    run_command(cmd)

    normalize_device_csv(device_dir, config.timezone)
    rename_device_csv(device_dir, device, config.measurements, target_date)

    archive_name = f"{device}_{target_date.strftime('%Y%m%d')}.tar.gz"
    archive_path = config.output_dir / target_date.strftime("%Y%m%d")
    archive_path.mkdir(parents=True, exist_ok=True)
    archive_path = archive_path / archive_name

    with tarfile.open(archive_path, "w:gz") as tar:
        tar.add(device_dir, arcname=device)
    logging.info("设备 %s 导出完成，压缩包: %s", device, archive_path)
    return archive_path


def upload_to_oss(local_path: Path, config: RuntimeConfig, target_date: date) -> str:
    relative_path = f"{config.oss_prefix}/{target_date.strftime('%Y%m%d')}/{local_path.name}"
    remote = f"oss://{config.oss_bucket}/{relative_path}"
    cmd = [config.ossutil_bin, "cp", "-f", str(local_path), remote]
    run_command(cmd)
    download_url = f"{config.oss_base_url}/{relative_path}"
    logging.info("已上传到 OSS: %s", download_url)
    return download_url


def post_wechat_message(config: RuntimeConfig, target_date: date, time_range: Dict[str, int], links: Dict[str, str]) -> None:
    if not config.wechat_webhook:
        logging.warning("未配置企业微信 webhook，跳过推送")
        return
    start_dt: datetime = time_range["start_dt"]  # type: ignore
    end_dt: datetime = time_range["end_dt"]  # type: ignore
    display_end = end_dt - timedelta(seconds=1)
    lines = [
        "诺瓦园区东西门一体机储能设备运行数据",
        f"> 时间范围：{start_dt.strftime('%Y-%m-%d %H:%M:%S')} ~ {display_end.strftime('%Y-%m-%d %H:%M:%S')}",
    ]
    for device, url in links.items():
        lines.append(f"> 设备 {device} 数据：[点击下载]({url})")
    content = "\n".join(lines)
    payload = {
        "msgtype": "markdown",
        "markdown": {"content": content},
    }
    req = request.Request(
        config.wechat_webhook,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
    )
    try:
        with request.urlopen(req, timeout=10) as resp:
            resp_body = resp.read().decode("utf-8")
            logging.debug("企微响应: %s", resp_body)
            data = json.loads(resp_body)
            if data.get("errcode") != 0:
                raise RuntimeError(f"企微推送失败: {data}")
    except error.URLError as exc:
        raise RuntimeError(f"企微推送失败: {exc}")
    logging.info("企业微信推送成功")


def main() -> None:
    args = parse_args()
    logging.basicConfig(
        level=getattr(logging, str(args.log_level).upper(), logging.INFO),
        format="%(asctime)s [%(levelname)s] %(message)s",
    )
    try:
        config = load_config(args)
    except Exception as exc:  # noqa: BLE001
        logging.error("加载配置失败: %s", exc)
        sys.exit(1)

    try:
        target_date = determine_target_date(config, args.target_date)
        time_range = calc_time_range(config, target_date)
        download_links: Dict[str, str] = {}
        for device in config.devices:
            archive = export_device(device, config, time_range, target_date)
            url = upload_to_oss(archive, config, target_date)
            download_links[device] = url
        post_wechat_message(config, target_date, time_range, download_links)
    except Exception as exc:  # noqa: BLE001
        logging.exception("任务执行失败: %s", exc)
        sys.exit(1)


if __name__ == "__main__":
    main()
