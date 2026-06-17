/* Anselm feature — entities 实体动作单源（… 菜单 = 该 kind 的全部「动作」，对齐后端 api.md 端点；无半成品）。
   rail 行 … 与 sea 头 … 共用 openEntityMenu。每项 onPick → runEntityAction：iterate 切 chat、editGraph 进图编辑器海洋、clearConfig/delete 走确认弹窗、其余 toast 反馈（mock）。
   纪律：菜单只放「做什么」（执行/生命周期/编辑/删除）。「看什么」（版本=tab、运行/调用/激活/firing/stderr 日志=页内段）不进菜单。
        例外：主动「预检」属动作非展示——能力检查(:capability-check)、挂载健康检查(mount-health 重跑) 各触发一次新检查，入菜单合法（页内段只展示上次结果）。
        改名/说明走页头 + 正文【就地编辑】（#1），故菜单无「改 meta」。AI 编辑(:iterate, sparkles) ≠ 重建运行环境(:edit 空 ops, hammer)。
   端点对齐（已核对后端代码）：function/handler/workflow :edit=ops；agent :edit=全量 Config 替换；handler DELETE /config（清空+停实例）；mcp /tools/{tool}:invoke（试调，裸结果）。 */
window.ENTITY_ACTIONS = {
  function: [
    { label: "运行", value: "run", icon: "play" },
    { label: "编辑代码", value: "editCode", icon: "edit" },
    { label: "重建运行环境", value: "rebuildEnv", icon: "forge" },
    { label: "AI 编辑", value: "iterate", icon: "iterate" },
    { label: "回滚到版本…", value: "revert", icon: "history" },
    { label: "删除", value: "delete", icon: "trash", danger: true },
  ],
  handler: [
    { label: "调用方法", value: "call", icon: "handler" },
    { label: "重启实例", value: "restart", icon: "spin" },
    { label: "配置（init args）", value: "config", icon: "gear" },
    { label: "编辑代码", value: "editCode", icon: "edit" },
    { label: "AI 编辑", value: "iterate", icon: "iterate" },
    { label: "回滚到版本…", value: "revert", icon: "history" },
    { label: "清空配置", value: "clearConfig", icon: "trash", danger: true },
    { label: "删除", value: "delete", icon: "trash", danger: true },
  ],
  agent: [
    { label: "调用", value: "invoke", icon: "agent" },
    { label: "挂载健康检查", value: "mountHealth", icon: "shield" },
    { label: "编辑配置", value: "editConfig", icon: "edit" },
    { label: "AI 编辑", value: "iterate", icon: "iterate" },
    { label: "回滚到版本…", value: "revert", icon: "history" },
    { label: "删除", value: "delete", icon: "trash", danger: true },
  ],
  workflow: [
    { label: "立即触发", value: "trigger", icon: "trigger" },
    { type: "label", label: "生命周期" },
    { label: "上线（监听）", value: "activate", icon: "play" },
    { label: "下线", value: "deactivate", icon: "stop" },
    { label: "暂存一次（stage）", value: "stage", icon: "scheduler" },
    { label: "强制终止全部在途", value: "kill", icon: "stop", danger: true },
    { type: "label", label: "图 / 编辑" },
    { label: "进入图编辑器", value: "editGraph", icon: "workflow" },
    { label: "能力检查", value: "capability", icon: "shield" },
    { label: "编辑并发策略", value: "concurrency", icon: "gear" },
    { label: "AI 编辑", value: "iterate", icon: "iterate" },
    { label: "回滚到版本…", value: "revert", icon: "history" },
    { label: "删除", value: "delete", icon: "trash", danger: true },
  ],
  trigger: [
    { label: "手动触发（fire）", value: "fire", icon: "zap" },
    { label: "编辑配置", value: "editConfig", icon: "edit" },
    { label: "AI 编辑", value: "iterate", icon: "iterate" },
    { label: "删除", value: "delete", icon: "trash", danger: true },
  ],
  control: [
    { label: "编辑分支（when→port）", value: "editBranch", icon: "control" },
    { label: "AI 编辑", value: "iterate", icon: "iterate" },
    { label: "回滚到版本…", value: "revert", icon: "history" },
    { label: "删除", value: "delete", icon: "trash", danger: true },
  ],
  approval: [
    { label: "编辑模板 / 决策规则", value: "editTemplate", icon: "approval" },
    { label: "AI 编辑", value: "iterate", icon: "iterate" },
    { label: "回滚到版本…", value: "revert", icon: "history" },
    { label: "删除", value: "delete", icon: "trash", danger: true },
  ],
  mcp: [
    { label: "重连（reconnect）", value: "reconnect", icon: "plug" },
    { label: "试调工具", value: "toolInvoke", icon: "play" },
    { label: "编辑配置", value: "editConfig", icon: "edit" },
    { label: "删除", value: "delete", icon: "trash", danger: true },
  ],
  skill: [
    { label: "激活（inline / fork）", value: "activate", icon: "skill" },
    { label: "编辑文件（SKILL.md）", value: "editFile", icon: "edit" },
    { label: "删除", value: "delete", icon: "trash", danger: true },
  ],
};

// 打开某实体的动作菜单（rail 行 … / sea 头 … 共用）。anchor = 触发的 … 按钮。
window.openEntityMenu = function (anchor, entity, ctx) {
  const items = window.ENTITY_ACTIONS[entity.kind] || [];
  window.AnMenu.open(anchor, {
    items, align: "end", placement: "bottom", namespace: "entity-actions",
    onPick: (v, it) => window.runEntityAction(v, entity, ctx, it),
  });
};

// 危险确认弹窗（清空配置 / 删除 等不可轻率动作）。
function confirmDanger(title, content, okLabel, onOk) {
  window.AnDialog && window.AnDialog.open({
    title, content,
    actions: [{ label: "取消", variant: "ghost" }, { label: okLabel, variant: "danger", onClick: onOk }],
  });
}

// 执行动作：iterate→切 chat、editGraph→进图编辑器海洋、clearConfig/delete→确认弹窗、其余→toast 反馈（mock，真后端时换 dio 调端点）。
window.runEntityAction = function (value, entity, ctx, item) {
  const I = ctx && ctx.Intent;
  const toast = (t, tone) => window.AnToast && window.AnToast.show({ text: t, tone });
  if (value === "iterate") { I && I.select({ kind: "conversation", id: "iterate:" + entity.id }); return; }
  if (value === "editGraph") { I && I.act && I.act({ verb: "editGraph", kind: "workflow", id: entity.id }); return; }
  if (value === "clearConfig") {
    confirmDanger("清空 " + entity.label + " 的配置？", "将清空全部 init 配置并停止常驻实例（DELETE /config）。", "清空配置",
      () => toast("已清空配置并停止实例 · " + entity.label + "（mock）", "danger"));
    return;
  }
  if (value === "delete") {
    confirmDanger("删除 " + entity.label + "？", "该实体及其全部版本将被软删除，可在保留期内恢复。", "删除",
      () => toast("已删除 " + entity.label + "（mock）", "danger"));
    return;
  }
  toast((item ? item.label : value) + " · " + entity.label + "（mock）");
};
