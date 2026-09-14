import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {extractGoImports} from './check-content-boundaries.mjs';

// Static half of docs/development/diagnostics-onboarding-contract.md: every
// content module that has landed must carry the minimal traces of diagnostics
// onboarding. It proves traces exist, never that the onboarding is semantically
// right and never that a real executor ran - the contract says so in prose
// because no static check can say it.
//
// The Go import lexer is reused from check-content-boundaries.mjs rather than
// written a second time. It already skips comments, strings, raw strings and
// rune literals; a second lexer would be a second place for that to be wrong.

const DIAGNOSTICS_IMPORT = 'github.com/multica-ai/multica/server/internal/content/diagnostics';
const CONTENT_ROOT = 'server/internal/content';
// The module that provides the package is named by the import path itself, so
// renaming the package cannot leave this constant pointing at nothing.
const PROVIDER_MODULE = DIAGNOSTICS_IMPORT.slice(DIAGNOSTICS_IMPORT.lastIndexOf('/') + 1);

// E2 call sites. Deliberately not NewID or Sanitize: they are tool functions,
// and using one does not mean the module records anything. Keeping them out is
// the entire value E2 adds on top of E1.
const CALL_SITES = {
  audit: ['Audit', 'CommitRun'],
  'technical log': ['Technical', 'SlogHandler', 'LogBuffer'],
  trace: ['Child', 'Pack', 'Unpack', 'DecodeQueuedEnvelope'],
};
const CALL_SITE_NAMES = Object.values(CALL_SITES).flat();

// Every symbol the "public entry" column of the onboarding contract names.
// The provider rule holds the code against this list, so renaming or deleting
// one of them turns the check red instead of quietly leaving the contract
// pointing at something that no longer exists. The error-code enum is
// deliberately absent: log.go's `codes` is unexported, and the contract says so
// rather than requiring a module to reference a private symbol.
export const CONTRACT_ENTRIES = [
  ...CALL_SITE_NAMES, 'NewLogBuffer',
  'Envelope', 'Outbox', 'MemoryOutbox', 'NewMemoryOutbox',
  'Scenario', 'Scenarios', 'Simulate', 'Evaluate',
  'Sanitize', 'RequestIdentity',
  'Store', 'Scope', 'Event',
];

const EXPIRES_SHAPE = /^\d{4}-\d{2}-\d{2}$/;

// Blank out comments and string, raw-string and rune literals, preserving
// offsets so a later regex cannot match text a Go compiler never sees.
export function stripGoTrivia(source) {
  const out = [...source], n = source.length;
  const blank = (from, to) => { for (let k = from; k < to && k < n; k++) if (out[k] !== '\n') out[k] = ' '; };
  let i = 0;
  while (i < n) {
    const c = source[i];
    if (c === '/' && source[i + 1] === '/') { let j = i; while (j < n && source[j] !== '\n') j++; blank(i, j); i = j; continue; }
    if (c === '/' && source[i + 1] === '*') { let j = i + 2; while (j < n && !(source[j] === '*' && source[j + 1] === '/')) j++; blank(i, Math.min(j + 2, n)); i = j + 2; continue; }
    if (c === '"' || c === "'") { let j = i + 1; while (j < n && source[j] !== c && source[j] !== '\n') { if (source[j] === '\\') j++; j++; } blank(i, Math.min(j + 1, n)); i = j + 1; continue; }
    if (c === '`') { let j = i + 1; while (j < n && source[j] !== '`') j++; blank(i, Math.min(j + 1, n)); i = j + 1; continue; }
    i++;
  }
  return out.join('');
}

// True when the source declares name as a func, method, type or var. Used by
// the provider rule to hold the contract's public-entry column against the
// code: rename or delete one of those entries and the check goes red.
export function declaresGoSymbol(source, name) {
  const code = stripGoTrivia(source);
  return new RegExp(`^func\\s+(?:\\([^)]*\\)\\s*)?${name}\\s*[(\\[]`, 'm').test(code)
    || new RegExp(`^type\\s+${name}[\\s{(\\[]`, 'm').test(code)
    || new RegExp(`^var\\s+${name}[\\s=]`, 'm').test(code);
}

function hasCallSite(source) {
  const code = stripGoTrivia(source);
  // An optional qualifier of any depth, so both diagnostics.Child(ctx) and the
  // method form store.Audit(ctx, ...) count, and a dot-imported bare name too.
  // `{` is allowed because SlogHandler and LogBuffer are types, used as literals.
  return new RegExp(`(?:^|[^\\w.])(?:[A-Za-z_]\\w*\\.)*(?:${CALL_SITE_NAMES.join('|')})\\s*[({]`).test(code);
}

function validateConfig(config) {
  const errors = [];
  const where = 'scripts/diagnostics-contract.json';
  if (config.version !== 1) { errors.push(`${where}: unsupported version ${JSON.stringify(config.version)} — expected 1`); return errors; }
  if (!Array.isArray(config.exemptions)) { errors.push(`${where}: "exemptions" must be an array`); return errors; }
  const seen = new Set();
  for (const [index, exemption] of config.exemptions.entries()) {
    const at = `${where}: exemption #${index + 1}`;
    if (!exemption || typeof exemption !== 'object') { errors.push(`${at}: must be an object`); continue; }
    const {module, reason, where: location, expires} = exemption;
    if (typeof module !== 'string' || !Object.hasOwn(config.modules, module)) errors.push(`${at}: unknown module ${JSON.stringify(module)} — it must be declared in scripts/content-boundaries.json`);
    else if (seen.has(module)) errors.push(`${at}: duplicate exemption for ${module}`);
    else seen.add(module);
    if (typeof reason !== 'string' || !reason.trim()) errors.push(`${at} (${module}): "reason" is required and must be non-empty — an exemption has to read like a decision, not a silent allowlist entry`);
    if (typeof location !== 'string' || !location.trim()) errors.push(`${at} (${module}): "where" is required and must name where the onboarding actually lives`);
    // An exemption without an expiry date is a permanent exemption, which has
    // exactly the same effect as deleting the check. Missing is invalid, not
    // defaulted.
    if (typeof expires !== 'string' || !EXPIRES_SHAPE.test(expires) || Number.isNaN(Date.parse(`${expires}T00:00:00Z`))) errors.push(`${at} (${module}): "expires" is required and must be a YYYY-MM-DD date — an exemption with no expiry is a permanent exemption`);
  }
  return errors;
}

export function check(files, config, now = new Date()) {
  const configErrors = validateConfig(config);
  // An invalid configuration fails on its own. Judging modules against a
  // configuration the check could not read would report the wrong thing.
  if (configErrors.length) return configErrors;

  const today = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  const exemptions = new Map(config.exemptions.map(e => [e.module, e]));
  const byModule = new Map();
  for (const [rawFile, source] of Object.entries(files)) {
    const file = rawFile.replaceAll('\\', '/');
    if (!file.startsWith(`${CONTENT_ROOT}/`) || !file.endsWith('.go')) continue;
    const module = file.slice(CONTENT_ROOT.length + 1).split('/')[0];
    if (!Object.hasOwn(config.modules, module)) continue;
    if (!byModule.has(module)) byModule.set(module, []);
    byModule.get(module).push({file, source, isTest: file.endsWith('_test.go')});
  }

  const errors = [];
  for (const module of Object.keys(config.modules)) {
    const moduleFiles = byModule.get(module) ?? [];
    // The contract binds a module the moment it lands. Eleven of the twelve
    // declared modules have no directory today; a check that reported them
    // would ship eleven red lines nobody could act on.
    if (!moduleFiles.length) continue;

    const exemption = exemptions.get(module);
    if (exemption) {
      const expires = new Date(`${exemption.expires}T00:00:00Z`);
      if (expires >= today) continue;
      errors.push(`${module}: exemption expired on ${exemption.expires} — renew it in scripts/diagnostics-contract.json or onboard the module; the conditions below are judged as if it were not registered`);
    }

    errors.push(...(module === PROVIDER_MODULE ? judgeProvider(module, moduleFiles) : judgeConsumer(module, moduleFiles)));
  }

  return errors;
}

// The provider cannot import itself, so E1/E2/E3 cannot apply to it. It is held
// to providing the surface instead - which also holds the contract's
// public-entry column against the code.
function judgeProvider(module, moduleFiles) {
  const errors = [];
  const code = moduleFiles.filter(f => !f.isTest);
  if (!code.some(f => /^package\s+diagnostics\b/m.test(stripGoTrivia(f.source)))) errors.push(`${module}: missing P1 — no non-test .go file declares "package ${PROVIDER_MODULE}"; this module is expected to provide the diagnostics surface, not consume it`);
  const missing = CONTRACT_ENTRIES.filter(name => !code.some(f => declaresGoSymbol(f.source, name)));
  if (missing.length) errors.push(`${module}: missing P2 — the onboarding contract points at ${missing.join(', ')}, which this module no longer declares; update docs/development/diagnostics-onboarding-contract.md or restore the entry`);
  if (!moduleFiles.some(f => f.isTest)) errors.push(`${module}: missing P3 — no _test.go in the module that provides the diagnostics surface`);
  return errors;
}

function judgeConsumer(module, moduleFiles) {
  const errors = [];
  const dir = `${CONTENT_ROOT}/${module}/`;
  const imports = f => extractGoImports(f.source).includes(DIAGNOSTICS_IMPORT);
  if (!moduleFiles.some(f => !f.isTest && imports(f))) errors.push(`${module}: missing E1 — no non-test .go file in ${dir} imports ${DIAGNOSTICS_IMPORT}`);
  if (!moduleFiles.some(f => !f.isTest && hasCallSite(f.source))) errors.push(`${module}: missing E2 — no audit, technical-log or trace call site in ${dir} (one of ${CALL_SITE_NAMES.join(', ')}); importing the package and using only a tool function such as NewID does not count`);
  if (!moduleFiles.some(f => f.isTest && imports(f))) errors.push(`${module}: missing E3 — no _test.go in ${dir} imports ${DIAGNOSTICS_IMPORT}`);
  return errors;
}

// Counts for the CLI summary, computed the same way check() does so the two
// cannot drift.
export function survey(files, modules) {
  let checked = 0, skipped = 0;
  for (const module of Object.keys(modules)) {
    const prefix = `${CONTENT_ROOT}/${module}/`;
    if (Object.keys(files).some(f => f.replaceAll('\\', '/').startsWith(prefix) && f.endsWith('.go'))) checked++; else skipped++;
  }
  return {checked, skipped};
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
  const read = rel => JSON.parse(fs.readFileSync(path.join(root, rel), 'utf8'));
  const boundaries = read('scripts/content-boundaries.json');
  const contract = read('scripts/diagnostics-contract.json');
  const files = {};
  const walk = dir => {
    if (!fs.existsSync(dir)) return;
    for (const entry of fs.readdirSync(dir, {withFileTypes: true})) {
      const p = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(p);
      else if (p.endsWith('.go')) files[path.relative(root, p).replaceAll('\\', '/')] = fs.readFileSync(p, 'utf8');
    }
  };
  walk(path.join(root, CONTENT_ROOT));
  const config = {version: contract.version, modules: boundaries.modules, exemptions: contract.exemptions};
  const errors = check(files, config);
  const {checked, skipped} = survey(files, boundaries.modules);
  if (errors.length) { console.error(errors.join('\n')); process.exitCode = 1; }
  else console.log(`Diagnostics contract passed (checked ${checked} landed module${checked === 1 ? '' : 's'}; skipped ${skipped} not yet landed)`);
}
