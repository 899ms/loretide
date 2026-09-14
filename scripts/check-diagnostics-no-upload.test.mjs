// Tests for scripts/check-diagnostics-no-upload.mjs.
//
// Case -> requirement map (specs/010-diag-evidence-gaps/spec.md):
//   the repository's own export path is clean ............ FR-011, SC-003a
//   a fetch to an external host is caught ................ FR-011, FR-011b, SC-003a
//   sendBeacon / XHR / WebSocket are caught .............. FR-011, FR-011b
//   refetch() and api.fetchRaw() are not caught .......... FR-011c, SC-003b
//   a missing file on the list fails rather than passes .. FR-012
//   the api client is not on the list .................... FR-011a
//
// Fixtures are synthetic path -> source maps, except the one case that reads the
// repository on purpose.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {check, EXPORT_PATH_FILES, EXCLUDED} from './check-diagnostics-no-upload.mjs';

const read = () => Object.fromEntries(EXPORT_PATH_FILES.map(f => [f, fs.readFileSync(new URL(`../${f}`, import.meta.url), 'utf8')]));

test('the repository export path makes no outbound request', () => {
  assert.deepEqual(check(read()), []);
});

test('a fetch to an external host is reported, naming the file and the primitive', () => {
  const files = read();
  const target = 'apps/web/platform/content-diagnostics.ts';
  files[target] += `\nvoid fetch("https://telemetry.example.test/upload", {method: "POST"});\n`;
  const errors = check(files);
  assert.equal(errors.length, 1, JSON.stringify(errors));
  assert.match(errors[0], new RegExp(`^${target}: `));
  assert.match(errors[0], /fetch/);
});

test('sendBeacon, XMLHttpRequest and WebSocket are reported too', () => {
  for (const [snippet, primitive] of [
    ['navigator.sendBeacon("/collect", blob);', 'sendBeacon'],
    ['const x = new XMLHttpRequest();', 'XMLHttpRequest'],
    ['const s = new WebSocket("wss://example.test");', 'WebSocket'],
  ]) {
    const files = read();
    files['packages/views/content/diagnostics/index.tsx'] += `\n${snippet}\n`;
    const errors = check(files);
    assert.equal(errors.length, 1, `${primitive}: ${JSON.stringify(errors)}`);
    assert.match(errors[0], new RegExp(primitive));
  }
});

// The panel calls refetch() four times today. A substring match on "fetch("
// reports all four, and the first reader of those four red lines would go
// looking for a network call that was never there.
test('refetch() and the shared api client are not mistaken for outbound calls', () => {
  const files = read();
  files['packages/views/content/diagnostics/index.tsx'] += `
    void d.overview.refetch();
    void d.events.refetch();
    const raw = await api.fetchRaw("/api/content-diagnostics/export");
    const again = someObject.prefetch();
  `;
  assert.deepEqual(check(files), []);
});

// A check that cannot find its files is green for the worst possible reason.
test('a file missing from the list fails instead of silently passing', () => {
  const files = read();
  delete files['apps/web/platform/content-diagnostics.ts'];
  const errors = check(files);
  assert.ok(errors.length, 'a missing file produced no error');
  assert.match(errors.join('\n'), /apps\/web\/platform\/content-diagnostics\.ts/);
  assert.match(errors.join('\n'), /missing/i);
});

// The download itself goes through the shared API client. Scanning it would
// report the feature working as a violation.
test('the shared api client is excluded from the list, on purpose', () => {
  assert.ok(!EXPORT_PATH_FILES.includes('packages/core/api/client.ts'));
  assert.ok(EXCLUDED.includes('packages/core/api/client.ts'));
});

test('the list names the four files of the export path', () => {
  assert.deepEqual([...EXPORT_PATH_FILES].sort(), [
    'apps/web/app/[workspaceSlug]/(dashboard)/diagnostics/page.tsx',
    'apps/web/platform/content-diagnostics.ts',
    'packages/core/content/diagnostics/queries.ts',
    'packages/views/content/diagnostics/index.tsx',
  ]);
});
