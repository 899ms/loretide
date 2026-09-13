import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {check,extractGoImports,hasComputedModuleLoad} from './check-content-boundaries.mjs';
const config=JSON.parse(fs.readFileSync(new URL('./content-boundaries.json',import.meta.url),'utf8'));
const AGENT_WORKFLOW='github.com/multica-ai/multica/server/internal/content/agent-workflow';
test('public dependencies pass; private, reverse, platform and unregistered imports fail',()=>{
 assert.deepEqual(check({'packages/core/content/work-editor/index.ts':'import {x} from "@multica/core/content/workspace-core"'},config),[]);
 for(const spec of ['@multica/core/content/workspace-core/storage','electron','next/navigation','@multica/views/content/work-editor','@multica/core/content/agent-workflow','@multica/core/content/missing']) {
  assert.ok(check({'packages/core/content/work-editor/index.ts':`import {x} from "${spec}"`},config).length,spec);
 }
 assert.ok(check({'server/internal/content/work-editor/a.go':'package editor\nimport "github.com/multica-ai/multica/server/internal/content/workspace-core/storage"'},config).length);
 assert.ok(check({'packages/core/content/work-editor/index.ts':'const x=import(name)'},config).length);
 assert.ok(check({'server/internal/handler/rogue.go':'package handler\nimport "github.com/multica-ai/multica/server/internal/content/diagnostics"'},config).length);
 assert.ok(check({'server/internal/handler/content_diagnostics.go':'package handler\nimport "github.com/multica-ai/multica/server/internal/content/diagnostics/storage"'},config).length);
});
test('cycles fail even if configuration permits both edges',()=>{
 const c=structuredClone(config); c.modules['workspace-core'].push('work-editor');
 const result=check({'packages/core/content/work-editor/index.ts':'export * from "@multica/core/content/workspace-core"','packages/core/content/workspace-core/index.ts':'export * from "@multica/core/content/work-editor"'},c);
 assert.ok(result.some(x=>x.startsWith('cycle:')));
});

test('three-module import cycle is reported',()=>{
 const c=structuredClone(config);
 c.modules['workspace-core'].push('topic-planning'); // a->b permitted; b->c and c->a already permitted below
 const result=check({
  'packages/core/content/workspace-core/index.ts':'export * from "@multica/core/content/topic-planning"',
  'packages/core/content/topic-planning/index.ts':'export * from "@multica/core/content/knowledge-base"',
  'packages/core/content/knowledge-base/index.ts':'export * from "@multica/core/content/workspace-core"',
 },c);
 assert.ok(result.some(x=>x.startsWith('cycle:')),JSON.stringify(result));
});

// Go grouped, aliased, blank and dot imports are all extracted; standard-library
// entries in a group are ignored while a forbidden module import is still caught.
test('Go grouped/aliased/blank/dot imports are extracted and classified',()=>{
 const src=`package editor\nimport (\n\t"fmt"\n\t_ "database/sql"\n\t. "errors"\n\twc "github.com/multica-ai/multica/server/internal/content/workspace-core"\n)`;
 assert.deepEqual(extractGoImports(src),['fmt','database/sql','errors','github.com/multica-ai/multica/server/internal/content/workspace-core']);
 // workspace-core is an allowed dependency of work-editor and imported at its root, so this passes.
 assert.deepEqual(check({'server/internal/content/work-editor/a.go':src},config),[]);
 // A forbidden module reached through an aliased grouped import must be caught.
 const bad=`package editor\nimport (\n\t"fmt"\n\taw "${AGENT_WORKFLOW}"\n)`;
 assert.ok(check({'server/internal/content/work-editor/a.go':bad},config).some(x=>/forbidden dependency/.test(x)));
});

// A `)` inside a comment must not truncate a Go import group and hide later
// imports (the previous non-greedy regex stopped at the first `)`).
test('Go import group with a comment containing a paren still sees later imports',()=>{
 const src=`package editor\nimport (\n\t"fmt" // wraps call foo() bar\n\t"${AGENT_WORKFLOW}"\n)`;
 assert.deepEqual(extractGoImports(src),['fmt',AGENT_WORKFLOW]);
 assert.ok(check({'server/internal/content/work-editor/a.go':src},config).some(x=>/forbidden dependency/.test(x)));
});

// `import` appearing only inside Go comments or string/raw-string literals is not
// a real import and must not be flagged.
test('Go imports inside comments and strings are ignored',()=>{
 const src=`package editor\n// import "${AGENT_WORKFLOW}"\nvar doc = \`import "${AGENT_WORKFLOW}"\`\nvar note = "the word import \\"x\\" here"\nimport "fmt"`;
 assert.deepEqual(extractGoImports(src),['fmt']);
 assert.deepEqual(check({'server/internal/content/work-editor/a.go':src},config),[]);
});

// A dynamic import()/require() whose argument is not a plain string literal is a
// statically uncomputable module load and must be rejected, not ignored — even
// through string concatenation, which the old regex missed.
test('computed import()/require() is rejected including concatenated arguments',()=>{
 for(const body of ['const p=import(name)','const p=import("a"+"b")','const p=import(`x`)','const r=require(mod)']){
  assert.ok(check({'packages/core/content/work-editor/index.ts':body},config).some(x=>/computed import forbidden/.test(x)),body);
 }
 // A plain string-literal dynamic import is static and boundary-checked, not rejected as computed.
 assert.ok(!hasComputedModuleLoad('const p=import("@multica/core/content/workspace-core")'));
});

// import()/require() text inside comments or strings must not trigger the
// computed-import rejection (the old raw-source regex false-positived here).
test('computed-import detection ignores comments and strings',()=>{
 const src='// import(dynamic)\nconst s="require(notReal)";\nexport const x=1;';
 assert.equal(hasComputedModuleLoad(src),false);
 assert.deepEqual(check({'packages/core/content/work-editor/index.ts':src},config),[]);
});

// TS re-export and type-only imports still create an architectural edge and are
// subject to the same layering and dependency rules as value imports.
test('TS re-export and type-only imports are still checked',()=>{
 assert.ok(check({'packages/core/content/work-editor/index.ts':'export {X} from "@multica/views/content/work-editor"'},config).some(x=>/reverse layer dependency/.test(x)));
 assert.ok(check({'packages/core/content/work-editor/index.ts':'export * from "@multica/core/content/agent-workflow"'},config).some(x=>/forbidden dependency/.test(x)));
 assert.ok(check({'packages/core/content/work-editor/index.ts':'import type {X} from "@multica/core/content/agent-workflow"'},config).some(x=>/forbidden dependency/.test(x)));
});

// A relative-path import that resolves into another module's private (non-index)
// path is a private import.
test('relative-path private module import is flagged',()=>{
 assert.ok(check({'packages/core/content/work-editor/index.ts':'import {x} from "../workspace-core/storage"'},config).some(x=>/private import/.test(x)));
 // A relative import within the same module stays allowed.
 assert.deepEqual(check({'packages/core/content/work-editor/index.ts':'import {x} from "./storage"'},config),[]);
});

// Windows-style backslash paths must be normalised so real violations are
// classified correctly rather than misreported as rogue adapters (or missed).
test('Windows backslash input paths are normalised before classification',()=>{
 const win='packages\\core\\content\\work-editor\\index.ts';
 const forbidden=check({[win]:'import {x} from "@multica/core/content/agent-workflow"'},config);
 assert.ok(forbidden.some(x=>/forbidden dependency/.test(x)),JSON.stringify(forbidden));
 assert.ok(!forbidden.some(x=>/content adapter/.test(x)),JSON.stringify(forbidden));
 // A legitimate allowed import on a backslash path must not raise a false positive.
 assert.deepEqual(check({[win]:'import {x} from "@multica/core/content/workspace-core"'},config),[]);
});
