import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

// DIAG-12, "不自动上传诊断包": the diagnostics bundle is downloaded to the
// operator's machine and goes nowhere else. The claim is that a piece of code
// is ABSENT, which a static check states directly; a runtime test could only
// report that it happened not to observe a request this time.
//
// Closes the row of the same name in docs/development/diagnostics-acceptance-mapping.md.

// The export/save path, named file by file rather than globbed. A glob would
// quietly widen the moment someone adds a file to one of these directories for
// an unrelated reason, and the day one of them legitimately needs fetch the glob
// pushes the author toward editing the check instead of thinking about it. A
// list makes "the scope changed" visible in the diff.
export const EXPORT_PATH_FILES = [
  // Saves the bytes to disk: createObjectURL + <a download> + click.
  'apps/web/platform/content-diagnostics.ts',
  // The panel. `download` arrives as a prop; the panel issues nothing itself.
  'packages/views/content/diagnostics/index.tsx',
  // The download mutation, which calls api.contentDiagnosticDownload.
  'packages/core/content/diagnostics/queries.ts',
  // Wires the panel to the platform layer.
  'apps/web/app/[workspaceSlug]/(dashboard)/diagnostics/page.tsx',
];

// Excluded on purpose, and recorded here rather than left implicit: the download
// runs through api.contentDiagnosticDownload -> fetchRaw on the shared client.
// That is the feature working. Scanning the client would report it as a
// violation and the only way to "fix" it would be to break the download.
export const EXCLUDED = ['packages/core/api/client.ts'];

// The primitives that reach the network directly. Anything routed through the
// shared API client is same-origin by construction and is not one of these.
const PRIMITIVES = [
  {name: 'fetch', pattern: /(?<![\w.$])fetch\s*\(/},
  {name: 'XMLHttpRequest', pattern: /(?<![\w.$])XMLHttpRequest\b/},
  {name: 'sendBeacon', pattern: /\bsendBeacon\s*\(/},
  {name: 'WebSocket', pattern: /(?<![\w.$])new\s+WebSocket\s*\(/},
];

// check takes a path -> source map so the negative cases are synthetic strings
// rather than files someone has to remember to delete.
export function check(files) {
  const errors = [];
  for (const file of EXPORT_PATH_FILES) {
    const source = files[file];
    if (typeof source !== 'string') {
      // Not "nothing to scan, therefore clean". A check that cannot find its
      // files passes forever, which looks exactly like a check that works.
      errors.push(`${file}: missing from the export path scan; update EXPORT_PATH_FILES or restore the file`);
      continue;
    }
    for (const {name, pattern} of PRIMITIVES) {
      if (pattern.test(source)) {
        errors.push(`${file}: ${name} is not allowed on the export path; the diagnostics bundle is saved locally and never uploaded`);
      }
    }
  }
  return errors;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
  const files = {};
  for (const file of EXPORT_PATH_FILES) {
    const full = path.join(root, file);
    if (fs.existsSync(full)) files[file] = fs.readFileSync(full, 'utf8');
  }
  const errors = check(files);
  if (errors.length) {
    console.error(errors.join('\n'));
    process.exitCode = 1;
  } else {
    console.log(`Diagnostics export path makes no outbound request (${EXPORT_PATH_FILES.length} files checked; ${EXCLUDED.length} excluded by design)`);
  }
}
