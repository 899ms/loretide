"use client";

import { useState } from "react";
import { useSettleOpDiagProposal, useWriteOpDiagTodo, type OpDiagProposal, type OpDiagTodo } from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { SettingsCard, SettingsSection } from "@multica/views/settings/layout";
import { useT } from "@multica/views/i18n";

export function FollowUp({ wsId, todos, proposals, todosLoading, proposalsLoading, todosFailed, proposalsFailed, refreshTodos, refreshProposals }: {
  wsId: string;
  todos: OpDiagTodo[];
  proposals: OpDiagProposal[];
  todosLoading: boolean;
  proposalsLoading: boolean;
  todosFailed: boolean;
  proposalsFailed: boolean;
  refreshTodos: () => void;
  refreshProposals: () => void;
}) {
  const { t } = useT("common");
  const writeTodo = useWriteOpDiagTodo(wsId); const settle = useSettleOpDiagProposal(wsId);
  const [edits, setEdits] = useState<Record<string, { title: string; note: string }>>({});
  const [conflict, setConflict] = useState(false);
  const [todoConflictId, setTodoConflictId] = useState("");
  const edit = (todo: OpDiagTodo) => edits[todo.todoId] ?? { title: todo.title, note: todo.note };
  const update = (todo: OpDiagTodo, patch: Partial<{ title: string; note: string }>) => setEdits((current) => ({ ...current, [todo.todoId]: { ...edit(todo), ...patch } }));
  const reviseTodo = (todo: OpDiagTodo, body: Record<string, unknown>) => {
    setTodoConflictId("");
    writeTodo.mutate({ path: `todos/${encodeURIComponent(todo.todoId)}/revisions`, body: { base_revision: todo.revision, ...body } }, {
      onError: (error) => { if (isConflict(error)) { setTodoConflictId(todo.todoId); refreshTodos(); } },
    });
  };
  const saveTodo = (todo: OpDiagTodo) => reviseTodo(todo, edit(todo));
  const changeTodoState = (todo: OpDiagTodo, state: "open" | "done" | "dropped") => reviseTodo(todo, { state });
  const settleProposal = (proposal: OpDiagProposal, action: "confirm" | "dismiss") => {
    setConflict(false);
    settle.mutate({ path: `profile-proposals/${encodeURIComponent(proposal.proposalId)}/${action}`, body: action === "confirm" ? { base_revision_id: proposal.baseRevisionId } : {} }, {
      onError: (error) => { if (isConflict(error)) { setConflict(true); refreshProposals(); } },
    });
  };
  return <SettingsSection title={t(($) => $.contentOperatingDiagnosis.blocks.followUp)}><div className="space-y-4">
    <SettingsCard><div className="space-y-3"><h3 className="text-sm font-medium">{t(($) => $.contentOperatingDiagnosis.todos)}</h3>
      {todosLoading ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loading)}</p> : todosFailed ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loadFailed)}</p> : todos.length === 0 ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.empty)}</p> : todos.filter((todo) => !todo.voided).map((todo) => <div className="space-y-2 border-t pt-3" key={todo.todoId}>
        <div className="grid gap-2 sm:grid-cols-[1fr_12rem]"><Input value={edit(todo).title} onChange={(event) => update(todo, { title: event.target.value })} /><Select items={[{ value: "open", label: t(($) => $.contentOperatingDiagnosisDetails.todoOpen) }, { value: "done", label: t(($) => $.contentOperatingDiagnosisDetails.todoDone) }, { value: "dropped", label: t(($) => $.contentOperatingDiagnosisDetails.todoDropped) }]} value={todo.state === "open" || todo.state === "done" || todo.state === "dropped" ? todo.state : ""} onValueChange={(value) => { if (value === "open" || value === "done" || value === "dropped") changeTodoState(todo, value); }}><SelectTrigger><SelectValue placeholder={todo.state === "unknown" ? t(($) => $.contentOperatingDiagnosis.unknown) : undefined} /></SelectTrigger><SelectContent><SelectItem value="open">{t(($) => $.contentOperatingDiagnosisDetails.todoOpen)}</SelectItem><SelectItem value="done">{t(($) => $.contentOperatingDiagnosisDetails.todoDone)}</SelectItem><SelectItem value="dropped">{t(($) => $.contentOperatingDiagnosisDetails.todoDropped)}</SelectItem></SelectContent></Select></div>
        <Textarea value={edit(todo).note} onChange={(event) => update(todo, { note: event.target.value })} placeholder={t(($) => $.contentOperatingDiagnosis.note)} />
        <p className="text-sm text-muted-foreground">{todo.originKind} · {todo.originReportId} · {todo.originVersionNo} · {todo.originGapKey || todo.originDecisionId}</p>
        {todoConflictId === todo.todoId && <p className="text-sm text-muted-foreground" role="alert">{t(($) => $.contentOperatingDiagnosisDetails.todoConflict)}</p>}
        <Button size="sm" variant="outline" disabled={writeTodo.isPending || (!edit(todo).title.trim())} onClick={() => saveTodo(todo)}>{t(($) => $.contentOperatingDiagnosisDetails.saveTodo)}</Button>
      </div>)}
      {writeTodo.isError && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.failed)}</p>}
    </div></SettingsCard>
    <SettingsCard><div className="space-y-3"><h3 className="text-sm font-medium">{t(($) => $.contentOperatingDiagnosis.proposals)}</h3>
      {proposalsLoading ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loading)}</p> : proposalsFailed ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loadFailed)}</p> : proposals.length === 0 ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.empty)}</p> : proposals.map((proposal) => <div className="space-y-2 border-t pt-3" key={proposal.proposalId}>
        {proposal.items.map((item) => <p className="text-sm" key={item.field}>{profileFieldLabel(t, item.field)}: {item.currentValue || item.currentStatus} → {item.proposedValue}</p>)}
        <p className="text-sm text-muted-foreground">{proposalStateLabel(t, proposal.state)}{proposal.appliedRevisionId && ` · ${proposal.appliedRevisionId}`}</p>
        {proposal.state === "proposed" && !proposal.voided && <div className="flex gap-2"><Button size="sm" variant="outline" disabled={settle.isPending || !proposal.baseIsCurrent} onClick={() => settleProposal(proposal, "confirm")}>{t(($) => $.contentOperatingDiagnosis.confirm)}</Button><Button size="sm" variant="outline" disabled={settle.isPending} onClick={() => settleProposal(proposal, "dismiss")}>{t(($) => $.contentOperatingDiagnosis.dismiss)}</Button></div>}
      </div>)}
      {conflict && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosisDetails.proposalConflict)}</p>}
      {settle.isError && !conflict && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.failed)}</p>}
    </div></SettingsCard>
  </div></SettingsSection>;
}

function isConflict(error: unknown) {
  if (!error || typeof error !== "object") return false;
  if ("status" in error && error.status === 409) return true;
  return "message" in error && typeof error.message === "string" && /409|base_revision_id/i.test(error.message);
}

function profileFieldLabel(t: ReturnType<typeof useT<"common">>["t"], field: string) {
  const fields = ["audience", "common_questions", "experience", "positioning", "content_pillars", "expression_style", "forbidden_expressions", "content_goals"];
  return fields.includes(field) ? t(`contentOperatingDiagnosisDetails.profileFields.${field}` as never) : t(($) => $.contentOperatingDiagnosis.unknown);
}

function proposalStateLabel(t: ReturnType<typeof useT<"common">>["t"], state: string) {
  if (state === "proposed") return t(($) => $.contentOperatingDiagnosisDetails.proposalProposed);
  if (state === "confirmed") return t(($) => $.contentOperatingDiagnosisDetails.proposalConfirmed);
  if (state === "dismissed") return t(($) => $.contentOperatingDiagnosisDetails.proposalDismissed);
  return t(($) => $.contentOperatingDiagnosis.unknown);
}
