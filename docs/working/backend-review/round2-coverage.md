---
id: WRK-006
type: working
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
expires: 2026-09-11
landed-into: ""
audience: [human, ai]
---

# round2-coverage —— 全仓一行不落覆盖台账

> 用户要求：整个代码仓库一行不落全审（含测试）。本文件是物理覆盖清单——613 个 Go 文件 + 配置文件，审完一个勾一个。`[x]` = 已逐行亲读。


## backend/cmd/docs (199 行)

- [ ] main.go (199)

## backend/cmd/server (55 行)

- [ ] main.go (55)

## backend/internal/app/agent (1556 行)

- [x] agent.go (168)
- [x] agent_mirror_test.go (46)
- [x] agent_stream_test.go (111)
- [x] agent_test.go (148)
- [x] catalog_source.go (42)
- [x] crud.go (306)
- [x] executions.go (49)
- [x] humanloop_test.go (143)
- [x] invoke.go (385)
- [x] mention_resolver.go (35)
- [x] relations.go (123)

## backend/internal/app/aispawn (294 行)

- [x] aispawn.go (156)
- [x] aispawn_test.go (138)

## backend/internal/app/apikey (873 行)

- [x] apikey.go (321)
- [x] apikey_test.go (240)
- [x] providers.go (89)
- [x] tester.go (223)

## backend/internal/app/approval (735 行)

- [x] approval.go (76)
- [x] approval_test.go (200)
- [x] catalog_source.go (38)
- [x] crud.go (280)
- [x] mention_resolver.go (33)
- [x] relations.go (108)

## backend/internal/app/attachment (869 行)

- [x] attachment.go (299)
- [x] attachment_test.go (380)
- [x] extractor.go (190)

## backend/internal/app/catalog (312 行)

- [x] catalog.go (102)
- [x] catalog_test.go (122)
- [x] mechanical.go (88)

## backend/internal/app/chat (2546 行)

- [x] ask_test.go (108)
- [x] autotitle.go (143)
- [x] chat.go (564)
- [x] chat_test.go (553)
- [x] danger_test.go (209)
- [x] emit.go (126)
- [x] history.go (192)
- [x] host.go (138)
- [x] interactions.go (68)
- [x] mention.go (131)
- [x] prompt.go (118)
- [x] runner.go (196)

## backend/internal/app/contextmgr (764 行)

- [x] contextmgr.go (210)
- [x] contextmgr_test.go (255)
- [x] pipeline.go (215)
- [x] prompt.go (84)

## backend/internal/app/control (754 行)

- [x] catalog_source.go (38)
- [x] control.go (78)
- [x] control_test.go (208)
- [x] crud.go (284)
- [x] mention_resolver.go (35)
- [x] relations.go (111)

## backend/internal/app/conversation (565 行)

- [x] conversation.go (284)
- [x] conversation_test.go (210)
- [x] relations.go (71)

## backend/internal/app/document (807 行)

- [x] catalog_source.go (53)
- [x] document.go (433)
- [x] document_test.go (191)
- [x] mention_resolver.go (41)
- [x] relations.go (89)

## backend/internal/app/entitystream (278 行)

- [ ] entitystream.go (175)
- [ ] entitystream_test.go (103)

## backend/internal/app/envfix (536 行)

- [x] envfix.go (215)
- [x] envfix_test.go (202)
- [x] fix.go (119)

## backend/internal/app/function (1635 行)

- [x] apply.go (169)
- [x] catalog_source.go (42)
- [x] crud.go (388)
- [x] executions.go (51)
- [x] function.go (170)
- [x] function_test.go (241)
- [x] mention_resolver.go (36)
- [x] relations.go (111)
- [x] run.go (151)
- [x] sandbox_adapter.go (200)
- [x] validate.go (76)

## backend/internal/app/handler (2569 行)

- [x] apply.go (273)
- [x] assemble.go (198)
- [x] call.go (217)
- [x] calls.go (40)
- [x] catalog_source.go (68)
- [x] catalog_source_test.go (25)
- [x] config.go (140)
- [x] crud.go (411)
- [x] handler.go (232)
- [x] handler_test.go (323)
- [x] manager.go (165)
- [x] mention_resolver.go (37)
- [x] relations.go (99)
- [x] sandbox_adapter.go (115)
- [x] spawn.go (140)
- [x] validate.go (70)
- [x] yield_test.go (16)

## backend/internal/app/humanloop (313 行)

- [ ] humanloop.go (210)
- [ ] humanloop_test.go (103)

## backend/internal/app/loop (2121 行)

- [x] emit.go (131)
- [x] forge_entities_test.go (77)
- [x] gate_test.go (60)
- [x] history.go (115)
- [x] loop.go (314)
- [x] loop_test.go (447)
- [x] progress.go (164)
- [x] progress_persist_test.go (86)
- [x] progress_test.go (93)
- [x] stream.go (362)
- [x] tools.go (272)

## backend/internal/app/mcp (1217 行)

- [x] calltool.go (175)
- [x] catalog_source.go (61)
- [x] install.go (281)
- [x] mcp.go (390)
- [x] mcp_test.go (269)
- [x] relations.go (41)

## backend/internal/app/memory (329 行)

- [x] memory.go (199)
- [x] memory_test.go (130)

## backend/internal/app/model (171 行)

- [x] capability.go (98)
- [x] capability_test.go (73)

## backend/internal/app/notification (246 行)

- [ ] notification.go (105)
- [ ] notification_test.go (141)

## backend/internal/app/relation (759 行)

- [ ] relation.go (401)
- [ ] relation_test.go (358)

## backend/internal/app/sandbox (1251 行)

- [x] disk.go (59)
- [x] owner_id_validation_test.go (47)
- [x] path_test.go (38)
- [x] restore.go (73)
- [x] restore_test.go (49)
- [x] sandbox.go (516)
- [x] spawn.go (272)
- [x] spawn_test.go (197)

## backend/internal/app/scheduler (2208 行)

- [x] advance.go (157)
- [x] dispatch.go (205)
- [x] kill.go (127)
- [x] kill_test.go (145)
- [x] query.go (39)
- [x] run.go (374)
- [x] scheduler.go (202)
- [x] scheduler_test.go (664)
- [x] walk.go (295)

## backend/internal/app/skill (690 行)

- [x] activate.go (112)
- [x] catalog_source.go (49)
- [x] mutate.go (137)
- [x] relations.go (104)
- [x] skill.go (83)
- [x] skill_test.go (205)

## backend/internal/app/subagent (679 行)

- [x] emit.go (87)
- [x] host.go (76)
- [x] registry.go (140)
- [x] subagent.go (193)
- [x] subagent_test.go (183)

## backend/internal/app/todo (484 行)

- [x] render.go (100)
- [x] todo.go (191)
- [x] todo_test.go (193)

## backend/internal/app/tool/agent (604 行)

- [x] agent.go (31)
- [x] agent_test.go (50)
- [x] executions.go (110)
- [x] forge.go (150)
- [x] forge_spec.go (11)
- [x] lifecycle.go (141)
- [x] query.go (93)
- [x] sentinels.go (18)

## backend/internal/app/tool/approval (539 行)

- [x] approval.go (28)
- [x] approval_test.go (137)
- [x] forge_spec.go (14)
- [x] lifecycle.go (244)
- [x] query.go (99)
- [x] sentinels.go (17)

## backend/internal/app/tool/ask (115 行)

- [x] ask.go (115)

## backend/internal/app/tool/control (567 行)

- [x] control.go (47)
- [x] control_test.go (138)
- [x] forge_spec.go (12)
- [x] lifecycle.go (254)
- [x] query.go (99)
- [x] sentinels.go (17)

## backend/internal/app/tool/document (853 行)

- [x] create.go (94)
- [x] delete.go (64)
- [x] document.go (53)
- [x] document_test.go (222)
- [x] edit.go (85)
- [x] forge_spec.go (14)
- [x] list.go (79)
- [x] move.go (92)
- [x] read.go (71)
- [x] search.go (79)

## backend/internal/app/tool (562 行)

- [x] fields.go (156)
- [x] tool.go (126)
- [x] tool_test.go (198)
- [x] toolset.go (82)

## backend/internal/app/tool/filesystem (1371 行)

- [x] edit.go (215)
- [x] edit_test.go (287)
- [x] filesystem.go (34)
- [x] read.go (219)
- [x] read_test.go (192)
- [x] write.go (183)
- [x] write_test.go (241)

## backend/internal/app/tool/function (795 行)

- [x] forge.go (162)
- [x] forge_spec.go (15)
- [x] forge_stream_test.go (59)
- [x] function.go (76)
- [x] function_test.go (63)
- [x] lifecycle.go (107)
- [x] query.go (99)
- [x] run.go (197)
- [x] sentinels.go (17)

## backend/internal/app/tool/handler (810 行)

- [x] call.go (188)
- [x] forge.go (160)
- [x] forge_spec.go (12)
- [x] handler.go (70)
- [x] handler_test.go (64)
- [x] manage.go (201)
- [x] query.go (97)
- [x] sentinels.go (18)

## backend/internal/app/tool/mcp (355 行)

- [x] dynamic.go (54)
- [x] mcp.go (60)
- [x] mcp_test.go (40)
- [x] sentinels.go (12)
- [x] system.go (189)

## backend/internal/app/tool/memory (461 行)

- [x] forget.go (60)
- [x] memory.go (49)
- [x] memory_test.go (188)
- [x] read.go (68)
- [x] write.go (96)

## backend/internal/app/tool/mount (544 行)

- [x] mount.go (319)
- [x] mount_test.go (225)

## backend/internal/app/tool/search (1781 行)

- [x] glob.go (195)
- [x] glob_test.go (157)
- [x] grep.go (222)
- [x] grep_rg.go (114)
- [x] grep_stdlib.go (458)
- [x] grep_test.go (239)
- [x] ls.go (211)
- [x] ls_test.go (138)
- [x] search.go (47)

## backend/internal/app/tool/shell (1065 行)

- [x] bash.go (278)
- [x] bash_stream_test.go (75)
- [x] danger.go (49)
- [x] kill.go (76)
- [x] manager.go (171)
- [x] output.go (133)
- [x] shell.go (64)
- [x] shell_test.go (219)

## backend/internal/app/tool/skill (348 行)

- [x] activate.go (111)
- [x] crud.go (188)
- [x] forge_spec.go (10)
- [x] sentinels.go (12)
- [x] skill.go (27)

## backend/internal/app/tool/subagent (191 行)

- [x] subagent.go (120)
- [x] subagent_test.go (71)

## backend/internal/app/tool/toolset (298 行)

- [x] search.go (178)
- [x] search_test.go (120)

## backend/internal/app/tool/trigger (490 行)

- [x] activations.go (111)
- [x] forge.go (180)
- [x] manage.go (54)
- [x] query.go (100)
- [x] sentinels.go (16)
- [x] trigger.go (29)

## backend/internal/app/tool/web (1270 行)

- [x] fetch.go (359)
- [x] fetch_stream_test.go (78)
- [x] fetch_test.go (142)
- [ ] search.go (254)
- [x] search_byok.go (196)
- [ ] search_test.go (175)
- [x] web.go (66)

## backend/internal/app/tool/workflow (905 行)

- [x] capability.go (61)
- [x] exec.go (230)
- [x] forge.go (232)
- [x] forge_spec.go (15)
- [x] query.go (101)
- [x] sentinels.go (17)
- [x] workflow.go (54)
- [x] workflow_test.go (195)

## backend/internal/app/trigger (1107 行)

- [x] catalog_source.go (36)
- [x] crud.go (211)
- [x] fire_entities_test.go (53)
- [x] lifecycle.go (116)
- [x] mention_resolver.go (33)
- [x] relations.go (104)
- [x] report.go (157)
- [x] stage_test.go (65)
- [x] trigger.go (128)
- [x] trigger_test.go (204)

## backend/internal/app/workflow (2060 行)

- [x] capability.go (286)
- [x] catalog_source.go (40)
- [x] crud.go (514)
- [x] execution.go (223)
- [x] execution_test.go (154)
- [x] mention_resolver.go (35)
- [x] relations.go (128)
- [x] workflow.go (185)
- [x] workflow_test.go (495)

## backend/internal/app/workspace (593 行)

- [x] workspace.go (325)
- [x] workspace_test.go (268)

## backend/internal/bootstrap (2615 行)

- [ ] aispawn.go (143)
- [ ] background_ctx_test.go (86)
- [ ] build.go (326)
- [ ] build_data.go (189)
- [ ] build_services.go (376)
- [ ] build_test.go (79)
- [ ] conversation.go (47)
- [ ] dispatch.go (169)
- [ ] dispatch_test.go (152)
- [ ] model_info.go (77)
- [ ] refresolver.go (220)
- [ ] refresolver_test.go (152)
- [ ] renderers.go (100)
- [ ] renderers_test.go (63)
- [ ] resolvers.go (177)
- [ ] resolvers_test.go (158)
- [ ] sensor.go (72)
- [ ] workflow_exec.go (29)

## backend/internal/domain/agent (251 行)

- [ ] agent.go (142)
- [ ] execution.go (109)

## backend/internal/domain/apikey (151 行)

- [ ] apikey.go (151)

## backend/internal/domain/approval (300 行)

- [ ] approval.go (176)
- [ ] approval_test.go (65)
- [ ] repository.go (59)

## backend/internal/domain/attachment (144 行)

- [ ] attachment.go (144)

## backend/internal/domain/catalog (90 行)

- [ ] catalog.go (32)
- [ ] source.go (58)

## backend/internal/domain/control (231 行)

- [ ] control.go (142)
- [ ] control_test.go (29)
- [ ] repository.go (60)

## backend/internal/domain/conversation (100 行)

- [ ] conversation.go (100)

## backend/internal/domain/crypto (25 行)

- [ ] encryptor.go (25)

## backend/internal/domain/document (124 行)

- [ ] document.go (124)

## backend/internal/domain/flowrun (274 行)

- [x] flowrun.go (170)
- [x] repository.go (104)

## backend/internal/domain/function (343 行)

- [ ] execution.go (113)
- [ ] function.go (136)
- [ ] function_test.go (17)
- [ ] repository.go (77)

## backend/internal/domain/handler (346 行)

- [ ] call_log.go (98)
- [ ] handler.go (126)
- [ ] handler_test.go (16)
- [ ] method.go (45)
- [ ] repository.go (61)

## backend/internal/domain/mcp (562 行)

- [ ] call_log.go (94)
- [ ] mcp.go (169)
- [ ] registry.go (217)
- [ ] registry_test.go (82)

## backend/internal/domain/memory (102 行)

- [ ] memory.go (102)

## backend/internal/domain/mention (101 行)

- [ ] mention.go (82)
- [ ] mention_test.go (19)

## backend/internal/domain/messages (366 行)

- [ ] messages.go (321)
- [ ] messages_test.go (45)

## backend/internal/domain/model (204 行)

- [ ] model.go (115)
- [ ] model_test.go (89)

## backend/internal/domain/notification (69 行)

- [ ] notification.go (69)

## backend/internal/domain/relation (380 行)

- [ ] entitykind.go (101)
- [ ] entitykind_test.go (68)
- [ ] relation.go (211)

## backend/internal/domain/sandbox (310 行)

- [ ] installer.go (65)
- [ ] sandbox.go (225)
- [ ] tooling.go (20)

## backend/internal/domain/skill (125 行)

- [ ] skill.go (125)

## backend/internal/domain/stream (413 行)

- [x] bridge.go (41)
- [x] event.go (66)
- [x] frame.go (88)
- [x] frame_test.go (22)
- [x] scope.go (62)
- [x] scope_test.go (27)
- [x] validate.go (59)
- [x] validate_test.go (48)

## backend/internal/domain/todo (105 行)

- [ ] todo.go (105)

## backend/internal/domain/trigger (360 行)

- [x] activation.go (39)
- [x] config.go (119)
- [x] config_test.go (37)
- [x] firing.go (38)
- [x] repository.go (34)
- [x] trigger.go (93)

## backend/internal/domain/websearch (104 行)

- [ ] websearch.go (73)
- [ ] websearch_test.go (31)

## backend/internal/domain/workflow (1573 行)

- [x] graph.go (356)
- [x] graph_test.go (271)
- [x] ops.go (374)
- [x] ops_test.go (182)
- [x] repository.go (100)
- [x] workflow.go (290)

## backend/internal/domain/workspace (127 行)

- [ ] workspace.go (127)

## backend/internal/infra/crypto (379 行)

- [x] aesgcm.go (91)
- [x] aesgcm_test.go (162)
- [x] fingerprint.go (78)
- [x] fingerprint_test.go (48)

## backend/internal/infra/db (192 行)

- [x] db.go (91)
- [x] db_test.go (70)
- [x] migrate.go (31)

## backend/internal/infra/fs/blob (315 行)

- [x] blob.go (181)
- [x] blob_test.go (134)

## backend/internal/infra/fs/memory (331 行)

- [x] frontmatter.go (58)
- [x] memory.go (172)
- [x] memory_test.go (101)

## backend/internal/infra/fs/skill (400 行)

- [x] frontmatter.go (46)
- [x] skill.go (216)
- [x] skill_test.go (138)

## backend/internal/infra/handler (243 行)

- [x] client.go (243)

## backend/internal/infra/llm (8624 行)

- [x] anthropic.go (542)
- [x] anthropic_test.go (193)
- [x] custom.go (478)
- [x] custom_test.go (148)
- [x] deepseek.go (473)
- [x] deepseek_test.go (135)
- [x] doubao.go (469)
- [x] doubao_test.go (150)
- [x] factory.go (66)
- [x] factory_test.go (106)
- [x] gemini.go (559)
- [x] gemini_test.go (180)
- [x] llm.go (281)
- [x] mock.go (135)
- [x] mock_test.go (41)
- [x] models_common.go (108)
- [x] moonshot.go (405)
- [x] moonshot_test.go (152)
- [x] multimodal_test.go (132)
- [x] ollama.go (548)
- [x] ollama_test.go (245)
- [x] openai.go (509)
- [x] openai_test.go (148)
- [x] openrouter.go (450)
- [x] openrouter_test.go (190)
- [x] provider.go (173)
- [x] qwen.go (432)
- [x] qwen_test.go (203)
- [x] retry_test.go (83)
- [x] sanitizer.go (55)
- [x] sanitizer_test.go (36)
- [x] transport.go (140)
- [x] transport_test.go (63)
- [x] zhipu.go (425)
- [x] zhipu_test.go (171)

## backend/internal/infra/logger (59 行)

- [ ] zap.go (32)
- [ ] zap_test.go (27)

## backend/internal/infra/mcp (866 行)

- [x] client.go (320)
- [x] config.go (38)
- [x] e2e_test.go (101)
- [x] progress.go (68)
- [x] progress_test.go (51)
- [x] registry.go (241)
- [x] registry_test.go (47)

## backend/internal/infra/sandbox (2331 行)

- [x] direct.go (647)
- [x] direct_test.go (197)
- [x] docker.go (149)
- [x] docker_test.go (66)
- [x] dotnet.go (58)
- [x] exec_helper.go (53)
- [x] install_e2e_test.go (67)
- [x] node.go (130)
- [x] node_test.go (51)
- [x] proc_darwin.go (25)
- [x] proc_linux.go (28)
- [x] proc_windows.go (79)
- [x] python.go (135)
- [x] python_test.go (59)
- [x] resolveexec_test.go (79)
- [x] spawn.go (204)
- [x] spawn_stream_test.go (45)
- [x] spawn_test.go (259)

## backend/internal/infra/store/agent (541 行)

- [x] agent.go (354)
- [x] agent_test.go (104)
- [x] executions.go (83)

## backend/internal/infra/store/apikey (315 行)

- [x] apikey.go (154)
- [x] apikey_test.go (161)

## backend/internal/infra/store/approval (507 行)

- [x] approval.go (298)
- [x] approval_test.go (209)

## backend/internal/infra/store/attachment (262 行)

- [x] attachment.go (123)
- [x] attachment_test.go (139)

## backend/internal/infra/store/control (507 行)

- [x] control.go (295)
- [x] control_test.go (212)

## backend/internal/infra/store/conversation (412 行)

- [x] conversation.go (143)
- [x] conversation_test.go (269)

## backend/internal/infra/store/document (450 行)

- [x] document.go (278)
- [x] document_test.go (172)

## backend/internal/infra/store/flowrun (595 行)

- [x] flowrun.go (336)
- [x] flowrun_test.go (259)

## backend/internal/infra/store/function (699 行)

- [x] executions.go (85)
- [x] function.go (373)
- [x] function_test.go (241)

## backend/internal/infra/store/handler (612 行)

- [x] calls.go (78)
- [x] handler.go (393)
- [x] handler_test.go (141)

## backend/internal/infra/store/mcp (468 行)

- [x] calls.go (55)
- [x] calls_test.go (53)
- [x] mcp.go (230)
- [x] mcp_test.go (130)

## backend/internal/infra/store/messages (677 行)

- [x] messages.go (337)
- [x] messages_test.go (340)

## backend/internal/infra/store/notification (102 行)

- [x] notification.go (102)

## backend/internal/infra/store/relation (198 行)

- [x] relation.go (198)

## backend/internal/infra/store/sandbox (471 行)

- [x] sandbox.go (250)
- [x] sandbox_test.go (221)

## backend/internal/infra/store/todo (217 行)

- [x] todo.go (97)
- [x] todo_test.go (120)

## backend/internal/infra/store/trigger (495 行)

- [x] activations.go (53)
- [x] firings.go (110)
- [x] trigger.go (181)
- [x] trigger_test.go (151)

## backend/internal/infra/store/workflow (624 行)

- [x] workflow.go (334)
- [x] workflow_test.go (290)

## backend/internal/infra/store/workspace (281 行)

- [x] workspace.go (124)
- [x] workspace_test.go (157)

## backend/internal/infra/stream (362 行)

- [x] bus.go (122)
- [x] bus_test.go (88)
- [x] subscribe.go (81)
- [x] subscribe_test.go (71)

## backend/internal/infra/trigger/cron (118 行)

- [x] cron.go (118)

## backend/internal/infra/trigger/fsnotify (235 行)

- [x] fsnotify.go (235)

## backend/internal/infra/trigger (52 行)

- [x] listener.go (52)

## backend/internal/infra/trigger/sensor (259 行)

- [x] sensor.go (186)
- [x] sensor_test.go (73)

## backend/internal/infra/trigger/webhook (256 行)

- [x] webhook.go (256)

## backend/internal/pkg/agentstate (283 行)

- [ ] activeskill_test.go (33)
- [ ] agentstate.go (150)
- [ ] agentstate_test.go (100)

## backend/internal/pkg/cel (363 行)

- [ ] cel.go (168)
- [ ] scoped_test.go (35)
- [ ] template.go (101)
- [ ] template_test.go (59)

## backend/internal/pkg/errors (373 行)

- [ ] error.go (73)
- [ ] error_test.go (75)
- [ ] kind.go (37)
- [ ] sentinel.go (17)
- [ ] standard_test.go (171)

## backend/internal/pkg/fspath (154 行)

- [x] fspath.go (75)
- [x] fspath_test.go (79)

## backend/internal/pkg/idgen (51 行)

- [ ] idgen.go (25)
- [ ] idgen_test.go (26)

## backend/internal/pkg/jsonrepair (262 行)

- [ ] jsonrepair.go (158)
- [ ] jsonrepair_test.go (104)

## backend/internal/pkg/limits (216 行)

- [ ] limits.go (151)
- [ ] limits_test.go (65)

## backend/internal/pkg/orm (1518 行)

- [x] compile.go (76)
- [x] crud_test.go (139)
- [x] db.go (83)
- [x] errors.go (44)
- [x] exec_test.go (31)
- [x] helper_test.go (94)
- [x] meta.go (130)
- [x] meta_test.go (60)
- [x] mutation.go (189)
- [x] mutation_test.go (68)
- [x] page_test.go (47)
- [x] query.go (92)
- [x] query_test.go (110)
- [x] repo.go (54)
- [x] scan.go (77)
- [x] select.go (188)
- [x] tx_test.go (36)

## backend/internal/pkg/pagination (106 行)

- [x] cursor.go (61)
- [x] cursor_test.go (45)

## backend/internal/pkg/pathguard (547 行)

- [x] pathguard.go (204)
- [x] pathguard_test.go (343)

## backend/internal/pkg/reqctx (455 行)

- [ ] agentstate.go (38)
- [ ] agentstate_test.go (38)
- [ ] conversation.go (116)
- [ ] conversation_test.go (45)
- [ ] flowrun.go (49)
- [ ] reqctx.go (57)
- [ ] reqctx_test.go (18)
- [ ] workspace.go (63)
- [ ] workspace_test.go (31)

## backend/internal/pkg/schema (193 行)

- [ ] schema.go (137)
- [ ] schema_test.go (56)

## backend/internal/pkg/tokencount (146 行)

- [ ] tokencount.go (73)
- [ ] tokencount_test.go (73)

## backend/internal/pkg/wikilink (143 行)

- [ ] wikilink.go (47)
- [ ] wikilink_test.go (96)

## backend/internal/transport/httpapi/handlers (4754 行)

- [x] agent.go (297)
- [x] aispawn.go (83)
- [x] apikey.go (164)
- [x] approval.go (231)
- [x] attachment.go (131)
- [x] catalog.go (49)
- [x] chat.go (169)
- [x] control.go (239)
- [x] conversation.go (171)
- [x] decode.go (43)
- [x] document.go (202)
- [x] flowrun.go (171)
- [x] function.go (306)
- [x] handler.go (342)
- [x] mcp.go (264)
- [x] memory.go (139)
- [x] model.go (74)
- [x] notification.go (95)
- [x] registrar.go (21)
- [x] relation.go (104)
- [x] sandbox.go (299)
- [x] skill.go (177)
- [x] stream.go (75)
- [x] stream_test.go (113)
- [x] todo.go (65)
- [x] trigger.go (172)
- [x] util.go (16)
- [x] workflow.go (324)
- [x] workspaces.go (218)

## backend/internal/transport/httpapi/middleware (384 行)

- [x] auth.go (83)
- [x] auth_test.go (82)
- [x] cors.go (74)
- [x] locale.go (27)
- [x] logger.go (69)
- [x] notfound.go (14)
- [x] recover.go (35)

## backend/internal/transport/httpapi/response (643 行)

- [x] envelope.go (76)
- [x] envelope_test.go (50)
- [x] errmap.go (81)
- [x] errmap_test.go (101)
- [x] page.go (61)
- [x] page_test.go (39)
- [x] sse.go (61)
- [x] stream.go (79)
- [x] stream_test.go (95)

## backend/internal/transport/httpapi/router (188 行)

- [x] chain.go (57)
- [x] chain_test.go (38)
- [x] recorder.go (93)

## 配置 / 数据文件

- [ ] .editorconfig (45)
- [ ] .env.example (11)
- [ ] .gitattributes (56)
- [ ] .gitignore (247)
- [ ] LICENSE (201)
- [ ] Makefile (121)
- [ ] backend/go.mod (38)
- [ ] backend/go.sum (82)
- [x] backend/internal/infra/mcp/registry_snapshot.json (1)
- [ ] devbox.json (15)
- [ ] devbox.lock (165)

**总计：87628 行 Go + 配置**
