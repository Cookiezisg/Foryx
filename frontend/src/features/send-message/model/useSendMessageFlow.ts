// Orchestrates send / cancel / self-heal for a single conversation.
// Extracted verbatim from ChatPane.onSend / onCancel so the component
// only handles rendering.
//
// 封装发送/取消/自愈编排;ChatPane 只负责渲染,不再含业务决策。

import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useSendMessage, useCancelStream } from "@entities/conversation";
import { qk } from "@shared/api";
// TODO(阶段4): ui store 拆进 app/model 后,将此 import 替换为正式 FSD 路径。
import { useUIStore } from "../../../store/ui.js";

import type { SendMessageBody } from "@entities/conversation";

// Superset of SendMessageBody — attachments/mentions are assembled here before
// casting to SendMessageBody so the wire shape stays identical to the original
// ChatPane.onSend logic.
//
// 发送时按原 ChatPane.onSend 逻辑组装 body，字段名与后端 API 完全一致。
interface SendPayload {
  content: string;
  attachments?: Array<{ name: string; size: number }>;
  mentions?: Array<{ type: string; id: string }>;
}

export function useSendMessageFlow(convId: string | null) {
  const { t } = useTranslation("conv");
  const qc = useQueryClient();
  const setActiveConv = useUIStore((s) => s.setActiveConv);
  const pushToast = useUIStore((s) => s.pushToast);

  const send = useSendMessage(convId as string);
  const cancel = useCancelStream(convId as string);

  const submit = ({ content, attachments, mentions }: SendPayload) => {
    const body = { content } as unknown as SendMessageBody & Record<string, unknown>;
    if (attachments?.length) body.attachments = attachments.map((a) => ({ fileName: a.name, sizeBytes: a.size }));
    if (mentions?.length) body.mentions = mentions.map((m) => ({ type: m.type, id: m.id }));
    send.mutate(body as SendMessageBody, {
      onError: (err: Error & { code?: string }) => {
        // Stale conv: backend says this conversation doesn't exist (deleted,
        // or belongs to a different user after account switch). Self-heal:
        // clear activeConv + refetch conversation list so the user sees the
        // real state instead of a stuck "send fails" loop.
        //
        // activeConv 已失效(被删 / 切户后跨用户残留)。自愈:清掉 +
        // 重拉对话列表。
        if (err?.code === "CONVERSATION_NOT_FOUND") {
          setActiveConv(null);
          qc.invalidateQueries({ queryKey: qk.conversations() });
          pushToast({ kind: "warn", title: t("toast.convGoneTitle"), desc: t("toast.convGoneDesc") });
          return;
        }
        pushToast({ kind: "error", title: t("toast.sendFailTitle"), desc: err.message });
      },
    });
  };

  const cancelStream = () => {
    cancel.mutate(undefined, {
      onError: (err: Error) => pushToast({ kind: "warn", title: t("toast.cancelFailTitle"), desc: err.message }),
    });
  };

  return {
    submit,
    cancelStream,
    isPending: send.isPending,
  };
}
