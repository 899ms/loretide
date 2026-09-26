// T076 mutation evidence for Issue #283 / 036 PR3.
// Run only inside the existing isolated CI test container, with its dedicated
// least-privilege PostgreSQL database already provisioned. Never run locally.
import { readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import path from 'node:path';

const root = process.cwd();
const server = path.join(root, 'server');
const dbURL = process.env.LORETIDE_DB_TEST_DATABASE_URL;
for (const name of [
  'LORETIDE_DB_TESTS',
  'LORETIDE_DB_TEST_DATABASE_URL',
  'LORETIDE_DB_TEST_DATABASE',
  'LORETIDE_DB_TEST_ROLE',
  'LORETIDE_DB_TEST_RUN_ID',
]) {
  if (!process.env[name]) throw new Error(`missing isolated test variable ${name}`);
}
if (process.env.LORETIDE_DB_TESTS !== '1') {
  throw new Error('refusing mutation run unless LORETIDE_DB_TESTS=1');
}

const applyClaim = `\tvar request idempotency.Request
\tif len(requests) > 0 {
\t\trequest = requests[0]
\t\treplay, replayed, claimErr := idempotency.Claim(ctx, tx, workspaceID, request)
\t\tif claimErr != nil {
\t\t\ts.reportFailure(ctx, workspaceID, actor, artifactID, intent.step, claimErr)
\t\t\treturn ArtifactVersion{}, claimErr
\t\t}
\t\tif replayed {
\t\t\tvar prior ArtifactVersion
\t\t\tif json.Unmarshal(replay, &prior) != nil {
\t\t\t\treturn ArtifactVersion{}, idempotency.ErrStorage
\t\t\t}
\t\t\treturn prior, nil
\t\t}
\t}
`;
const claimAfterValidation = applyClaim.replace('\tvar request idempotency.Request\n', '');

const adoptPreflight = `\tdocument, err := s.Works.Document(ctx, workspaceID, actor, current.WorkID, current.ArtifactID)
\tif err != nil {
\t\treturn SuggestionView{}, worksReadError(err)
\t}
\tif document.LatestVersionID != current.BaseVersionID {
\t\treturn SuggestionView{}, SearchConflict{Field: "base_version_id"}
\t}
\tif !document.DraftSaved {
\t\treturn SuggestionView{}, SearchConflict{Field: "draft_status"}
\t}
`;

const reviewCopyMutation = `	reviewTx, err := w.store.DB.Begin(ctx)
	if err != nil {
		return "", topicplanning.ErrStorage
	}
	defer reviewTx.Rollback(ctx)
	if _, err = reviewTx.Exec(ctx, \`INSERT INTO content_review_request
		(review_request_id, workspace_id, work_id, artifact_id, version_id, account_id, channel, snapshot, status, requested_by)
		SELECT 'mutation-copy-' || $1, workspace_id, work_id, artifact_id, $1, account_id, channel, snapshot, status, requested_by
		FROM content_review_request
		WHERE workspace_id=$2 AND artifact_id=$3 AND version_id=$4 AND status='approved'
		ORDER BY created_at LIMIT 1\`, version.VersionID, workspaceID, apply.ArtifactID, apply.BaseVersionID); err != nil {
		return "", topicplanning.ErrStorage
	}
	if err = reviewTx.Commit(ctx); err != nil {
		return "", topicplanning.ErrStorage
	}
`;

const cases = [
  {
    name: 'claim-after-base-validation',
    patches: [
      {
        file: 'server/internal/content/work-editor/version.go',
        from: applyClaim,
        to: '\tvar request idempotency.Request\n',
      },
      {
        file: 'server/internal/content/work-editor/version.go',
        from: '\tctx, err = s.audit(ctx, tx, workspaceID, actor, artifactID, intent.step)\n',
        to: claimAfterValidation + '\tctx, err = s.audit(ctx, tx, workspaceID, actor, artifactID, intent.step)\n',
      },
    ],
    runs: [
      { suite: 'work-editor', test: 'TestApplyBodyAppendsOneSuggestionVersionAndReplays', diagnostic: /replay =/ },
      { suite: 'work-editor', test: 'TestApplyBodySameKeyConcurrentWritesOneVersion', diagnostic: /concurrent ApplyBody:/ },
      { suite: 'handler', test: 'TestContentSearchSuggestionRetryReplaysTheRealWorkVersion', diagnostic: /recovered adoption =/ },
    ],
  },
  {
    name: 'adoption-preflight-after-decision-commit',
    patches: [
      {
        file: 'server/internal/content/topic-planning/search_suggestion.go',
        from: adoptPreflight,
        to: '',
      },
      {
        file: 'server/internal/content/topic-planning/search_suggestion.go',
        from: `\tif err = tx.Commit(ctx); err != nil {
\t\treturn SuggestionView{}, ErrStorage
\t}
\treturn s.completeSearchAdoption(ctx, workspaceID, actor, current, decision)
`,
        to: `\tif err = tx.Commit(ctx); err != nil {
\t\treturn SuggestionView{}, ErrStorage
\t}
${adoptPreflight}\treturn s.completeSearchAdoption(ctx, workspaceID, actor, current, decision)
`,
      },
    ],
    runs: [
      { suite: 'handler', test: 'TestContentSearchSuggestionAdoptionPreflightAndRepeat', diagnostic: /draft preflight wrote/ },
    ],
  },
  {
    name: 'deleted-workspace-maps-to-storage',
    patches: [
      {
        file: 'server/internal/handler/content_search_suggestions.go',
        from: `\tcase errors.Is(err, workspacecore.ErrNotFound), errors.Is(err, workeditor.ErrNotFound),
\t\terrors.Is(err, workeditor.ErrInvalid), errors.Is(err, topicplanning.ErrNotFound):
`,
        to: `\tcase errors.Is(err, workspacecore.ErrNotFound):
\t\treturn topicplanning.ErrStorage
\tcase errors.Is(err, workeditor.ErrNotFound), errors.Is(err, workeditor.ErrInvalid),
\t\terrors.Is(err, topicplanning.ErrNotFound):
`,
      },
    ],
    runs: [
      { suite: 'handler', test: 'TestContentSearchSuggestionAdaptersAndEndpointsAfterWorkspaceDeletion', diagnostic: /Document after the deletion =/ },
    ],
  },
  {
    name: 'adoption-copies-old-approved-review',
    patches: [
      {
        file: 'server/internal/handler/content_search_suggestions.go',
        from: '\treturn version.VersionID, nil\n',
        to: reviewCopyMutation + '\treturn version.VersionID, nil\n',
      },
    ],
    runs: [
      { suite: 'handler', test: 'TestContentSearchAdoptionKeepsOldReviewAndDeliveryOnV3', diagnostic: /v4 inherited/ },
      { suite: 'handler', test: 'TestSearchHandlersDoNotWriteDeliveryOrCallNetwork', diagnostic: /writes downstream state/ },
    ],
  },
];

function patchFiles(patches) {
  const originals = new Map();
  const updated = new Map();
  for (const patch of patches) {
    const file = path.resolve(root, patch.file);
    if (!originals.has(file)) {
      const original = readFileSync(file);
      originals.set(file, original);
      updated.set(file, original.toString('utf8'));
    }
    const current = updated.get(file);
    const occurrences = current.split(patch.from).length - 1;
    if (occurrences !== 1) {
      throw new Error(`${patch.file}: expected exactly one mutation anchor, found ${occurrences}`);
    }
    updated.set(file, current.replace(patch.from, patch.to));
  }
  for (const [file, contents] of updated) writeFileSync(file, contents, 'utf8');
  return { originals, mutated: new Map([...updated].map(([file, text]) => [file, Buffer.from(text, 'utf8')])) };
}

function restoreFiles(originals, mutated) {
  for (const [file, original] of originals) {
    const current = readFileSync(file);
    if (!current.equals(mutated.get(file))) {
      throw new Error(`refusing to overwrite an unexpected concurrent change in ${path.relative(root, file)}`);
    }
    writeFileSync(file, original);
  }
  for (const [file, original] of originals) {
    if (!readFileSync(file).equals(original)) {
      throw new Error(`byte-for-byte restore failed for ${path.relative(root, file)}`);
    }
  }
}

function commandFor(run) {
  if (run.suite === 'work-editor') {
    return {
      command: 'go',
      args: ['test', '-json', '-count=1', '-run', `^${run.test}$`, './internal/content/work-editor'],
      cwd: server,
      env: { ...process.env, LORETIDE_WORK_TEST_DATABASE_URL: dbURL },
    };
  }
  return {
    command: 'bash',
    args: ['scripts/test-go-db.sh', '--suite', run.suite, '--json', '--', '-run', `^${run.test}$`],
    cwd: root,
    env: process.env,
  };
}

function runTest(run) {
  const spec = commandFor(run);
  const result = spawnSync(spec.command, spec.args, {
    cwd: spec.cwd,
    env: spec.env,
    encoding: 'utf8',
    maxBuffer: 16 * 1024 * 1024,
    timeout: 180000,
  });
  if (result.error) throw result.error;
  const stdout = result.stdout ?? '';
  const events = stdout.split(/\r?\n/).filter((line) => line.startsWith('{')).map((line) => JSON.parse(line));
  return { status: result.status, events, output: stdout + (result.stderr ?? '') };
}

function assertGreen(run, result, label) {
  const terminal = result.events.filter((event) => event.Test === run.test && ['pass', 'fail', 'skip'].includes(event.Action));
  if (result.status !== 0 || terminal.length !== 1 || terminal[0].Action !== 'pass') {
    throw new Error(`${label}: ${run.test} did not PASS exactly once; exit=${result.status}; events=${JSON.stringify(terminal)}`);
  }
}

function assertMutationCaught(run, result, label) {
  const runEvent = result.events.some((event) => event.Test === run.test && event.Action === 'run');
  const failEvent = result.events.some((event) => event.Test === run.test && event.Action === 'fail');
  const assertionOutput = result.events
    .filter((event) => event.Test === run.test && event.Action === 'output')
    .map((event) => event.Output ?? '')
    .join('\n');
  if (result.status === 0 || !runEvent || !failEvent || !run.diagnostic.test(assertionOutput) || !/\w+_test\.go:\d+:/.test(assertionOutput)) {
    throw new Error(`${label}: ${run.test} was not caught by its expected test assertion (compile errors and skips do not count); output:\n${assertionOutput.slice(-5000)}`);
  }
  console.log(`RED ${label}: ${run.test} failed at a test assertion`);
}

for (const mutation of cases) {
  for (const run of mutation.runs) assertGreen(run, runTest(run), `${mutation.name} baseline`);
  const patched = patchFiles(mutation.patches);
  let mutationError;
  try {
    for (const run of mutation.runs) assertMutationCaught(run, runTest(run), mutation.name);
  } catch (error) {
    mutationError = error;
  } finally {
    restoreFiles(patched.originals, patched.mutated);
    for (const run of mutation.runs) assertGreen(run, runTest(run), `${mutation.name} restored`);
    console.log(`RESTORED GREEN ${mutation.name}: all source bytes match the pre-mutation snapshot`);
  }
  if (mutationError) throw mutationError;
}

console.log('T076 mutation evidence complete: all four mutants were assertion-caught and restored green.');
