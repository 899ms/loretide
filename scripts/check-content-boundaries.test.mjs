import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {check} from './check-content-boundaries.mjs';
const config=JSON.parse(fs.readFileSync(new URL('./content-boundaries.json',import.meta.url),'utf8'));
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
