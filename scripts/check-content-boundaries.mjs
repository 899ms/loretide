import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import ts from 'typescript';

export function check(files, config) {
  const errors = [], edges = new Map();
  const classify = file => {
    for (const root of config.roots) if (file.startsWith(root+'/')) {
      const [module, ...rest] = file.slice(root.length+1).split('/');
      return {root, module, rest};
    }
  };
  for (const [file, source] of Object.entries(files)) {
    if (/\.(test\.tsx?|test\.mjs)$|_test\.go$/.test(file)) continue;
    const owner = classify(file);
    if (owner && !Object.hasOwn(config.modules,owner.module)) { errors.push(`${file}: unregistered module`); continue; }
    const imports = file.endsWith('.go')
      ? [...source.matchAll(/\bimport\s*(?:\(([\s\S]*?)\)|(?:[\w.]+\s+)?"([^"]+)")/g)].flatMap(m => m[2] ? [m[2]] : [...m[1].matchAll(/"([^"]+)"/g)].map(x=>x[1]))
      : ts.preProcessFile(source,true,true).importedFiles.map(x=>x.fileName);
    if (owner && !file.endsWith('.go') && /\b(?:import|require)\s*\(\s*[^'"\s]/.test(source)) errors.push(`${file}: computed import forbidden`);
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
