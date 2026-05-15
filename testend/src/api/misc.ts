/**
 * Misc small APIs — attachments only. Subagent + todo intentionally have
 * NO HTTP routes:
 *
 *   - Subagent runs live in the unified `messages` table with
 *     `attrs.kind=subagent_run` + `parentBlockId` pointing at the tool_call
 *     that spawned them. UI reads via `/api/v1/conversations/{id}/messages`.
 *
 *   - Todos are tool-driven (TaskCreate / TaskUpdate / TaskList / TaskGet
 *     system tools). UI observes them via the `notifications` SSE stream
 *     with `type=todo` and refetches via `messages` (todos persist as
 *     blocks attached to the spawning conversation).
 */

import { postJSON } from './client';
import type { Attachment } from '@/types/domain';

/* ───────── attachments ───────── */
export const attachmentAPI = {
  /** multipart upload — backend returns 201 with the persisted Attachment. */
  upload: (file: File, conversationId?: string) => {
    const fd = new FormData();
    fd.append('file', file);
    if (conversationId) fd.append('conversationId', conversationId);
    return postJSON<Attachment>('/api/v1/attachments', fd);
  },
};
