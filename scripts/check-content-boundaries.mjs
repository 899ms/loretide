import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import ts from 'typescript';

// Extract Go import paths with a small lexer that skips comments and string,
// raw-string and rune literals. A regex over raw source mistakes `import "x"`
// inside a comment or string for a real import, and a `)` inside a comment
// truncates a non-greedy import-group match and hides later imports. The lexer
// only recognises the `import` keyword in code context, so neither happens.
export function extractGoImports(source) {
  const specs = [], n = source.length;
  const ident = c => c !== undefined && /[A-Za-z0-9_]/.test(c);
  // Read an interpreted string that starts at the opening quote; returns
  // [value, indexAfterClosingQuote]. Backslash escapes are consumed literally.
  const readString = i => {
    i++; let v = '';
    while (i < n && source[i] !== '"') { if (source[i] === '\\') { v += source[i + 1] ?? ''; i += 2; continue; } v += source[i++]; }
    return [v, i + 1];
  };
  // Read a raw string delimited by backticks (no escapes).
  const readRaw = i => { i++; let v = ''; while (i < n && source[i] !== '`') v += source[i++]; return [v, i + 1]; };
  const skipTrivia = i => {
    for (;;) {
      while (i < n && /\s/.test(source[i])) i++;
      if (source[i] === '/' && source[i + 1] === '/') { while (i < n && source[i] !== '\n') i++; continue; }
      if (source[i] === '/' && source[i + 1] === '*') { i += 2; while (i < n && !(source[i] === '*' && source[i + 1] === '/')) i++; i += 2; continue; }
      break;
    }
    return i;
  };
  let i = 0;
  while (i < n) {
    const c = source[i];
    if (c === '/' && source[i + 1] === '/') { while (i < n && source[i] !== '\n') i++; continue; }
    if (c === '/' && source[i + 1] === '*') { i += 2; while (i < n && !(source[i] === '*' && source[i + 1] === '/')) i++; i += 2; continue; }
    if (c === '"') { i = readString(i)[1]; continue; }
    if (c === '`') { i = readRaw(i)[1]; continue; }
    if (c === "'") { i++; while (i < n && source[i] !== "'") { if (source[i] === '\\') i++; i++; } i++; continue; }
    if (c === 'i' && source.startsWith('import', i) && !ident(source[i - 1]) && !ident(source[i + 6])) {
      let j = skipTrivia(i + 6);
      if (source[j] === '(') {
        j++;
        for (;;) {
          if (j >= n) break;
          if (source[j] === '/' && source[j + 1] === '/') { while (j < n && source[j] !== '\n') j++; continue; }
          if (source[j] === '/' && source[j + 1] === '*') { j += 2; while (j < n && !(source[j] === '*' && source[j + 1] === '/')) j++; j += 2; continue; }
          if (source[j] === ')') { j++; break; }
          if (source[j] === '"') { const [v, nj] = readString(j); specs.push(v); j = nj; continue; }
          if (source[j] === '`') { const [v, nj] = readRaw(j); specs.push(v); j = nj; continue; }
          j++;
        }
        i = j; continue;
      }
      // Single import: an optional alias (identifier, `_` or `.`) then the path.
      let k = j;
      if (source[k] === '.') k++; else while (ident(source[k])) k++;
      k = skipTrivia(k);
      if (source[k] === '"') { const [v, nj] = readString(k); specs.push(v); i = nj; continue; }
      if (source[k] === '`') { const [v, nj] = readRaw(k); specs.push(v); i = nj; continue; }
      i = Math.max(k, i + 6); continue;
    }
    i++;
  }
  return specs;
}

// True when a TS/JS file performs a module load that cannot be resolved
// statically: a dynamic import() or require() whose argument is not a plain
// string literal (an identifier, concatenation, template literal, or missing).
// Detected on the AST so text inside comments and strings is never matched.
export function hasComputedModuleLoad(source) {
  const sf = ts.createSourceFile('boundary-check.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  let found = false;
  const walk = node => {
    if (found) return;
    if (ts.isCallExpression(node)) {
      const isDynamicImport = node.expression.kind === ts.SyntaxKind.ImportKeyword;
      const isRequire = ts.isIdentifier(node.expression) && node.expression.text === 'require';
      if (isDynamicImport || isRequire) {
        const arg = node.arguments[0];
        if (!arg || !ts.isStringLiteral(arg)) found = true;
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(sf);
  return found;
}

export function check(files, config) {
  const errors = [], edges = new Map();
  const classify = file => {
    for (const root of config.roots) if (file.startsWith(root+'/')) {
      const [module, ...rest] = file.slice(root.length+1).split('/');
      return {root, module, rest};
    }
  };
  for (const [rawFile, source] of Object.entries(files)) {
    // Normalise Windows separators so classification and relative-path
    // resolution work on backslash inputs instead of silently treating a
    // content file as external and skipping its boundary checks.
    const file = rawFile.replaceAll('\\', '/');
    if (/\.(test\.tsx?|test\.mjs)$|_test\.go$/.test(file)) continue;
    const owner = classify(file);
    if (owner && !Object.hasOwn(config.modules,owner.module)) { errors.push(`${file}: unregistered module`); continue; }
    const imports = file.endsWith('.go')
      ? extractGoImports(source)
      : ts.preProcessFile(source,true,true).importedFiles.map(x=>x.fileName);
    if (owner && !file.endsWith('.go') && hasComputedModuleLoad(source)) errors.push(`${file}: computed import forbidden`);
    for (const spec of imports) {
      let target = spec.replace('github.com/multica-ai/multica/server/','server/').replace('@multica/core/','packages/core/').replace('@multica/views/','packages/views/');
      if (spec.startsWith('.')) target = path.posix.normalize(path.posix.join(path.posix.dirname(file),spec));
      const other = classify(target);
      if (!owner) {
        if (other && (!config.adapters.includes(file) || other.rest.length && !/^index\.tsx?$/.test(other.rest.join('/')))) errors.push(`${file}: unapproved content adapter or private import ${spec}`);
        continue;
      }
      if (other) {
        if (!Object.hasOwn(config.modules,other.module)) errors.push(`${file}: unknown target ${spec}`);
        if (owner.root !== other.root && !(owner.root==='packages/views/content' && other.root==='packages/core/content')) errors.push(`${file}: reverse layer dependency ${spec}`);
        if (owner.module !== other.module && !config.modules[owner.module].includes(other.module)) errors.push(`${file}: forbidden dependency ${spec}`);
        if (owner.module !== other.module || owner.root !== other.root) {
          if (other.rest.length && !(other.rest.length===1 && /^index(?:\.ts)?$/.test(other.rest[0]))) errors.push(`${file}: private import ${spec}`);
          if (owner.module!==other.module) { const set=edges.get(owner.module)||new Set(); set.add(other.module); edges.set(owner.module,set); }
        }
      } else {
        const stdGo = file.endsWith('.go') && !spec.split('/')[0].includes('.');
        const allowed = config.upstreamImports[owner.root].some(p=>spec===p || (p==='@multica/ui' || p.startsWith('github.') || p.startsWith('go.')) && spec.startsWith(p+'/'));
        if (!stdGo && !allowed) errors.push(`${file}: unapproved upstream import ${spec}`);
      }
    }
    if (owner?.root==='packages/core/content' && /\b(localStorage|process\.env)\b/.test(source)) errors.push(`${file}: platform access in core`);
  }
  const visit=(node,stack=[])=>{ if(stack.includes(node)) { errors.push(`cycle: ${[...stack,node].join(' -> ')}`); return; } for(const dep of edges.get(node)||[]) visit(dep,[...stack,node]); };
  for(const node of edges.keys()) visit(node);
  return [...new Set(errors)];
}

if (process.argv[1] && path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
  const config=JSON.parse(fs.readFileSync(path.join(root,'scripts/content-boundaries.json'),'utf8'));
  const files={};
  const walk=dir=>{ if(!fs.existsSync(dir))return; for(const entry of fs.readdirSync(dir,{withFileTypes:true})) {if(['node_modules','.next','dist','data','.git','vendor'].includes(entry.name))continue;const p=path.join(dir,entry.name); if(entry.isDirectory())walk(p); else if(/\.(go|tsx?)$/.test(p))files[path.relative(root,p).replaceAll('\\','/')]=fs.readFileSync(p,'utf8');}};
  ['server','packages','apps/web'].forEach(r=>walk(path.join(root,r)));
  const errors=check(files,config);
  if(errors.length) { console.error(errors.join('\n')); process.exitCode=1; }
  else console.log(`Content boundaries passed (${Object.keys(files).length} files; ${Object.keys(config.modules).length} registered modules)`);
}
