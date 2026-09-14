// Tests for scripts/check-diagnostics-contract.mjs.
//
// Case -> requirement map (specs/007-diag-delivery-contract/spec.md):
//   consumer passing all three .......................... FR-004, FR-011
//   missing E1 / E2 / E3, each named separately ......... FR-006, FR-011, FR-011a, SC-002
//   declared-but-not-landed module stays silent ......... FR-005, SC-003
//   tool-function-only use is not a call site ........... FR-011 (E2)
//   real diagnostics module (provider rule) ............. FR-004, SC-001, SC-003
//   exemption honoured while unexpired ................... FR-012
//   exemption without expires / reason / where invalid ... FR-012a, SC-002a
//   unknown module or version in config invalid ......... FR-012
//   expired exemption stops admitting, names expiry ..... FR-012b, SC-002a
//   contract's public entries stay declared in the code .. FR-002, SC-001
//   contract document names every checked entry ......... FR-001, FR-002, SC-001
//
// Fixtures are synthetic path -> source maps. A negative case must never be a
// real directory in this repository: the existing boundary checker walks the
// tree and would flag a fake module, so the two checks would fight.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {check, stripGoTrivia, declaresGoSymbol, CONTRACT_ENTRIES} from './check-diagnostics-contract.mjs';

const boundaries = JSON.parse(fs.readFileSync(new URL('./content-boundaries.json', import.meta.url), 'utf8'));
const DIAG = 'github.com/multica-ai/multica/server/internal/content/diagnostics';
const NOW = new Date('2026-09-14T00:00:00Z');

const config = (exemptions = []) => ({version: 1, modules: boundaries.modules, exemptions});
const src = (module, name, body) => ({[`server/internal/content/${module}/${name}`]: body});

// A module that satisfies all three: imports diagnostics, calls a recognised
// entry, and has a test that imports diagnostics too.
const passing = module => ({
  ...src(module, 'service.go', `package editor\nimport "${DIAG}"\n\nfunc save(s *diagnostics.Store) error { return s.Audit(ctx, scope, e) }\n`),
  ...src(module, 'service_test.go', `package editor\nimport (\n\t"testing"\n\t"${DIAG}"\n)\n\nfunc TestSave(t *testing.T) { _ = diagnostics.Event{} }\n`),
});

test('a module with import, call site and test passes with no output', () => {
  assert.deepEqual(check(passing('work-editor'), config(), NOW), []);
});

test('missing E1 (no import) is reported as E1 and not as something else', () => {
  const files = {
    ...src('work-editor', 'service.go', 'package editor\n\nfunc save(s *Store) error { return s.Audit(ctx, scope, e) }\n'),
    ...src('work-editor', 'service_test.go', `package editor\nimport (\n\t"testing"\n\t"${DIAG}"\n)\n`),
  };
  const errors = check(files, config(), NOW);
  assert.equal(errors.length, 1, JSON.stringify(errors));
  assert.match(errors[0], /^work-editor: missing E1 /);
});

test('missing E2 (import but no call site) is reported as E2', () => {
  const files = {
    ...src('work-editor', 'service.go', `package editor\nimport "${DIAG}"\n\nvar _ = diagnostics.Event{}\n`),
    ...src('work-editor', 'service_test.go', `package editor\nimport (\n\t"testing"\n\t"${DIAG}"\n)\n`),
  };
  const errors = check(files, config(), NOW);
  assert.equal(errors.length, 1, JSON.stringify(errors));
  assert.match(errors[0], /^work-editor: missing E2 /);
});

test('missing E3 (no test referencing diagnostics) is reported as E3', () => {
  const files = {
    ...src('work-editor', 'service.go', `package editor\nimport "${DIAG}"\n\nfunc save(s *diagnostics.Store) error { return s.Audit(ctx, scope, e) }\n`),
    ...src('work-editor', 'service_test.go', 'package editor\nimport "testing"\n\nfunc TestSave(t *testing.T) {}\n'),
  };
  const errors = check(files, config(), NOW);
  assert.equal(errors.length, 1, JSON.stringify(errors));
  assert.match(errors[0], /^work-editor: missing E3 /);
});

test('each missing condition is named on its own, not collapsed into one verdict', () => {
  const files = src('work-editor', 'service.go', 'package editor\n\nfunc save() {}\n');
  const errors = check(files, config(), NOW);
  assert.equal(errors.length, 3, JSON.stringify(errors));
  for (const condition of ['E1', 'E2', 'E3']) {
    assert.ok(errors.some(e => e.startsWith(`work-editor: missing ${condition} `)), condition);
  }
});

test('a module declared in the module graph but with no directory produces nothing', () => {
  // source-inbox, topic-planning and nine others are declared and have no code.
  const errors = check(passing('work-editor'), config(), NOW);
  assert.deepEqual(errors, []);
  assert.ok(!errors.join('\n').includes('source-inbox'));
});

test('importing diagnostics but only calling a tool function is not a call site', () => {
  const files = {
    ...src('work-editor', 'service.go', `package editor\nimport "${DIAG}"\n\nfunc id() string { return diagnostics.NewID() }\n`),
    ...src('work-editor', 'service_test.go', `package editor\nimport (\n\t"testing"\n\t"${DIAG}"\n)\n`),
  };
  const errors = check(files, config(), NOW);
  assert.equal(errors.length, 1, JSON.stringify(errors));
  assert.match(errors[0], /^work-editor: missing E2 /);
});

test('an import inside a comment or string does not satisfy E1', () => {
  const files = {
    ...src('work-editor', 'service.go', `package editor\n// import "${DIAG}"\nvar doc = \`import "${DIAG}"\`\n\nfunc save(s *Store) error { return s.Audit(ctx, scope, e) }\n`),
    ...src('work-editor', 'service_test.go', `package editor\nimport (\n\t"testing"\n\t"${DIAG}"\n)\n`),
  };
  assert.ok(check(files, config(), NOW).some(e => e.startsWith('work-editor: missing E1 ')));
});

test('a call site inside a comment or string does not satisfy E2', () => {
  const files = {
    ...src('work-editor', 'service.go', `package editor\nimport "${DIAG}"\n\n// s.Audit(ctx, scope, e) is what this will do later\nvar todo = "diagnostics.Technical(ctx, e)"\n`),
    ...src('work-editor', 'service_test.go', `package editor\nimport (\n\t"testing"\n\t"${DIAG}"\n)\n`),
  };
  assert.ok(check(files, config(), NOW).some(e => e.startsWith('work-editor: missing E2 ')));
});

test('a test file alone does not satisfy E1 - the import must be in non-test code', () => {
  const files = src('work-editor', 'service_test.go', `package editor\nimport (\n\t"testing"\n\t"${DIAG}"\n)\n\nfunc TestX(t *testing.T) { _ = diagnostics.Event{} }\n`);
  const errors = check(files, config(), NOW);
  assert.ok(errors.some(e => e.startsWith('work-editor: missing E1 ')), JSON.stringify(errors));
  assert.ok(!errors.some(e => e.startsWith('work-editor: missing E3 ')), JSON.stringify(errors));
});

test('each of the three call-site categories satisfies E2 on its own', () => {
  for (const callSite of ['s.Audit(ctx, scope, e)', 's.Technical(ctx, e)', 'diagnostics.Child(ctx)']) {
    const files = {
      ...src('work-editor', 'service.go', `package editor\nimport "${DIAG}"\n\nfunc f() { ${callSite} }\n`),
      ...src('work-editor', 'service_test.go', `package editor\nimport (\n\t"testing"\n\t"${DIAG}"\n)\n`),
    };
    assert.deepEqual(check(files, config(), NOW), [], callSite);
  }
});

// The provider cannot import itself, so E1/E2/E3 do not apply to it. It is
// judged on providing the surface instead: the package declaration, every
// public entry the contract names, and tests.
test('the real diagnostics module passes the provider rule', () => {
  const root = new URL('../server/internal/content/diagnostics/', import.meta.url);
  const files = {};
  for (const name of fs.readdirSync(root)) {
    if (name.endsWith('.go')) files[`server/internal/content/diagnostics/${name}`] = fs.readFileSync(new URL(name, root), 'utf8');
  }
  assert.deepEqual(check(files, config(), NOW), []);
});

test('the provider rule names a public entry the contract points at but the code no longer exports', () => {
  const files = {
    'server/internal/content/diagnostics/store.go': 'package diagnostics\n\nfunc (s *Store) Audit() error { return nil }\n',
    'server/internal/content/diagnostics/store_test.go': 'package diagnostics\n\nimport "testing"\n',
  };
  const errors = check(files, config(), NOW);
  assert.ok(errors.some(e => /^diagnostics: missing P2 .*CommitRun/.test(e)), JSON.stringify(errors));
});

test('stripGoTrivia removes comments and string literals but keeps code', () => {
  assert.equal(stripGoTrivia('a // b\nc').includes('b'), false);
  assert.equal(stripGoTrivia('a /* b */ c').includes('b'), false);
  assert.equal(stripGoTrivia('x := "b"').includes('b'), false);
  assert.equal(stripGoTrivia('x := `b`').includes('b'), false);
  assert.ok(stripGoTrivia('a // b\nAudit(').includes('Audit('));
});

test('declaresGoSymbol finds func, method, type and var declarations', () => {
  assert.ok(declaresGoSymbol('func Child(ctx context.Context) {}', 'Child'));
  assert.ok(declaresGoSymbol('func (s *Store) Audit(ctx context.Context) error { return nil }', 'Audit'));
  assert.ok(declaresGoSymbol('type SlogHandler struct{}', 'SlogHandler'));
  assert.ok(declaresGoSymbol('var Scenarios=[]Scenario{}', 'Scenarios'));
  assert.ok(!declaresGoSymbol('func NewLogBuffer(n int) {}', 'LogBuffer'));
});

// --- exemption registry (scripts/diagnostics-contract.json) ---------------

const bare = module => src(module, 'service.go', 'package editor\n\nfunc save() {}\n');
const exemption = over => ({module: 'work-editor', reason: 'audit is written in the handler layer', where: 'server/internal/handler/work_editor.go', expires: '2026-12-31', ...over});

test('an unexpired exemption admits a module that would otherwise fail all three', () => {
  assert.deepEqual(check(bare('work-editor'), config([exemption()]), NOW), []);
});

test('an exemption without expires is an invalid configuration, not a permanent pass', () => {
  const {expires, ...withoutExpires} = exemption();
  const errors = check(bare('work-editor'), config([withoutExpires]), NOW);
  assert.ok(errors.some(e => /"expires" is required/.test(e)), JSON.stringify(errors));
  // The configuration failure stands on its own: the check does not go on to
  // judge modules against a configuration it could not read.
  assert.ok(!errors.some(e => /missing E[123] /.test(e)), JSON.stringify(errors));
});

test('an exemption with a malformed expires is invalid', () => {
  for (const expires of ['31/12/2026', '2026-13-40', '', 'soon', 2026]) {
    const errors = check(bare('work-editor'), config([exemption({expires})]), NOW);
    assert.ok(errors.some(e => /"expires" is required/.test(e)), JSON.stringify([expires, errors]));
  }
});

test('an exemption without reason or without where is invalid', () => {
  assert.ok(check(bare('work-editor'), config([exemption({reason: '  '})]), NOW).some(e => /"reason" is required/.test(e)));
  assert.ok(check(bare('work-editor'), config([exemption({where: ''})]), NOW).some(e => /"where" is required/.test(e)));
});

test('an exemption for a module that is not in the module graph is invalid', () => {
  assert.ok(check(bare('work-editor'), config([exemption({module: 'not-a-module'})]), NOW).some(e => /unknown module/.test(e)));
});

test('an unsupported config version is invalid and nothing else is judged', () => {
  const errors = check(bare('work-editor'), {version: 2, modules: boundaries.modules, exemptions: []}, NOW);
  assert.equal(errors.length, 1, JSON.stringify(errors));
  assert.match(errors[0], /unsupported version/);
});

test('an expired exemption stops admitting and the failure names the expiry, not just missing evidence', () => {
  const errors = check(bare('work-editor'), config([exemption({expires: '2026-09-13'})]), NOW);
  assert.ok(errors.some(e => /^work-editor: exemption expired on 2026-09-13 /.test(e)), JSON.stringify(errors));
  // Re-judged as if it were never registered: all three conditions reported.
  for (const condition of ['E1', 'E2', 'E3']) {
    assert.ok(errors.some(e => e.startsWith(`work-editor: missing ${condition} `)), condition);
  }
});

test('an exemption expiring today still admits; it lapses the day after', () => {
  assert.deepEqual(check(bare('work-editor'), config([exemption({expires: '2026-09-14'})]), NOW), []);
  assert.ok(check(bare('work-editor'), config([exemption({expires: '2026-09-14'})]), new Date('2026-09-15T00:00:00Z')).some(e => /exemption expired/.test(e)));
});

test('an exemption for a module that has not landed produces nothing at all', () => {
  // source-inbox has no directory; an exemption for it must not turn into output.
  assert.deepEqual(check(passing('work-editor'), config([exemption({module: 'source-inbox'})]), NOW), []);
});

test('two exemptions for the same module are invalid', () => {
  assert.ok(check(bare('work-editor'), config([exemption(), exemption()]), NOW).some(e => /duplicate exemption/.test(e)));
});

// --- the contract document and the code it points at stay in step ----------

test('every public entry the contract document names is declared by the diagnostics module', () => {
  const doc = fs.readFileSync(new URL('../docs/development/diagnostics-onboarding-contract.md', import.meta.url), 'utf8');
  for (const name of CONTRACT_ENTRIES) {
    assert.ok(doc.includes(name), `${name} is checked against the code but is not named in the onboarding contract`);
  }
});
