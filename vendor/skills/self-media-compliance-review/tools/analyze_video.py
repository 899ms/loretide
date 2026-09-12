#!/usr/bin/env python3
"""Prepare traceable local video evidence for an agent compliance review."""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import math
import re
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Any

try:
    from extract_claims import extract_claims
except ModuleNotFoundError:
    from tools.extract_claims import extract_claims


class VideoAnalysisError(RuntimeError):
    """Raised when the evidence package cannot be prepared safely."""


RISK_ORDER = {"pass": 0, "low": 1, "medium": 2, "high": 3, "blocker": 4}
FRAME_SCALE = "scale=1280:-2:force_original_aspect_ratio=decrease"
SUPPORTED_SIDECARS = (".srt", ".vtt", ".ass", ".ssa", ".txt")


def _run(command: list[str], timeout: int, check: bool = True) -> subprocess.CompletedProcess[str]:
    try:
        completed = subprocess.run(
            command,
            check=False,
            capture_output=True,
            text=True,
            timeout=timeout,
        )
    except subprocess.TimeoutExpired as exc:
        raise VideoAnalysisError(f"command timed out after {timeout}s: {command[0]}") from exc
    if check and completed.returncode != 0:
        message = (completed.stderr or completed.stdout or "unknown error").strip()
        raise VideoAnalysisError(f"{command[0]} failed: {message[-1200:]}")
    return completed


def _require_tool(name: str) -> str:
    path = shutil.which(name)
    if not path:
        raise VideoAnalysisError(f"missing required executable: {name}")
    return path


def _timecode(seconds: float) -> str:
    milliseconds = max(0, round(seconds * 1000))
    hours, remainder = divmod(milliseconds, 3_600_000)
    minutes, remainder = divmod(remainder, 60_000)
    secs, millis = divmod(remainder, 1000)
    return f"{hours:02d}:{minutes:02d}:{secs:02d}.{millis:03d}"


def _parse_rate(value: str | None) -> float:
    if not value or value in {"0/0", "N/A"}:
        return 0.0
    if "/" in value:
        numerator, denominator = value.split("/", 1)
        try:
            return float(numerator) / float(denominator)
        except (ValueError, ZeroDivisionError):
            return 0.0
    try:
        return float(value)
    except ValueError:
        return 0.0


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def probe_video(video_path: Path, ffprobe: str, timeout: int) -> dict[str, Any]:
    completed = _run(
        [ffprobe, "-v", "error", "-show_format", "-show_streams", "-of", "json", str(video_path)],
        timeout,
    )
    try:
        payload = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise VideoAnalysisError(f"ffprobe returned invalid JSON: {exc}") from exc

    streams = payload.get("streams") or []
    video_streams = [stream for stream in streams if stream.get("codec_type") == "video"]
    audio_streams = [stream for stream in streams if stream.get("codec_type") == "audio"]
    subtitle_streams = [stream for stream in streams if stream.get("codec_type") == "subtitle"]
    if not video_streams:
        raise VideoAnalysisError("input contains no video stream")

    primary = video_streams[0]
    format_info = payload.get("format") or {}
    raw_duration = format_info.get("duration") or primary.get("duration") or 0
    try:
        duration = float(raw_duration)
    except (TypeError, ValueError):
        duration = 0.0
    if duration <= 0:
        raise VideoAnalysisError("could not determine a positive video duration")

    width = int(primary.get("width") or 0)
    height = int(primary.get("height") or 0)
    return {
        "duration_seconds": round(duration, 3),
        "duration_timecode": _timecode(duration),
        "format_name": format_info.get("format_name", ""),
        "size_bytes": int(format_info.get("size") or video_path.stat().st_size),
        "bit_rate": int(format_info.get("bit_rate") or 0),
        "video": {
            "codec": primary.get("codec_name", ""),
            "width": width,
            "height": height,
            "aspect_ratio": round(width / height, 4) if height else 0,
            "fps": round(_parse_rate(primary.get("avg_frame_rate") or primary.get("r_frame_rate")), 3),
            "pixel_format": primary.get("pix_fmt", ""),
            "rotation": (primary.get("tags") or {}).get("rotate", "0"),
        },
        "audio_streams": [
            {
                "index": stream.get("index"),
                "codec": stream.get("codec_name", ""),
                "channels": stream.get("channels", 0),
                "sample_rate": stream.get("sample_rate", ""),
                "language": (stream.get("tags") or {}).get("language", ""),
            }
            for stream in audio_streams
        ],
        "subtitle_streams": [
            {
                "index": stream.get("index"),
                "codec": stream.get("codec_name", ""),
                "language": (stream.get("tags") or {}).get("language", ""),
            }
            for stream in subtitle_streams
        ],
    }


def _dedupe_times(items: list[tuple[float, str]], duration: float, tolerance: float = 0.35) -> list[tuple[float, str]]:
    output: list[tuple[float, str]] = []
    for seconds, kind in sorted(items, key=lambda item: (item[0], item[1])):
        seconds = min(max(0.0, seconds), max(0.0, duration - 0.05))
        if any(abs(seconds - existing) <= tolerance for existing, _kind in output):
            continue
        output.append((seconds, kind))
    return output


def periodic_times(duration: float, interval: float, max_frames: int) -> list[float]:
    if max_frames <= 0:
        return []
    if interval <= 0:
        if duration <= 60:
            interval = 5
        elif duration <= 300:
            interval = 15
        else:
            interval = max(30, duration / max_frames)
    values = []
    current = interval
    while current < duration - 0.5 and len(values) < max_frames:
        values.append(current)
        current += interval
    return values


def detect_scene_times(
    video_path: Path,
    ffmpeg: str,
    threshold: float,
    max_scenes: int,
    timeout: int,
) -> tuple[list[float], list[str]]:
    if max_scenes <= 0:
        return [], []
    command = [
        ffmpeg,
        "-hide_banner",
        "-loglevel",
        "info",
        "-i",
        str(video_path),
        "-map",
        "0:v:0",
        "-vf",
        f"select='gt(scene,{threshold})',showinfo",
        "-an",
        "-f",
        "null",
        "-",
    ]
    completed = _run(command, timeout, check=False)
    if completed.returncode != 0:
        message = (completed.stderr or "scene detection failed").strip().splitlines()
        return [], [message[-1] if message else "scene detection failed"]
    values = []
    for match in re.finditer(r"pts_time:([0-9]+(?:\.[0-9]+)?)", completed.stderr):
        value = float(match.group(1))
        if not values or abs(value - values[-1]) > 0.35:
            values.append(value)
    if len(values) > max_scenes:
        step = len(values) / max_scenes
        values = [values[min(len(values) - 1, math.floor(index * step))] for index in range(max_scenes)]
    return values, []


def extract_frame(video_path: Path, output_path: Path, seconds: float, ffmpeg: str, timeout: int) -> None:
    _run(
        [
            ffmpeg,
            "-hide_banner",
            "-loglevel",
            "error",
            "-ss",
            f"{seconds:.3f}",
            "-i",
            str(video_path),
            "-map",
            "0:v:0",
            "-frames:v",
            "1",
            "-vf",
            FRAME_SCALE,
            "-q:v",
            "2",
            "-y",
            str(output_path),
        ],
        timeout,
    )
    if not output_path.is_file() or output_path.stat().st_size == 0:
        raise VideoAnalysisError(f"frame extraction produced no image at {_timecode(seconds)}")


def extract_frames(
    video_path: Path,
    output_dir: Path,
    duration: float,
    scene_times: list[float],
    periodic_interval: float,
    max_frames: int,
    ffmpeg: str,
    timeout: int,
) -> list[dict[str, Any]]:
    opening = [(value, "opening") for value in (0.0, 1.0, 3.0, 5.0) if value < duration]
    ending = [(max(0.0, duration - 0.5), "ending")]
    periodic = [(value, "periodic") for value in periodic_times(duration, periodic_interval, max_frames)]
    scenes = [(value, "scene") for value in scene_times]
    samples = _dedupe_times(opening + scenes + periodic + ending, duration)
    if len(samples) > max_frames:
        mandatory = [item for item in samples if item[1] in {"opening", "ending"}]
        optional = [item for item in samples if item[1] not in {"opening", "ending"}]
        remaining = max(0, max_frames - len(mandatory))
        if len(optional) > remaining and remaining:
            step = len(optional) / remaining
            optional = [optional[min(len(optional) - 1, math.floor(index * step))] for index in range(remaining)]
        samples = _dedupe_times(mandatory + optional[:remaining], duration)

    frames_dir = output_dir / "frames"
    frames_dir.mkdir()
    frames = []
    for seconds, kind in samples:
        millis = round(seconds * 1000)
        filename = f"frame_{millis:010d}_{kind}.jpg"
        frame_path = frames_dir / filename
        extract_frame(video_path, frame_path, seconds, ffmpeg, timeout)
        frames.append(
            {
                "kind": kind,
                "seconds": round(seconds, 3),
                "timecode": _timecode(seconds),
                "path": frame_path.relative_to(output_dir).as_posix(),
            }
        )
    return frames


def create_contact_sheets(
    output_dir: Path,
    frames: list[dict[str, Any]],
    ffmpeg: str,
    timeout: int,
) -> tuple[list[dict[str, Any]], list[str]]:
    frame_count = len(frames)
    if frame_count <= 0:
        return [], []
    sheet_dir = output_dir / "contact_sheets"
    sheet_dir.mkdir()
    sheet_pattern = sheet_dir / "sheet_%02d.jpg"
    columns = min(4, frame_count)
    rows = min(4, math.ceil(min(frame_count, 16) / columns))
    cells_per_sheet = columns * rows
    sheet_count = math.ceil(frame_count / cells_per_sheet)
    command = [
        ffmpeg,
        "-hide_banner",
        "-loglevel",
        "error",
        "-pattern_type",
        "glob",
        "-i",
        str(output_dir / "frames" / "frame_*.jpg"),
        "-vf",
        "scale=320:320:force_original_aspect_ratio=decrease,"
        "pad=320:360:(ow-iw)/2:(oh-ih)/2:color=white,"
        f"tile={columns}x{rows}:padding=4:margin=4",
        "-frames:v",
        str(sheet_count),
        "-q:v",
        "2",
        "-y",
        str(sheet_pattern),
    ]
    completed = _run(command, timeout, check=False)
    warnings = []
    if completed.returncode != 0:
        warnings.append(f"contact sheet creation failed: {(completed.stderr or '').strip()[-500:]}")
    sheets = []
    for sheet_index, path in enumerate(sorted(sheet_dir.glob("sheet_*.jpg"))):
        start = sheet_index * cells_per_sheet
        cells = []
        for cell_index, frame in enumerate(frames[start : start + cells_per_sheet], 1):
            cells.append(
                {
                    "cell": cell_index,
                    "timecode": frame["timecode"],
                    "kind": frame["kind"],
                    "frame_path": frame["path"],
                }
            )
        sheets.append(
            {
                "path": path.relative_to(output_dir).as_posix(),
                "grid": f"{columns}x{rows}",
                "cells": cells,
            }
        )
    return sheets, warnings


def _parse_intervals(text: str, prefix: str) -> list[dict[str, Any]]:
    starts = [float(value) for value in re.findall(fr"{prefix}_start:([0-9]+(?:\.[0-9]+)?)", text)]
    ends = [float(value) for value in re.findall(fr"{prefix}_end:([0-9]+(?:\.[0-9]+)?)", text)]
    intervals = []
    for index, start in enumerate(starts):
        if index >= len(ends):
            break
        end = ends[index]
        intervals.append(
            {
                "start_seconds": round(start, 3),
                "end_seconds": round(end, 3),
                "duration_seconds": round(max(0.0, end - start), 3),
                "start_timecode": _timecode(start),
                "end_timecode": _timecode(end),
            }
        )
    return intervals


def detect_technical_signals(
    video_path: Path,
    has_audio: bool,
    ffmpeg: str,
    timeout: int,
) -> tuple[dict[str, Any], list[str]]:
    warnings = []
    black = _run(
        [
            ffmpeg,
            "-hide_banner",
            "-loglevel",
            "info",
            "-i",
            str(video_path),
            "-map",
            "0:v:0",
            "-vf",
            "blackdetect=d=0.5:pic_th=0.98",
            "-an",
            "-f",
            "null",
            "-",
        ],
        timeout,
        check=False,
    )
    if black.returncode != 0:
        warnings.append("black-frame detection did not complete")
    black_intervals = _parse_intervals(black.stderr, "black") if black.returncode == 0 else []

    silence_intervals: list[dict[str, Any]] = []
    if has_audio:
        silence = _run(
            [
                ffmpeg,
                "-hide_banner",
                "-loglevel",
                "info",
                "-i",
                str(video_path),
                "-map",
                "0:a:0",
                "-af",
                "silencedetect=noise=-45dB:d=1.0",
                "-vn",
                "-f",
                "null",
                "-",
            ],
            timeout,
            check=False,
        )
        if silence.returncode != 0:
            warnings.append("silence detection did not complete")
        else:
            silence_intervals = _parse_intervals(silence.stderr, "silence")
    return {"black_intervals": black_intervals, "silence_intervals": silence_intervals}, warnings


def extract_audio(video_path: Path, output_dir: Path, ffmpeg: str, timeout: int) -> Path:
    audio_path = output_dir / "audio.wav"
    _run(
        [
            ffmpeg,
            "-hide_banner",
            "-loglevel",
            "error",
            "-i",
            str(video_path),
            "-map",
            "0:a:0",
            "-vn",
            "-ac",
            "1",
            "-ar",
            "16000",
            "-c:a",
            "pcm_s16le",
            "-y",
            str(audio_path),
        ],
        timeout,
    )
    return audio_path


def extract_embedded_subtitle(video_path: Path, output_dir: Path, ffmpeg: str, timeout: int) -> tuple[Path | None, str | None]:
    subtitle_path = output_dir / "embedded_subtitles.srt"
    completed = _run(
        [
            ffmpeg,
            "-hide_banner",
            "-loglevel",
            "error",
            "-i",
            str(video_path),
            "-map",
            "0:s:0",
            "-c:s",
            "srt",
            "-y",
            str(subtitle_path),
        ],
        timeout,
        check=False,
    )
    if completed.returncode != 0 or not subtitle_path.is_file():
        subtitle_path.unlink(missing_ok=True)
        return None, f"embedded subtitle extraction failed: {(completed.stderr or '').strip()[-500:]}"
    return subtitle_path, None


def find_sidecars(video_path: Path) -> list[Path]:
    output = []
    for suffix in SUPPORTED_SIDECARS:
        candidate = video_path.with_suffix(suffix)
        if candidate.is_file():
            output.append(candidate.resolve())
    return output


def run_whisper(
    audio_path: Path,
    output_dir: Path,
    model: str,
    language: str,
    timeout: int,
) -> tuple[Path | None, str | None]:
    whisper = shutil.which("whisper")
    if not whisper:
        return None, "Whisper transcription requested but `whisper` is not installed"
    transcript_dir = output_dir / "transcript"
    transcript_dir.mkdir()
    command = [
        whisper,
        str(audio_path),
        "--model",
        model,
        "--output_dir",
        str(transcript_dir),
        "--output_format",
        "all",
        "--verbose",
        "False",
        "--fp16",
        "False",
    ]
    if language and language != "auto":
        command.extend(["--language", language])
    completed = _run(command, timeout, check=False)
    transcript_path = transcript_dir / f"{audio_path.stem}.srt"
    if completed.returncode != 0 or not transcript_path.is_file():
        return None, f"Whisper transcription failed: {(completed.stderr or completed.stdout or '').strip()[-800:]}"
    return transcript_path, None


def _subtitle_seconds(value: str) -> float:
    normalized = value.strip().replace(",", ".")
    parts = normalized.split(":")
    try:
        if len(parts) == 3:
            return int(parts[0]) * 3600 + int(parts[1]) * 60 + float(parts[2])
        if len(parts) == 2:
            return int(parts[0]) * 60 + float(parts[1])
    except ValueError:
        return 0.0
    return 0.0


def parse_timed_text(path: Path) -> list[dict[str, Any]]:
    text = path.read_text(encoding="utf-8", errors="replace").replace("\r\n", "\n")
    if path.suffix.lower() in {".srt", ".vtt"}:
        segments = []
        for block in re.split(r"\n\s*\n", text):
            lines = [line.strip() for line in block.splitlines() if line.strip()]
            timing_index = next((index for index, line in enumerate(lines) if "-->" in line), None)
            if timing_index is None:
                continue
            start_raw, end_raw = [value.strip().split()[0] for value in lines[timing_index].split("-->", 1)]
            content = " ".join(lines[timing_index + 1 :])
            content = re.sub(r"<[^>]+>", "", content).strip()
            if content:
                segments.append(
                    {
                        "start_seconds": round(_subtitle_seconds(start_raw), 3),
                        "end_seconds": round(_subtitle_seconds(end_raw), 3),
                        "text": content,
                    }
                )
        return segments
    if path.suffix.lower() in {".ass", ".ssa"}:
        segments = []
        for line in text.splitlines():
            if not line.startswith("Dialogue:"):
                continue
            fields = line.split(",", 9)
            if len(fields) < 10:
                continue
            content = re.sub(r"\{[^}]*\}", "", fields[9]).replace("\\N", " ").strip()
            if content:
                segments.append(
                    {
                        "start_seconds": round(_subtitle_seconds(fields[1]), 3),
                        "end_seconds": round(_subtitle_seconds(fields[2]), 3),
                        "text": content,
                    }
                )
        return segments
    plain = re.sub(r"\s+", " ", text).strip()
    return [{"start_seconds": 0.0, "end_seconds": 0.0, "text": plain}] if plain else []


def scan_text_sources(sources: list[dict[str, Any]]) -> dict[str, Any]:
    findings = []
    segment_count = 0
    for source in sources:
        path = Path(source["absolute_path"])
        segments = parse_timed_text(path)
        segment_count += len(segments)
        for segment in segments:
            claims = extract_claims(segment["text"])
            for category, items in claims.items():
                for item in items:
                    findings.append(
                        {
                            "category": category,
                            "severity": item["risk"],
                            "timecode": _timecode(segment["start_seconds"]),
                            "text": item["text"],
                            "reason": item["reason"],
                            "fix": item["fix"],
                            "source": source["label"],
                        }
                    )
    findings.sort(key=lambda item: (-RISK_ORDER.get(item["severity"], 0), item["timecode"], item["text"]))
    highest = max((item["severity"] for item in findings), key=lambda value: RISK_ORDER.get(value, 0), default="pass")
    return {
        "highest_text_signal": highest,
        "segment_count": segment_count,
        "finding_count": len(findings),
        "findings": findings,
        "limitation": "文本命中仅为预筛信号；必须结合上下文、画面、声音和平台规则复核。",
    }


def _public_text_source(path: Path, label: str, output_dir: Path) -> dict[str, str]:
    try:
        display_path = path.relative_to(output_dir).as_posix()
    except ValueError:
        display_path = str(path)
    return {"label": label, "path": display_path, "absolute_path": str(path)}


def render_review_brief(manifest: dict[str, Any]) -> str:
    coverage = manifest["coverage"]
    lines = [
        "# 视频合规审核证据包",
        "",
        f"- 视频: `{manifest['source']['path']}`",
        f"- 目标平台: {' / '.join(manifest['target_platforms']) or '未指定'}",
        f"- SHA-256: `{manifest['source']['sha256']}`",
        f"- 时长: {manifest['metadata']['duration_timecode']}",
        f"- 抽样帧: {coverage['frame_count']} 张",
        f"- 音频: {'已提取' if coverage['audio_extracted'] else '无音轨或提取失败'}",
        f"- 字幕/转写: {'已覆盖' if coverage['text_review_available'] else '未覆盖'}",
        "- 结论状态: 待 agent 查看证据后判断",
        "",
        "## 必审证据",
        "",
    ]
    for frame in manifest["evidence"]["frames"]:
        lines.append(f"- `{frame['timecode']}` [{frame['kind']}] `{frame['path']}`")
    lines.extend(["", "## Contact sheets", ""])
    for sheet in manifest["evidence"]["contact_sheets"]:
        mapping = " / ".join(
            f"cell {cell['cell']}={cell['timecode']} [{cell['kind']}]" for cell in sheet["cells"]
        )
        lines.append(f"- `{sheet['path']}` ({sheet['grid']}): {mapping}")
    if not manifest["evidence"]["contact_sheets"]:
        lines.append("- Contact sheet 生成失败；请逐张检查原帧。")
    lines.extend(["", "## 文本预筛", ""])
    precheck = manifest["text_precheck"]
    lines.append(f"- 最高文本信号: {precheck['highest_text_signal']}")
    lines.append(f"- 命中数量: {precheck['finding_count']}")
    for finding in precheck["findings"][:30]:
        lines.append(
            f"- {finding['severity']} `{finding['timecode']}` {finding['reason']}: "
            f"`{finding['text']}`；建议: {finding['fix']}"
        )
    if not precheck["findings"]:
        lines.append("- 未命中文本规则；这不等于视频通过审核。")
    lines.extend(["", "## 技术信号", ""])
    lines.append(f"- 黑屏区间: {len(manifest['technical_signals']['black_intervals'])}")
    lines.append(f"- 静音区间: {len(manifest['technical_signals']['silence_intervals'])}")
    lines.extend(["", "## 待核验", ""])
    for item in manifest["pending_verification"]:
        lines.append(f"- {item}")
    lines.extend(
        [
            "",
            "## Agent 审核要求",
            "",
            "- 查看全部 contact sheet，并按 manifest 时间点复查疑似风险原帧。",
            "- 同时审核可听口播和字幕；只有字幕而未听音频时，不得认为音频已通过。",
            "- 检查封面/首 0-5 秒、主要场景、CTA/商品段和结尾。",
            "- 将每个风险映射到平台规则，并给出时间点、原因和可执行修改建议。",
            "- 未覆盖完整画面或声音时，不得输出 Pass；应标记待核验和覆盖限制。",
        ]
    )
    return "\n".join(lines) + "\n"


def analyze_video(args: argparse.Namespace) -> dict[str, Any]:
    video_path = Path(args.video).expanduser().resolve()
    if not video_path.is_file():
        raise VideoAnalysisError(f"video file not found: {video_path}")
    ffmpeg = _require_tool("ffmpeg")
    ffprobe = _require_tool("ffprobe")

    output_dir = Path(args.output_dir).expanduser().resolve()
    if output_dir.exists():
        if not output_dir.is_dir():
            raise VideoAnalysisError(f"output path is not a directory: {output_dir}")
        if any(output_dir.iterdir()):
            raise VideoAnalysisError(f"output directory is not empty: {output_dir}")
    output_dir.mkdir(parents=True, exist_ok=True)

    warnings: list[str] = []
    metadata = probe_video(video_path, ffprobe, args.command_timeout)
    duration = metadata["duration_seconds"]
    scene_times, scene_warnings = detect_scene_times(
        video_path,
        ffmpeg,
        args.scene_threshold,
        args.max_scene_frames,
        args.command_timeout,
    )
    warnings.extend(scene_warnings)
    frames = extract_frames(
        video_path,
        output_dir,
        duration,
        scene_times,
        args.periodic_seconds,
        args.max_frames,
        ffmpeg,
        args.command_timeout,
    )
    contact_sheets, sheet_warnings = create_contact_sheets(output_dir, frames, ffmpeg, args.command_timeout)
    warnings.extend(sheet_warnings)

    has_audio = bool(metadata["audio_streams"])
    audio_path = None
    if has_audio:
        try:
            audio_path = extract_audio(video_path, output_dir, ffmpeg, args.command_timeout)
        except VideoAnalysisError as exc:
            warnings.append(str(exc))

    technical_signals, technical_warnings = detect_technical_signals(
        video_path, has_audio, ffmpeg, args.command_timeout
    )
    warnings.extend(technical_warnings)

    text_sources: list[dict[str, str]] = []
    seen_text_paths: set[Path] = set()

    def add_text_source(path: Path, label: str) -> None:
        resolved = path.expanduser().resolve()
        if not resolved.is_file():
            raise VideoAnalysisError(f"text evidence file not found: {resolved}")
        if resolved in seen_text_paths:
            return
        seen_text_paths.add(resolved)
        text_sources.append(_public_text_source(resolved, label, output_dir))

    for index, raw_path in enumerate(args.text_file or [], 1):
        add_text_source(Path(raw_path), f"provided-text-{index}")

    sidecars = find_sidecars(video_path)
    for index, sidecar in enumerate(sidecars, 1):
        add_text_source(sidecar, f"sidecar-{index}")

    embedded_subtitle = None
    if metadata["subtitle_streams"]:
        embedded_subtitle, subtitle_warning = extract_embedded_subtitle(
            video_path, output_dir, ffmpeg, args.command_timeout
        )
        if subtitle_warning:
            warnings.append(subtitle_warning)
        if embedded_subtitle:
            add_text_source(embedded_subtitle, "embedded-subtitle")

    transcript_path = None
    if args.transcribe:
        if audio_path:
            transcript_path, whisper_warning = run_whisper(
                audio_path,
                output_dir,
                args.whisper_model,
                args.language,
                args.transcription_timeout,
            )
            if whisper_warning:
                warnings.append(whisper_warning)
            if transcript_path:
                add_text_source(transcript_path, "whisper-transcript")
        else:
            warnings.append("transcription requested but no audio track was extracted")

    text_precheck = scan_text_sources(text_sources)
    text_review_available = text_precheck["segment_count"] > 0
    pending = [
        "完整视频画面仍需由具备视觉能力的 agent 复核；抽样可能漏掉极短风险画面。",
        "版权、肖像、隐私、商标、素材授权与账号资质无法仅凭视频文件确认。",
        "标题、发布文案、评论区引导、商品链接和账号主页未包含在视频文件中，需另行提供。",
    ]
    if has_audio and not transcript_path:
        pending.append("视频有音轨但未生成 Whisper 转写；必须直接听取音频或提供完整口播稿。")
    if has_audio:
        pending.append("BGM、音效、语气以及字幕/转写与实际音频是否一致，仍需完成听觉复核。")
    if not text_review_available:
        pending.append("未取得字幕或转写文本，口播和屏幕文字风险尚未完成文本审核。")
    if not has_audio:
        pending.append("视频没有可识别音轨；需确认静音是否符合发布预期。")

    manifest = {
        "schema_version": "1.0",
        "analysis_type": "video_compliance_evidence",
        "generated_at": dt.datetime.now(dt.timezone.utc).astimezone().isoformat(timespec="seconds"),
        "target_platforms": args.platform or [],
        "source": {
            "path": str(video_path),
            "sha256": _sha256(video_path),
        },
        "metadata": metadata,
        "coverage": {
            "frame_sampling": "opening + scene changes + periodic + ending",
            "frame_count": len(frames),
            "contact_sheet_count": len(contact_sheets),
            "audio_present": has_audio,
            "audio_extracted": bool(audio_path),
            "embedded_subtitle_present": bool(metadata["subtitle_streams"]),
            "provided_text_count": len(args.text_file or []),
            "sidecar_subtitle_count": len(sidecars),
            "whisper_requested": bool(args.transcribe),
            "whisper_completed": bool(transcript_path),
            "text_review_available": text_review_available,
        },
        "evidence": {
            "frames": frames,
            "contact_sheets": contact_sheets,
            "audio": audio_path.relative_to(output_dir).as_posix() if audio_path else "",
            "text_sources": [
                {"label": item["label"], "path": item["path"]} for item in text_sources
            ],
        },
        "technical_signals": technical_signals,
        "text_precheck": text_precheck,
        "warnings": warnings,
        "pending_verification": pending,
        "verdict": {
            "status": "not_assigned",
            "reason": "The evidence-preparation tool does not issue a final compliance verdict.",
        },
    }
    manifest_path = output_dir / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (output_dir / "review_brief.md").write_text(render_review_brief(manifest), encoding="utf-8")
    return {
        "status": "evidence_ready",
        "output_dir": str(output_dir),
        "manifest": str(manifest_path),
        "review_brief": str(output_dir / "review_brief.md"),
        "frame_count": len(frames),
        "text_finding_count": text_precheck["finding_count"],
        "warnings": warnings,
        "verdict": "not_assigned",
    }


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Prepare frames, audio, subtitles, technical signals, and text prechecks for video compliance review."
    )
    parser.add_argument("--video", required=True, help="Local video file path")
    parser.add_argument("--output-dir", required=True, help="New or empty directory for the evidence package")
    parser.add_argument("--platform", action="append", default=[], help="Target platform; repeat for multiple platforms")
    parser.add_argument(
        "--text-file",
        action="append",
        default=[],
        help="Optional transcript/subtitle/script file; repeat for multiple files",
    )
    parser.add_argument("--max-frames", type=int, default=24)
    parser.add_argument("--max-scene-frames", type=int, default=10)
    parser.add_argument("--scene-threshold", type=float, default=0.35)
    parser.add_argument("--periodic-seconds", type=float, default=0.0, help="0 selects an interval automatically")
    parser.add_argument("--transcribe", action="store_true", help="Run the optional local Whisper CLI")
    parser.add_argument("--whisper-model", default="tiny")
    parser.add_argument("--language", default="auto")
    parser.add_argument("--command-timeout", type=int, default=900)
    parser.add_argument("--transcription-timeout", type=int, default=3600)
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    if args.max_frames < 5:
        parser.error("--max-frames must be at least 5 so opening and ending evidence are retained")
    if not 0 < args.scene_threshold < 1:
        parser.error("--scene-threshold must be between 0 and 1")
    try:
        summary = analyze_video(args)
    except VideoAnalysisError as exc:
        print(json.dumps({"status": "error", "error": str(exc)}, ensure_ascii=False), file=sys.stderr)
        return 2
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
