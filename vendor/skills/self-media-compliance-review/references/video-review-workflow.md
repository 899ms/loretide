# Video Review Workflow

Read this file whenever a local or acquired video is part of the review. Also
read `video-evidence-integrity.md` before remote acquisition, OCR/ASR, or use of
a multimodal model.

## Prepare Evidence

For a local file, run:

```bash
python3 tools/analyze_video.py \
  --video <video.mp4> \
  --platform <douyin> \
  --output-dir <qa/video-evidence>
```

The tool requires `ffmpeg` and `ffprobe`. Add repeated `--text-file` arguments
for supplied scripts or subtitles. Add `--transcribe` only when local Whisper
is installed and transcription is needed. Whisper may download a model, so
tell the user before initiating a download.

The evidence package contains:

- `manifest.json`: schema version, source SHA-256, metadata, coverage, exact
  frame timecodes, audio/subtitle status, technical signals, and text prechecks;
- `review_brief.md`: evidence index and review checklist;
- opening 0-5 second, scene-change, periodic, and ending frames;
- contact sheets for triage and original frames for confirmation;
- mono 16 kHz review audio when an audio stream exists;
- embedded or sidecar subtitle references and optional local transcription.

The manifest conforms to `../schemas/video-evidence-manifest.schema.json`.
Automated black-screen, silence, and text matches are precheck signals. The tool
intentionally leaves the verdict as `not_assigned`.

## Review Evidence

1. Read `manifest.json` and `review_brief.md`.
2. Inspect every contact sheet, then open original frames for every suspected
   timecode. Check the cover, first frame, opening 0-5 seconds, scene changes,
   product and CTA segments, sensitive visuals, and ending.
3. Review audible speech and BGM using confirmed audio, a supplied transcript,
   or optional local transcription. Subtitles do not prove the voiceover is
   identical.
4. Review visible subtitles, stickers, watermarks, QR codes, contacts, product
   claims, before/after imagery, AI labels, and privacy identifiers.
5. Map each finding to an exact timecode or frame and a platform rule. Give a
   concrete edit.
6. Increase sampling or inspect the full timeline for long, fast-cut, high-risk,
   or regulated-domain material.

Frame sampling can miss brief content. If any audible or visible region remains
unreviewed, put it in `待核验` and do not assign `Pass`. Never describe sampled
keyframes as full-frame semantic review.

## Remote and Model Evidence

Use only an available, authorized channel to acquire remote media. If the media
cannot be accessed, state that it was not reviewed and request a local file,
transcript, or screenshots. Never infer `Pass` from an inaccessible URL.

When the user authorizes Gemini or another multimodal model, submit a validated
reviewable video, preserve the successful raw structured response locally, and
confirm claimed timecodes against original frames, visible subtitles, or
reviewable audio. A failed call, fallback value, partial download, or model-only
claim is not evidence.

Maintain a source ledger separating share copy, platform metadata, acquired
media, local observations, OCR/ASR, and model output. Report mismatches as
`待核验`; do not infer manipulation or decide which source is true without
independent evidence.

## Delivery Integration

Keep frames, audio, transcripts, and manifests under `qa/` or `internal/`.
Store the report at `qa/compliance_review.md` when the project has a release
package. Record its path and final risk level in handoff notes. Do not treat a
video as final while an unresolved `Blocker` or unaccepted `High` remains.
