# Video Evidence Integrity

Use this reference for remote video acquisition, TikHub API evidence, OCR/ASR,
Gemini or another full-video multimodal review, and illustrated compliance
reports.

## Non-Negotiable Boundaries

- TikHub is opt-in. Call it only when the user explicitly requests TikHub or an
  explicitly requested live lookup selects it as the announced channel.
- Use only TikHub's documented direct REST API under `/api/v1/...` through
  `tools/tikhub/bin/tikhub` or `tools/tikhub/lib/tikhub_client.py`.
- Never call `mcp.tikhub.io`, a TikHub MCP server, or a TikHub MCP tool.
- API availability is not permission. Calls may consume quota or incur fees.
- Keep raw responses, media, signed URLs, model outputs, screenshots, reports,
  and logs in a gitignored local directory. Do not commit or publish them.
- Never print or report API keys, cookies, authorization headers, account
  security identifiers, cache tokens, or signed media URLs.

## Direct REST Discovery

The bundled CLI uses cached endpoint catalogs generated from TikHub's public
OpenAPI document. Inspect the catalog before calling an endpoint:

```bash
tools/tikhub/bin/tikhub list douyin fetch_one_video
tools/tikhub/bin/tikhub describe douyin douyin_app_v3_fetch_one_video_by_share_url
```

Common Douyin acquisition endpoints include:

- `douyin_app_v3_fetch_one_video_by_share_url`: resolve a share URL.
- `douyin_app_v3_fetch_one_video`: fetch item metadata by `aweme_id`.
- `douyin_app_v3_fetch_video_high_quality_play_url`: request a high-quality
  media URL when available.
- `douyin_web_fetch_one_video`: web fallback by `aweme_id`.
- `douyin_web_fetch_video_high_quality_play_url`: web high-quality fallback.

Use only cataloged endpoint names and parameters. Do not guess paths or silently
switch providers. A failed TikHub request means TikHub evidence is unavailable;
it does not mean the post or risk does not exist.

## Acquisition Workflow

1. Announce TikHub direct REST as the selected channel and note that it may be
   billed.
2. Create a gitignored evidence directory. Save each raw API response once with
   an endpoint/timestamp name; preserve failed responses instead of overwriting
   them with later retries.
3. Resolve the share URL and record the returned content ID separately from the
   user-supplied caption.
4. Fetch item metadata. Preserve the platform description, author display name,
   duration, risk flags, ad/AI fields, and media candidates as source data, not
   as verified facts about the downloaded bytes.
5. Download one reviewable media stream. Treat every media URL as confidential
   because it may contain signatures or short-lived credentials.
6. Validate the file with `ffprobe`, record duration, dimensions, codecs, byte
   size, and SHA-256, then compare several timestamps with metadata thumbnails
   or another returned stream when possible.
7. Only after validation, run `tools/analyze_video.py` and optional full-video
   multimodal review.

Do not call a preallocated file complete because its logical size matches the
expected size. A stalled range download, missing MP4 `moov` atom, failed
`ffprobe`, unexpected duration, or missing stream means the media is incomplete
and cannot be used as reviewed evidence. Label a lower-quality but complete
stream `完整审阅码流`; reserve `原始上传画质` for a separately validated source
that the API actually identifies as such.

## Source Ledger

Keep these evidence classes separate in notes and reports:

| Evidence class | What it proves | What it does not prove |
| --- | --- | --- |
| User-supplied share caption | What the user pasted | Current platform caption or actual media content |
| TikHub/platform metadata | What the API returned at capture time | That media bytes, caption, and current app page all match |
| Downloaded media | The exact reviewed bytes identified by hash | Product qualifications, price rules, rights, or current post state |
| Local frames/audio | Visible/audible evidence at recorded timecodes | Unsampled short events or external context |
| OCR/ASR | A machine-generated text hypothesis | Exact wording when confidence or audio quality is poor |
| Multimodal model output | A model observation over the submitted media | Platform truth, legal conclusion, or proof without timecode corroboration |

If the user caption, API description, thumbnails, and media disagree, report a
`来源一致性异常`. State the exact mismatch and preserve all versions. Possible
causes may include short-link remapping, dynamic creative, stale cache, API data
association, or platform changes, but keep every cause as a hypothesis. Do not
accuse the user, platform, provider, or creator of falsification without
independent evidence.

## Frame and Audio Coverage

- `tools/analyze_video.py` performs evidence preparation, not semantic review of
  every frame. Its scene detection and technical filters may decode the full
  stream, while semantic inspection still covers sampled frames.
- Report the exact sample count and strategy: opening, scene changes, periodic,
  sensitive timecodes, and ending.
- Use `关键帧/抽帧审核 + 完整视频多模态分析` when that is the actual method.
- Use `逐帧语义审核` only when every decoded frame was individually subjected
  to the claimed semantic process and the count is recorded.
- Increase sampling for high-risk, regulated, long, or fast-cut videos. Do not
  issue `Pass` when a material visual or audio surface remains unreviewed.
- Treat Whisper/OCR output as untrusted transcription until it matches audible
  speech, visible captions, or a supplied script. Preserve uncertain text as
  `不清楚/待核验`; do not repair it from context and present the repair as fact.

## Full-Video Multimodal Review

Gemini or another video-capable model is optional. Use it only when an available
configured channel is authorized for the task.

1. Submit the actual validated video MIME payload, not only contact sheets, when
   claiming full-video analysis.
2. Prompt for structured JSON with full-timeline segments, exact timecodes,
   visible text, audible text marked by confidence, risk checks, and missing
   evidence.
3. Save the raw response locally and require an explicit `success: true` or an
   equivalent successful API response.
4. Reject analyzer fallback objects such as "analysis failed, assume quality is
   good". A failed model call contributes no compliance evidence.
5. Cross-check every material finding against original frames, visible captions,
   audible speech, or another independent source. Model statements without a
   confirmable timecode remain inference.
6. Do not let a model identify a person, product qualification, medical
   condition, price truth, copyright ownership, or AI-generation status solely
   from appearance.

## Illustrated Reports

When the user requests PDF or illustrated output, include:

- scope, platform, risk level, evidence date, and non-legal-advice boundary;
- source ledger and any caption/metadata/media mismatch;
- timeline findings with timecodes and rule labels;
- contact sheets plus enlarged high-risk frames with captions;
- separate sections for confirmed observations, model observations, and items
  awaiting product-page/qualification/authorization evidence;
- acquisition coverage and limitations, including failed high-quality media;
- remediation and a release checklist.

Before delivery, visually inspect every PDF page, verify images are embedded,
check text extraction, and search the output for secrets or signed URLs. Keep
the report local unless the user explicitly authorizes publishing it.
