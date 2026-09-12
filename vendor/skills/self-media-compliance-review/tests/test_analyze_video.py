import json
import shutil
import subprocess
from pathlib import Path

import pytest

from tools.analyze_video import VideoAnalysisError, analyze_video, build_parser, parse_timed_text


def _make_test_video(path: Path, *, with_audio: bool) -> None:
    command = [
        "ffmpeg",
        "-hide_banner",
        "-loglevel",
        "error",
        "-f",
        "lavfi",
        "-i",
        "color=c=black:s=320x240:d=2:r=12",
    ]
    if with_audio:
        command.extend(["-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-shortest"])
    command.extend(["-c:v", "mpeg4"])
    if with_audio:
        command.extend(["-c:a", "aac"])
    command.extend(["-y", str(path)])
    subprocess.run(command, check=True, capture_output=True)


def test_parse_timed_text_preserves_timecodes(tmp_path):
    subtitle = tmp_path / "sample.srt"
    subtitle.write_text(
        "1\n00:00:01,250 --> 00:00:02,500\n全网最低\n\n",
        encoding="utf-8",
    )

    assert parse_timed_text(subtitle) == [
        {"start_seconds": 1.25, "end_seconds": 2.5, "text": "全网最低"}
    ]


@pytest.mark.skipif(
    not shutil.which("ffmpeg") or not shutil.which("ffprobe"),
    reason="ffmpeg and ffprobe are required",
)
def test_analyze_video_builds_traceable_evidence_without_assigning_verdict(tmp_path):
    video = tmp_path / "sample.mp4"
    output = tmp_path / "evidence"
    subtitle = tmp_path / "sample.srt"
    _make_test_video(video, with_audio=True)
    subtitle.write_text(
        "1\n00:00:00,200 --> 00:00:01,200\n这是全网最低价\n\n",
        encoding="utf-8",
    )
    args = build_parser().parse_args(
        [
            "--video",
            str(video),
            "--output-dir",
            str(output),
            "--platform",
            "xiaohongshu",
            "--text-file",
            str(subtitle),
        ]
    )

    summary = analyze_video(args)
    manifest = json.loads((output / "manifest.json").read_text())

    assert summary["status"] == "evidence_ready"
    assert summary["verdict"] == "not_assigned"
    assert manifest["verdict"]["status"] == "not_assigned"
    assert manifest["target_platforms"] == ["xiaohongshu"]
    assert len(manifest["source"]["sha256"]) == 64
    assert manifest["coverage"]["audio_present"] is True
    assert manifest["coverage"]["audio_extracted"] is True
    assert manifest["coverage"]["frame_count"] >= 2
    assert manifest["text_precheck"]["finding_count"] >= 1
    assert any("音频" in item or "BGM" in item for item in manifest["pending_verification"])
    assert (output / "review_brief.md").is_file()


@pytest.mark.skipif(
    not shutil.which("ffmpeg") or not shutil.which("ffprobe"),
    reason="ffmpeg and ffprobe are required",
)
def test_analyze_video_refuses_nonempty_output_directory(tmp_path):
    video = tmp_path / "sample.mp4"
    output = tmp_path / "evidence"
    output.mkdir()
    (output / "keep.txt").write_text("do not overwrite")
    _make_test_video(video, with_audio=False)
    args = build_parser().parse_args(
        ["--video", str(video), "--output-dir", str(output)]
    )

    with pytest.raises(VideoAnalysisError, match="output directory is not empty"):
        analyze_video(args)

