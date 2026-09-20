# Issue #114 · Phase 2 分类清单：`server/internal/handler`

## 这份清单是什么

`internal/handler` 的 296 个测试文件按 `dbtest` 构建约束分成三类。每一条的依据都是**编译器可验证**的，不是文本检索：

- **tagged**（200 个）：文件里每一个 `Test*` 都触及数据库，整文件加 `//go:build dbtest`。
- **split**（57 个）：文件里既有触及数据库的测试、也有不触及的。触及的那些移进同名的 `*_dbtest_test.go`，其余留在无标签文件里，供默认路径运行。
- **untagged**（39 个）：没有任何声明触及数据库，原样保留。

## 判定方法

「触及数据库」的定义是**对数据库根符号的可达性**，不是「名字里有 db」：

根符号 = `testHandler`、`testPool`、`testUserID`、`testWorkspaceID`、`testRuntimeID`、`dbfx`、`handlerSuite`、`handlerTestEmail`、`handlerTestName`、`handlerTestWorkspaceSlug`、`TestMain`、`setupHandlerTestFixture`、`cleanupHandlerTestFixture`。它们全部由 `TestMain` 在连上隔离库之后初始化，包里任何一条到达数据库的路径都从它们出发。

一个顶层声明只要引用了根符号，或引用了另一个已被判定为「触及」的声明，它本身就是「触及」的；对全包求不动点闭包。**闭包的正确性由编译器兜底**：无标签构建里只要还剩一处引用了被标签挡掉的符号，`go vet ./internal/handler` 就编译不过。本清单里的每一条都经过了这一关。

只有「还剩下至少一个不触及数据库的 `Test*`」的文件才拆分。把一个孤立的辅助函数从整体数据库文件里拆出来不会多跑一个测试，只会多一个文件；反过来，若某个整体打标签文件里的辅助函数仍被无标签构建引用，它会被改判为 split（这一步同样由编译器验证）。

## 验证

| 项 | 结果 |
|---|---|
| `go vet ./internal/handler`（无标签） | 通过 |
| `go vet -tags=dbtest ./internal/handler` | 通过 |
| 无任何 `LORETIDE_DB_TEST_*` 配置下 `go test ./internal/handler` | **退出 0，1066 PASS / 40 SKIP / 0 FAIL**（改动前：整包不执行） |
| `scripts/test-go-db.sh --suite handler`（隔离库） | **3808 PASS / 47 SKIP / 0 FAIL**，与改动前基线逐数字一致 |

40 条 SKIP 是 `REDIS_TEST_URL` 未配置的既有 Redis 用例；dbtest 模式下的 47 条同理。

两个数字的关系：dbtest 模式跑的是全部 3808 条，无标签模式跑其中 1066 条，**没有任何测试因为这次分类而消失**。

---

## split（拆分，57）

| 文件 | 依据 |
|---|---|
| `agent_access_test.go` | 12/16 个声明触及数据库；首个：privateAgentTestFixture: references root testPool |
| `agent_composio_allowlist_test.go` | 6/7 个声明触及数据库；首个：allowlistFixture: references root testPool |
| `agent_conversation_starters_test.go` | 1/2 个声明触及数据库；首个：TestAgentConversationStartersRoundTrip: references root testHandler |
| `agent_env_permission_test.go` | 5/6 个声明触及数据库；首个：agentEnvOwnerFixture: references root testPool |
| `agent_test.go` | 19/23 个声明触及数据库；首个：TestListWorkspaceAgentTaskSnapshot: references root testRuntimeID |
| `agent_thinking_test.go` | 14/16 个声明触及数据库；首个：TestCreateAgent_ThinkingLevel_ValidationConsistency: references root testPool |
| `attachment_capability_test.go` | 8/15 个声明触及数据库；首个：installProxyModeStorage: references root testHandler |
| `attachment_url_mode_test.go` | 6/9 个声明触及数据库；首个：loadTestAttachment: references root testHandler |
| `attribution_response_test.go` | 2/5 个声明触及数据库；首个：TestHydrateTaskAttributionsFillsUserRef: references root testHandler |
| `auth_google_error_code_test.go` | 1/8 个声明触及数据库；首个：TestGoogleLoginSuccessfulExistingUser: references root testHandler |
| `auth_signup_test.go` | 1/9 个声明触及数据库；首个：TestEmailCodeAllowlistErrors: references root testHandler |
| `chat_title_test.go` | 12/16 个声明触及数据库；首个：withStubLLM: references root testHandler |
| `client_usage_test.go` | 1/4 个声明触及数据库；首个：TestUpsertClientUsageKeepsRuntimeSnapshotOnActivityRefresh: references root testUserID |
| `comment_list_test.go` | 32/40 个声明触及数据库；首个：newCommentListFixture: references root testUserID |
| `config_test.go` | 14/17 个声明触及数据库；首个：TestGetConfigReportsCdnSignedMode: references root testHandler |
| `contact_sales_test.go` | 10/14 个声明触及数据库；首个：clearContactSalesForEmail: references root testPool |
| `daemon_batch_claim_test.go` | 9/13 个声明触及数据库；首个：seedQueuedIssueTask: references root testPool |
| `daemon_chat_resume_fallback_test.go` | 2/7 个声明触及数据库；首个：TestClaimTaskChatCompletePointerSkipsSessionFallbackQuery: references root testWorkspaceID |
| `daemon_comment_delivery_test.go` | 23/40 个声明触及数据库；首个：createCommentDeliveryFixture: references root testWorkspaceID |
| `daemon_test.go` | 103/119 个声明触及数据库；首个：setHandlerTestWorkspaceRepos: references root dbfx |
| `dashboard_test.go` | 22/29 个声明触及数据库；首个：TestDashboardEndpoints: references root testPool |
| `dingtalk_test.go` | 16/19 个声明触及数据库；首个：wireDingTalkInstallService: references root testHandler |
| `file_test.go` | 42/78 个声明触及数据库；首个：createHandlerTestChatSession: references root testWorkspaceID |
| `github_test.go` | 34/55 个声明触及数据库；首个：TestWebhook_MergedPR_AdvancesLinkedIssueToDone: references root testPool |
| `handler_test.go` | 118/123 个声明触及数据库；首个：testHandler: root symbol testHandler |
| `heartbeat_scheduler_test.go` | 9/13 个声明触及数据库；首个：TestBatchedHeartbeatScheduler_CoalescesAndFlushes: references root testUserID |
| `heartbeat_test.go` | 12/24 个声明触及数据库；首个：readRuntimeRow: references root testPool |
| `inbox_test.go` | 7/9 个声明触及数据库；首个：inboxRequest: references root testUserID |
| `issue_channel_media_merge_test.go` | 3/8 个声明触及数据库；首个：TestUpdateIssueMergesChannelMediaAppendedAfterEditorBase: references root testPool |
| `issue_child_done_batch_stage_test.go` | 1/7 个声明触及数据库；首个：TestBatchChildDonePreservesRepresentativeAndParentOrder: references root testUserID |
| `issue_move_test.go` | 1/5 个声明触及数据库；首个：TestMoveIssueRejectsUnsafeInputs: references root testUserID |
| `issue_table_query_test.go` | 12/24 个声明触及数据库；首个：TestIssueTableExplicitEmptyAssigneesMatchesNone: references root testWorkspaceID |
| `lark_test.go` | 7/14 个声明触及数据库；首个：TestListActiveLarkInstallations_SkipsOrphans: references root testPool |
| `mika_agent_test.go` | 6/10 个声明触及数据库；首个：createMika: references root testHandler |
| `plugin_mcp_test.go` | 5/9 个声明触及数据库；首个：installMCPPlugin: references root testHandler |
| `plugin_surface_test.go` | 1/10 个声明触及数据库；首个：TestServePluginSurfaceRejectsConfiguredAppOriginBeforeOpeningToken: references pluginHandlerRequest (plugin_test.go) |
| `project_resource_test.go` | 17/21 个声明触及数据库；首个：TestProjectResourceLifecycle: references root testWorkspaceID |
| `property_test.go` | 19/30 个声明触及数据库；首个：createTestProperty: references root testHandler |
| `runtime_local_skills_redis_store_test.go` | 1/18 个声明触及数据库；首个：TestRedisLocalSkillImportStore_CompletePreservesFiles: references root testWorkspaceID |
| `runtime_local_skills_test.go` | 15/22 个声明触及数据库；首个：newRequestAsUser: references root testWorkspaceID |
| `runtime_model_catalog_fallback_test.go` | 5/6 个声明触及数据库；首个：reportModelList: references root testHandler |
| `runtime_model_catalog_test.go` | 1/20 个声明触及数据库；首个：TestInitiateListModels_ForceSkipsCatalogCache: references root testHandler |
| `runtime_unbind_delete_test.go` | 14/19 个声明触及数据库；首个：TestDeleteAgentRuntime_StructuredConflict: references root testHandler |
| `runtime_visibility_test.go` | 13/15 个声明触及数据库；首个：runtimeVisibilityFixture: references root testWorkspaceID |
| `search_timeout_test.go` | 2/5 个声明触及数据库；首个：TestRunSearchQuery_StatementTimeoutFires: references root testPool |
| `skill_import_archive_test.go` | 4/18 个声明触及数据库；首个：newSkillArchiveImportRequest: references root testWorkspaceID |
| `skill_import_duplicate_test.go` | 3/5 个声明触及数据库；首个：TestExistingSkillIdentityByNameReturnsIDAndName: references root testHandler |
| `skill_refresh_test.go` | 13/18 个声明触及数据库；首个：createImportTargetSkillWithConfig: references root testPool |
| `source_context_integration_test.go` | 3/4 个声明触及数据库；首个：TestRetrySourceContextQuickCreateReturnsIssueLimitRecovery: references root testWorkspaceID |
| `squad_briefing_test.go` | 15/18 个声明触及数据库；首个：seedSquadForBriefing: references root testHandler |
| `squad_comment_trigger_test.go` | 13/16 个声明触及数据库；首个：shouldEnqueueSquadLeaderOnCommentForTest: references root testHandler |
| `task_message_batch_test.go` | 6/8 个声明触及数据库；首个：seedBatchTask: references root dbfx |
| `task_message_truncation_test.go` | 1/2 个声明触及数据库；首个：TestCreateTaskMessagesKeepsTruncationTriState: references root testHandler |
| `workspace_delete_fence_test.go` | 8/10 个声明触及数据库；首个：waitForBlockedWriter: references root testPool |
| `workspace_delete_task_discovery_test.go` | 10/12 个声明触及数据库；首个：newWorkspaceDeletePathFixture: references root testPool |
| `workspace_test.go` | 24/31 个声明触及数据库；首个：TestCreateWorkspace_RejectsReservedSlug: references root testHandler |
| `worktree_claim_gate_test.go` | 12/19 个声明触及数据库；首个：TestWorktreeDeliveryMetadataRoundTripsThroughBothTerminalPaths: references root testHandler |

---

## untagged（保持无标签，39）

| 文件 | 依据 |
|---|---|
| `actor_guards_test.go` | 没有任何声明触及数据库根符号（0/4） |
| `admission_test.go` | 没有任何声明触及数据库根符号（0/2） |
| `agent_runtime_config_mask_test.go` | 没有任何声明触及数据库根符号（0/7） |
| `agent_work_dir_test.go` | 没有任何声明触及数据库根符号（0/3） |
| `auth_session_test.go` | 没有任何声明触及数据库根符号（0/1） |
| `autopilot_webhook_iprl_test.go` | 没有任何声明触及数据库根符号（0/9） |
| `autopilot_webhook_test.go` | 没有任何声明触及数据库根符号（0/23） |
| `chat_wait_reason_test.go` | 没有任何声明触及数据库根符号（0/1） |
| `comment_decision_test.go` | 没有任何声明触及数据库根符号（0/3） |
| `comment_reply_authz_test.go` | 没有任何声明触及数据库根符号（0/1） |
| `daemon_chat_prompt_test.go` | 没有任何声明触及数据库根符号（0/5） |
| `featureflag_test.go` | 没有任何声明触及数据库根符号（0/3） |
| `handler_writejson_test.go` | 没有任何声明触及数据库根符号（0/2） |
| `integrations_composio_test.go` | 没有任何声明触及数据库根符号（0/27） |
| `issue_child_done_stage_test.go` | 没有任何声明触及数据库根符号（0/10） |
| `issue_payload_shape_test.go` | 没有任何声明触及数据库根符号（0/7） |
| `mcp_overlay_test.go` | 没有任何声明触及数据库根符号（0/9） |
| `mika_onboarding_opening_test.go` | 没有任何声明触及数据库根符号（0/5） |
| `mika_onboarding_test.go` | 没有任何声明触及数据库根符号（0/7） |
| `parse_since_param_test.go` | 没有任何声明触及数据库根符号（0/2） |
| `quick_action_test.go` | 没有任何声明触及数据库根符号（0/6） |
| `runtime_liveness_store_test.go` | 没有任何声明触及数据库根符号（0/4） |
| `runtime_model_catalog_redis_cache_test.go` | 没有任何声明触及数据库根符号（0/4） |
| `runtime_models_redis_store_test.go` | 没有任何声明触及数据库根符号（0/8） |
| `runtime_models_test.go` | 没有任何声明触及数据库根符号（0/4） |
| `runtime_redis_keys_test.go` | 没有任何声明触及数据库根符号（0/2） |
| `runtime_update_redis_store_test.go` | 没有任何声明触及数据库根符号（0/10） |
| `runtime_update_test.go` | 没有任何声明触及数据库根符号（0/4） |
| `search_test.go` | 没有任何声明触及数据库根符号（0/31） |
| `skill_path_portability_test.go` | 没有任何声明触及数据库根符号（0/1） |
| `skill_test.go` | 没有任何声明触及数据库根符号（0/42） |
| `slack_test.go` | 没有任何声明触及数据库根符号（0/1） |
| `source_context_state_test.go` | 没有任何声明触及数据库根符号（0/6） |
| `telegram_test.go` | 没有任何声明触及数据库根符号（0/3） |
| `trigger_test.go` | 没有任何声明触及数据库根符号（0/9） |
| `webhook_rate_limiter_test.go` | 没有任何声明触及数据库根符号（0/10） |
| `wecom_install_error_matrix_test.go` | 没有任何声明触及数据库根符号（0/2） |
| `wecom_web_test.go` | 没有任何声明触及数据库根符号（0/5） |
| `workspace_mcp_test.go` | 没有任何声明触及数据库根符号（0/13） |

---

## tagged（整文件加标签，200）

| 文件 | 依据 |
|---|---|
| `activity_test.go` | 11/11 个声明触及数据库；首个：fetchTimeline: references root testHandler |
| `admission_security_mul4525_test.go` | 3/4 个声明触及数据库；首个：seedSecurityTestOwner: references root testWorkspaceID |
| `agent_builder_test.go` | 34/35 个声明触及数据库；首个：TestCreateAgentBuilderSessionCreatesIsolatedHiddenBuilder: references root testHandler |
| `agent_concurrency_test.go` | 2/2 个声明触及数据库；首个：TestCreateAgent_MaxConcurrentTasksBoundsAndDefault: references root testPool |
| `agent_permission_test.go` | 17/17 个声明触及数据库；首个：createPermissionTestMember: references root testPool |
| `agent_runtime_required_test.go` | 6/6 个声明触及数据库；首个：TestCommentMention_UnboundAgentReportsRuntimeRequired: references root testHandler |
| `agent_runtime_skills_broadcast_test.go` | 1/1 个声明触及数据库；首个：TestSetAgentRuntimeSkillEnabledBroadcastsAgentStatus: references root testUserID |
| `agent_task_usage_response_test.go` | 2/2 个声明触及数据库；首个：TestListAgentTasksHydratesUsage: references root dbfx |
| `assign_invoke_denial_test.go` | 1/2 个声明触及数据库；首个：TestAssignAgent_DenialReasonNamesPermissionNotMode: references root testWorkspaceID |
| `autopilot_acting_member_test.go` | 10/13 个声明触及数据库；首个：actingCaller.request: references newRequestAs (agent_access_test.go) |
| `autopilot_attribution_transfer_test.go` | 10/10 个声明触及数据库；首个：seedTransferMember: references root testWorkspaceID |
| `autopilot_broadcast_redaction_test.go` | 1/1 个声明触及数据库；首个：TestAutopilotTriggerBroadcastsCarryNoWebhookCredential: references root testPool |
| `autopilot_cron_preview_test.go` | 7/8 个声明触及数据库；首个：cronPreviewRequest: references newRequest (handler_test.go) |
| `autopilot_list_test.go` | 6/6 个声明触及数据库；首个：insertListTestAutopilot: references root testWorkspaceID |
| `autopilot_manual_trigger_invoker_test.go` | 10/12 个声明触及数据库；首个：triggerInvokerFixture: references root testWorkspaceID |
| `autopilot_mention_authority_test.go` | 13/14 个声明触及数据库；首个：newAutopilotDelegationFixture: references root testHandler |
| `autopilot_permissions_test.go` | 12/12 个声明触及数据库；首个：createPlainMember: references root testPool |
| `autopilot_private_leader_test.go` | 7/7 个声明触及数据库；首个：TestCreateAutopilot_SquadPrivateLeader_PlainMemberBlocked: references root testHandler |
| `autopilot_quota_handler_test.go` | 1/1 个声明触及数据库；首个：TestAutopilotQuotaManualAndWebhookEnforcement: references root testHandler |
| `autopilot_subscriber_test.go` | 14/14 个声明触及数据库；首个：installAutopilotSubscriberInsertFailure: references root testPool |
| `autopilot_trigger_creator_backfill_test.go` | 1/2 个声明触及数据库；首个：TestMigration467BackfillsTriggerCreatorFromAutopilot: references root testUserID |
| `autopilot_trigger_error_test.go` | 1/1 个声明触及数据库；首个：TestTriggerAutopilot_InternalFailureDoesNotEchoError: references root testPool |
| `autopilot_webhook_handler_test.go` | 41/42 个声明触及数据库；首个：createWebhookTestAgent: references root testWorkspaceID |
| `avatar_test.go` | 34/36 个声明触及数据库；首个：withAvatarStorage: references root testHandler |
| `cancel_task_by_user_test.go` | 26/26 个声明触及数据库；首个：taskStatus: references root dbfx |
| `channel_context_lock_order_test.go` | 1/9 个声明触及数据库；首个：TestChannelContextLockOrder: references root testWorkspaceID |
| `channel_new_e2e_test.go` | 6/22 个声明触及数据库；首个：TestChannelClearCommandE2EStartsFreshProviderSession: references root testHandler |
| `channel_title_regression_test.go` | 1/1 个声明触及数据库；首个：TestChannelChatTitle_UsesCurrentSourceForLLMAndPublishesAfterCAS: references root testHandler |
| `chat_archive_cancel_test.go` | 17/19 个声明触及数据库；首个：TestArchivingAChannelBoundChatSessionCancelsItsQueuedTasks: references root testHandler |
| `chat_attachment_reply_test.go` | 9/9 个声明触及数据库；首个：seedRunningChatTask: references root testPool |
| `chat_channel_command_visibility_test.go` | 5/5 个声明触及数据库；首个：insertChatVisibilityMessage: references insertChatMessageRole (chat_channel_command_visibility_test.go) |
| `chat_channel_debounce_boundary_test.go` | 4/5 个声明触及数据库；首个：appendChannelUserMessage: references root testPool |
| `chat_delete_cancel_broadcast_test.go` | 1/1 个声明触及数据库；首个：TestDeleteChatSession_BroadcastsTaskCancelled: references root testUserID |
| `chat_draft_restore_race_test.go` | 6/7 个声明触及数据库；首个：seedDraftRestoreRaceFixture: references root testUserID |
| `chat_draft_restore_test.go` | 9/10 个声明触及数据库；首个：seedDraftRestore: references root testPool |
| `chat_history_test.go` | 19/22 个声明触及数据库；首个：TestChatHistoryScopePassesImmutableContextGeneration: references newRequest (handler_test.go) |
| `chat_input_ownership_test.go` | 31/37 个声明触及数据库；首个：setupDirectChatSession: references root testPool |
| `chat_pending_tasks_test.go` | 19/22 个声明触及数据库；首个：chatPendingCtxAs: references root testHandler |
| `chat_project_context_test.go` | 6/6 个声明触及数据库；首个：createChatProjectTestProject: references root testPool |
| `chat_test.go` | 18/18 个声明触及数据库；首个：withChatTestWorkspaceCtx: references root testWorkspaceID |
| `claim_project_context_test.go` | 7/15 个声明触及数据库；首个：foreignWorkspaceWithProject: references root dbfx |
| `cloud_billing_test.go` | 25/28 个声明触及数据库；首个：TestCloudBillingProxiesForwardCorrectly: references root testHandler |
| `cloud_runtime_test.go` | 7/10 个声明触及数据库；首个：useCloudRuntimeProxy: references root testHandler |
| `comment_cap_thread_test.go` | 13/14 个声明触及数据库；首个：seedCommentRow: references root testWorkspaceID |
| `comment_content_sanitize_test.go` | 2/2 个声明触及数据库；首个：TestCreateComment_StripsNullBytesInsteadOf500: references root testPool |
| `comment_duplicate_enqueue_race_test.go` | 16/17 个声明触及数据库；首个：dupRaceFixture: references root testPool |
| `comment_fold_test.go` | 9/12 个声明触及数据库；首个：setCommentResolvedAt: references root testUserID |
| `comment_merge_failclosed_test.go` | 1/1 个声明触及数据库；首个：TestMergeCommentIntoPendingTask_FailClosedKeepsOriginalSnapshot: references root testWorkspaceID |
| `comment_reconcile_test.go` | 15/16 个声明触及数据库；首个：completeTaskViaHandler: references root testHandler |
| `comment_reply_authz_handler_test.go` | 3/3 个声明触及数据库；首个：TestCreateComment_TriggeredTaskRejectsTopLevelComment: references root testHandler |
| `comment_resolve_test.go` | 8/11 个声明触及数据库；首个：resolveCommentHTTP: references root testHandler |
| `comment_thread_queue_test.go` | 2/2 个声明触及数据库；首个：TestCommentThreadQueuesMergeAndExecuteIndependently: references root testUserID |
| `comment_touch_issue_test.go` | 9/9 个声明触及数据库；首个：TestCreateComment_BumpsIssueActivity: references root testHandler |
| `comment_trigger_outcomes_test.go` | 5/6 个声明触及数据库；首个：TestCreateComment_MixedMentionSurfacesPartialTriggerOutcomes: references root testHandler |
| `comment_trigger_preview_test.go` | 32/33 个声明触及数据库；首个：createCommentTriggerPreviewIssue: references root testPool |
| `comment_worker_handoff_test.go` | 5/7 个声明触及数据库；首个：claimWorkerReplyRun: references root testWorkspaceID |
| `content_account_logging_test.go` | 5/7 个声明触及数据库；首个：TestAStorageFailureListingAccountsIsLoggedWithItsCause: references root testHandler |
| `content_account_profile_test.go` | 7/8 个声明触及数据库；首个：setExpressionProfile: references root testUserID |
| `content_account_revision_test.go` | 10/10 个声明触及数据库；首个：setPrompt: references root testHandler |
| `content_account_scope_run_test.go` | 3/4 个声明触及数据库；首个：TestADiagnosticRunDoesNotWriteBackTheAccountsScope: references root testHandler |
| `content_account_scope_test.go` | 15/17 个声明触及数据库；首个：setScope: references root testUserID |
| `content_account_test.go` | 10/10 个声明触及数据库；首个：accountWorkspace: references root testPool |
| `content_account_write_fence_test.go` | 4/4 个声明触及数据库；首个：TestAccountWritesAreRefusedAfterTheWorkspaceIsDeleted: references root testHandler |
| `content_diagnostics_authz_test.go` | 6/8 个声明触及数据库；首个：diagnosticsWorkspace: references root dbfx |
| `content_diagnostics_linkage_test.go` | 6/6 个声明触及数据库；首个：simulateRun: references root testUserID |
| `content_diagnostics_test.go` | 20/22 个声明触及数据库；首个：TestContentDiagnosticsAuthAndFaultGate: references root testHandler |
| `content_grant_prompt_test.go` | 3/3 个声明触及数据库；首个：TestTwoAccountsWithIdenticalPersonaPromptsStillCannotReadEachOther: references root testHandler |
| `content_revision_fence_test.go` | 4/4 个声明触及数据库；首个：TestAWriteIsRefusedAfterTheWorkspaceIsDeleted: references root testHandler |
| `content_topic_fence_test.go` | 1/1 个声明触及数据库；首个：TestContentTopicWritesAreFencedByWorkspaceDeletion: references root testPool |
| `cross_issue_originator_test.go` | 14/17 个声明触及数据库；首个：newCrossIssueChain: references root dbfx |
| `daemon_batch_claim_finalize_test.go` | 10/11 个声明触及数据库；首个：TestClaimTasksByRuntime_MaxTasksZeroClaimsNothing: references root testHandler |
| `daemon_claim_attachment_batch_test.go` | 6/12 个声明触及数据库；首个：seedLegacyChatUserMessage: references root dbfx |
| `daemon_claim_cancelled_session_test.go` | 17/20 个声明触及数据库；首个：TestClaimTask_ChatResumesCancelledTurnSession: references root dbfx |
| `daemon_claim_channel_type_test.go` | 14/15 个声明触及数据库；首个：seedChannelBinding: references seedChannelBindingOfChatType (daemon_claim_channel_type_test.go) |
| `daemon_claim_continuity_gap_test.go` | 3/4 个声明触及数据库；首个：claimOneTaskForRuntime: references root testHandler |
| `daemon_claim_issue_status_test.go` | 2/2 个声明触及数据库；首个：TestClaimTaskByRuntime_PopulatesIssueStatusCatalog: references root dbfx |
| `daemon_claim_workspace_mcp_test.go` | 6/6 个声明触及数据库；首个：claimAgentMcpConfigForTest: references root testHandler |
| `daemon_comment_workspace_scope_test.go` | 5/5 个声明触及数据库；首个：insertCommentForScopeTest: references root testPool |
| `daemon_context_exhausted_test.go` | 2/2 个声明触及数据库；首个：TestCompleteTask_ContextExhaustionFromOlderDaemonIsRecordedAsFailed: references root testHandler |
| `daemon_deregister_batch_test.go` | 4/9 个声明触及数据库；首个：newRuntimeBatchReadFailureHandler: references root testHandler |
| `daemon_legacy_skill_redirect_test.go` | 1/1 个声明触及数据库；首个：TestClaimTaskByRuntime_LegacySkillRedirectFollowsTheCapability: references root testPool |
| `daemon_rpc_test.go` | 2/2 个声明触及数据库；首个：TestDaemonRPCHandler_TasksClaim: references root testPool |
| `daemon_runtime_access_test.go` | 3/3 个声明触及数据库；首个：TestClaimTaskByRuntime_OwnerlessAgentOnPrivateRuntimeFailsExplicitly: references root testWorkspaceID |
| `daemon_skill_load_failure_test.go` | 4/9 个声明触及数据库；首个：newSkillFileReadFailureHandler: references root testHandler |
| `daemon_skill_resolve_scope_test.go` | 8/18 个声明触及数据库；首个：newSpyHandler: references root testHandler |
| `daemon_task_lookup_test.go` | 4/6 个声明触及数据库；首个：TestGetTaskStatus_WorkspaceLookupFailure_Returns500: references root dbfx |
| `daemon_ws_test.go` | 4/4 个声明触及数据库；首个：TestBuildDaemonWebSocketIdentitySeedsBatchRuntimeLeases: references root dbfx |
| `dashboard_agent_visibility_test.go` | 2/5 个声明触及数据库；首个：TestDashboardPerAgentRollupsFoldRestrictedAgents: references root testPool |
| `delegated_failure_recovery_test.go` | 1/1 个声明触及数据库；首个：TestUpdateComment_RequeuesDelegatedFailureRecoverySurvivor: references root testPool |
| `fail_task_successor_test.go` | 9/9 个声明触及数据库；首个：TestFailTask_SkipsAutoRetryWhenManualRerunAlreadyQueued: references root testHandler |
| `feedback_test.go` | 8/8 个声明触及数据库；首个：TestCreateFeedbackHappyPath: references root testHandler |
| `invitation_test.go` | 16/36 个声明触及数据库；首个：useSeatCapacity: references root testHandler |
| `issue_agent_create_e2e_test.go` | 3/3 个声明触及数据库；首个：createPrivateAgentOwnedBy: references root testWorkspaceID |
| `issue_agent_create_origin_test.go` | 2/2 个声明触及数据库；首个：TestCreateIssue_AgentCreate_StampsActingTaskOrigin: references root testWorkspaceID |
| `issue_assignee_types_test.go` | 1/1 个声明触及数据库；首个：TestListIssues_AssigneeTypesFilter: references root testUserID |
| `issue_autopilot_assignment_authority_test.go` | 12/15 个声明触及数据库；首个：newRunOnlyAutopilotFixture: references root dbfx |
| `issue_batch_test.go` | 12/13 个声明触及数据库；首个：TestBatchUpdateNoMutationReturnsZero: references root testHandler |
| `issue_cancel_status_no_cancel_test.go` | 3/4 个声明触及数据库；首个：insertIssueTaskWithStatus: references root testPool |
| `issue_child_done_resolver_test.go` | 7/10 个声明触及数据库；首个：TestChildDoneStatusResolver: references root dbfx |
| `issue_child_done_test.go` | 23/24 个声明触及数据库；首个：newChildDoneFixture: references root testPool |
| `issue_children_for_parents_test.go` | 12/15 个声明触及数据库；首个：newChildrenBatchFixture: references root testWorkspaceID |
| `issue_collection_revision_test.go` | 1/1 个声明触及数据库；首个：TestIssueCollectionProjectionsIncludePositiveRevision: references root testWorkspaceID |
| `issue_count_failure_test.go` | 1/5 个声明触及数据库；首个：TestListIssuesCountFailureDoesNotReturnPartialSuccess: references root testWorkspaceID |
| `issue_create_labels_test.go` | 9/9 个声明触及数据库；首个：createTestIssueLabel: references root testHandler |
| `issue_create_position_test.go` | 3/3 个声明触及数据库；首个：TestCreateIssuePositionTopOfColumn: references root testWorkspaceID |
| `issue_grouped_test.go` | 1/1 个声明触及数据库；首个：TestListGroupedIssuesAssigneePaginatesPerGroup: references root testPool |
| `issue_involves_test.go` | 19/21 个声明触及数据库；首个：setupInvolvesFixture: references root testPool |
| `issue_last_activity_test.go` | 4/4 个声明触及数据库；首个：TestUpdateIssueActivityExcludesPositionOnlyWrites: references root testPool |
| `issue_limit_validation_test.go` | 2/2 个声明触及数据库；首个：TestListIssues_LimitValidation: references root testHandler |
| `issue_metadata_noop_test.go` | 5/7 个声明触及数据库；首个：readMetadataRowState: references root dbfx |
| `issue_metadata_test.go` | 7/7 个声明触及数据库；首个：TestIssueMetadataSetGetDelete: references root testHandler |
| `issue_reassign_no_cancel_test.go` | 6/6 个声明触及数据库；首个：insertRunningIssueTask: references root testPool |
| `issue_revision_test.go` | 11/22 个声明触及数据库；首个：insertWorkflowTestIssue: references root testWorkspaceID |
| `issue_scheduled_test.go` | 2/2 个声明触及数据库；首个：TestListIssues_ScheduledFilter: references root testHandler |
| `issue_sort_test.go` | 1/1 个声明触及数据库；首个：TestListIssuesSortsByStatusAndUpdatedAt: references root testWorkspaceID |
| `issue_status_position_test.go` | 7/7 个声明触及数据库；首个：destinationMin: references root dbfx |
| `issue_status_reorder_test.go` | 8/8 个声明触及数据库；首个：insertCustomStatus: references root testPool |
| `issue_status_test.go` | 35/35 个声明触及数据库；首个：seedTestCatalog: references root testWorkspaceID |
| `issue_table_filters_test.go` | 3/3 个声明触及数据库；首个：TestListIssues_TableFacetsAreServerSide: references root testPool |
| `issue_table_status_category_test.go` | 11/16 个声明触及数据库；首个：seedStatusCategoryFixture: references root testPool |
| `issue_table_working_agents_facet_test.go` | 4/4 个声明触及数据库；首个：workingAgentsFacetCounts: references root testHandler |
| `issue_trigger_backlog_category_test.go` | 1/1 个声明触及数据库；首个：TestBacklogToCustomBacklogStatusDoesNotTrigger: references root testWorkspaceID |
| `issue_trigger_preview_test.go` | 11/11 个声明触及数据库；首个：seededReadyAgentID: references root testPool |
| `issue_update_project_scope_test.go` | 3/3 个声明触及数据库；首个：TestUpdateIssueProjectStaysInWorkspace: references root testWorkspaceID |
| `issue_validation_test.go` | 6/6 个声明触及数据库；首个：TestCreateIssueInvalidStatusReturns400: references root testHandler |
| `issue_view_preference_test.go` | 2/2 个声明触及数据库；首个：TestIssueViewPreferenceRoundTrip: references root testWorkspaceID |
| `issue_view_test.go` | 13/13 个声明触及数据库；首个：createIssueViewForTest: references root testPool |
| `label_test.go` | 14/14 个声明触及数据库；首个：TestLabelCRUD: references root testHandler |
| `mention_self_trigger_test.go` | 6/7 个声明触及数据库；首个：enqueueMentionedAgentTasksForTest: references root testHandler |
| `mika_onboarding_endpoint_test.go` | 8/9 个声明触及数据库；首个：startMikaOnboarding: references root testHandler |
| `mika_second_member_test.go` | 5/6 个声明触及数据库；首个：addSecondWorkspaceMember: references root testPool |
| `notification_preference_test.go` | 4/4 个声明触及数据库；首个：notificationPreferenceRequest: references root testHandler |
| `onboarding_test.go` | 13/16 个声明触及数据库；首个：newWaitlistTestUser: references root testPool |
| `personal_access_token_test.go` | 11/12 个声明触及数据库；首个：insertTestPAT: references root testHandler |
| `plugin_action_test.go` | 13/13 个声明触及数据库；首个：installPluginForAction: references root testHandler |
| `plugin_callback_test.go` | 10/11 个声明触及数据库；首个：issueCallbackToken: references root testHandler |
| `plugin_example_test.go` | 6/14 个声明触及数据库；首个：installExamplePlugin: references root testWorkspaceID |
| `plugin_hook_test.go` | 14/15 个声明触及数据库；首个：installHookPlugin: references root testHandler |
| `plugin_package_concurrency_test.go` | 3/4 个声明触及数据库；首个：TestDeleteAndInstallRaceLeavesNoDanglingInstallation: references root testPool |
| `plugin_package_test.go` | 10/13 个声明触及数据库；首个：uploadPluginBundle: references root testHandler |
| `plugin_skill_test.go` | 5/8 个声明触及数据库；首个：installSkillPlugin: references root testHandler |
| `plugin_test.go` | 18/23 个声明触及数据库；首个：pluginHandlerRequest: references root testUserID |
| `project_dates_test.go` | 3/4 个声明触及数据库；首个：TestProjectStartDueDateLifecycle: references root testWorkspaceID |
| `project_issue_stats_test.go` | 1/1 个声明触及数据库；首个：TestProjectTerminalIssueStatusKeysFallsBackToCanonicalKeys: references root testHandler |
| `project_validation_test.go` | 8/8 个声明触及数据库；首个：TestCreateProjectInvalidStatusReturns400: references root testWorkspaceID |
| `quick_action_access_test.go` | 3/3 个声明触及数据库；首个：seedPrivateQuickActionOwnedByOther: references root testPool |
| `quick_create_parent_test.go` | 1/1 个声明触及数据库；首个：TestQuickCreateIssueParentTrustBoundary: references root testWorkspaceID |
| `resource_label_cascade_test.go` | 12/12 个声明触及数据库；首个：insertLabelRow: references root testPool |
| `rollup_guard_test.go` | 1/2 个声明触及数据库；首个：lockRollupSingleton: references root testPool |
| `runtime_custom_name_test.go` | 7/7 个声明触及数据库；首个：patchRuntimeCustomName: references root testHandler |
| `runtime_local_skills_overwrite_test.go` | 16/17 个声明触及数据库；首个：createImportTargetSkill: references root testWorkspaceID |
| `runtime_lookup_metrics_test.go` | 3/5 个声明触及数据库；首个：TestAgentRuntimeLookupWSHotPathIsZeroRead: references root testWorkspaceID |
| `runtime_profile_handler_test.go` | 10/10 个声明触及数据库；首个：insertRuntimeProfileFixture: references root testPool |
| `runtime_test.go` | 6/6 个声明触及数据库；首个：TestRuntimeHandlersRejectMalformedRuntimeID: references root testHandler |
| `runtime_unbind_preserves_data_test.go` | 13/13 个声明触及数据库；首个：TestPublishRuntimeTeardown_UsesAutomaticGCActorAndRefreshAction: references root testHandler |
| `runtime_unbind_squad_test.go` | 12/12 个声明触及数据库；首个：seedIsolatedRuntime: references root testPool |
| `runtime_update_authorization_test.go` | 5/5 个声明触及数据库；首个：setRuntimeTestMemberRole: references root testWorkspaceID |
| `runtime_update_error_classification_test.go` | 2/4 个声明触及数据库；首个：TestInitiateUpdate_InfrastructureErrorIsNotAConflict: references root testRuntimeID |
| `search_cancelled_rank_test.go` | 7/8 个声明触及数据库；首个：seedRankIssue: references root testUserID |
| `search_candidate_parity_test.go` | 5/8 个声明触及数据库；首个：TestBuildSearchQuery_CandidateFirstParity: references root dbfx |
| `search_response_test.go` | 2/3 个声明触及数据库；首个：TestSearchIssuesOmitsExactTotal: references root testHandler |
| `seat_capacity_test.go` | 11/13 个声明触及数据库；首个：TestSeatCapacityHandlerBoundsWorkspaceLockWait: references root testHandler |
| `share_link_test.go` | 16/28 个声明触及数据库；首个：clearShareLinksForTestWorkspace: references root testPool |
| `skill_list_test.go` | 7/7 个声明触及数据库；首个：TestListSkills_OmitsContent: references root testWorkspaceID |
| `skill_metadata_test.go` | 9/9 个声明触及数据库；首个：newLargeSkillFixture: references root testWorkspaceID |
| `skill_search_test.go` | 3/3 个声明触及数据库；首个：TestSearchSkillsReturnsNormalizedClawHubCandidates: references root testHandler |
| `squad_assign_trigger_test.go` | 1/1 个声明触及数据库；首个：TestCreateIssueAssignedToSquadEnqueuesLeader: references root testWorkspaceID |
| `squad_briefing_claim_test.go` | 8/9 个声明触及数据库；首个：claimAgentInstructionsForTest: references root testHandler |
| `squad_creator_scope_test.go` | 7/7 个声明触及数据库；首个：squadScopeReq: references root testWorkspaceID |
| `squad_evaluation_provenance_test.go` | 12/14 个声明触及数据库；首个：newSquadEvalFixture: references root dbfx |
| `squad_id_enqueue_test.go` | 2/2 个声明触及数据库；首个：TestCreateComment_SquadMentionStampsSquadIDOnLeaderTask: references root testHandler |
| `squad_member_status_test.go` | 2/2 个声明触及数据库；首个：TestDeriveSquadMemberStatus: references runtimeStatus (daemon_deregister_batch_test.go) |
| `squad_no_action_test.go` | 9/10 个声明触及数据库；首个：newRunningSquadLeaderTaskFixture: references root dbfx |
| `squad_parent_status_contract_test.go` | 2/3 个声明触及数据库；首个：TestSquadAssignedLeaderCanWrapUpOnCommentTurn: references root testHandler |
| `squad_private_leader_test.go` | 7/7 个声明触及数据库；首个：TestCreateIssue_SquadPrivateLeader_PlainMemberBlocked: references root testHandler |
| `squad_worker_comment_wakes_leader_test.go` | 3/3 个声明触及数据库；首个：TestCreateComment_WorkerAgentCommentWakesSquadLeader_MUL4015: references root testPool |
| `subscriber_revoke_race_test.go` | 2/2 个声明触及数据库；首个：TestSubtreeUnsubscribe_LosesToConcurrentRevoke: references root testHandler |
| `subscriber_subtree_endpoint_test.go` | 5/5 个声明触及数据库；首个：TestUnsubscribeEndpoints_SubtreeIsASeparateRoute: references root testHandler |
| `subscriber_test.go` | 1/1 个声明触及数据库；首个：TestSubscriberAPI: references root testHandler |
| `task_cancellation_actor_test.go` | 1/1 个声明触及数据库；首个：TestTaskCancellationActor_SnapshotsAgentName: references root testHandler |
| `task_payload_nul_test.go` | 6/6 个声明触及数据库；首个：seedNULTask: references root testWorkspaceID |
| `task_runs_scope_test.go` | 17/22 个声明触及数据库；首个：newFamilyFixture: references root dbfx |
| `task_terminal_wakeup_test.go` | 2/4 个声明触及数据库；首个：failTaskViaHandler: references root testHandler |
| `task_usage_response_test.go` | 1/1 个声明触及数据库；首个：TestListTasksByIssueHydratesUsage: references root testHandler |
| `timeline_hardcap_test.go` | 15/19 个声明触及数据库；首个：fetchTimelineRecorder: references root testHandler |
| `user_language_test.go` | 6/7 个声明触及数据库；首个：newLanguageTestUser: references root testPool |
| `user_timezone_test.go` | 5/5 个声明触及数据库；首个：newTimezoneTestUser: references root testPool |
| `uuidv7_ids_test.go` | 3/4 个声明触及数据库；首个：TestHotTableInsertsPersistUUIDv7: references root testWorkspaceID |
| `vcs_test.go` | 4/4 个声明触及数据库；首个：vcsHandlerRequest: references root testWorkspaceID |
| `vcs_webhook_test.go` | 15/18 个声明触及数据库；首个：withVCSBox: references root testHandler |
| `webhook_delivery_test.go` | 31/33 个声明触及数据库；首个：setSigningSecretViaHandler: references root testHandler |
| `webhook_status_resolver_test.go` | 1/4 个声明触及数据库；首个：TestWebhookStatusResolver: references root dbfx |
| `workspace_auto_precheck_test.go` | 10/11 个声明触及数据库；首个：autoPrecheckWorkspace: references root testPool |
| `workspace_delete_diagnostics_race_test.go` | 7/16 个声明触及数据库；首个：seedContentDiagnosticsRaceFixture: references root testPool |
| `workspace_delete_diagnostics_test.go` | 1/1 个声明触及数据库；首个：TestDeleteWorkspace_PurgesContentDiagnosticsAtomically: references root testHandler |
| `workspace_delete_lock_test.go` | 2/3 个声明触及数据库；首个：TestDeleteWorkspace_FailsFastWhenRollupLockHeld: references root testUserID |
| `workspace_delete_manifest_test.go` | 1/4 个声明触及数据库；首个：TestWorkspaceDeletionManifestCoversPublicSchema: references root testPool |
| `workspace_mcp_api_test.go` | 15/17 个声明触及数据库；首个：createWorkspaceMcpServerForTest: references root testPool |
| `workspace_mcp_race_test.go` | 2/2 个声明触及数据库；首个：TestWorkspaceMcpServerCreate_CannotLandAfterWorkspaceTeardownCommits: references root testHandler |
| `workspace_timezone_test.go` | 9/12 个声明触及数据库；首个：workspaceWithOwner: references root testUserID |

