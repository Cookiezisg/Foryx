/**
 * Users API — local profile management (no auth, just identity switching).
 * Backend: /api/v1/users (list / create / get / delete / activate).
 */

import { deleteEmpty, getJSON, patchJSON, postJSON } from './client';

export interface User {
  id: string;
  username: string;
  displayName: string;
  avatarColor?: string;
  /** §i18n — preferred UI locale; one of "zh-CN" | "en". */
  language?: 'zh-CN' | 'en';
  lastUsedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export const usersAPI = {
  list: () => getJSON<User[]>('/api/v1/users'),

  get: (id: string) => getJSON<User>(`/api/v1/users/${id}`),

  create: (in_: {
    username: string;
    displayName?: string;
    avatarColor?: string;
    language?: 'zh-CN' | 'en';
  }) => postJSON<User>('/api/v1/users', in_),

  update: (
    id: string,
    patch: { displayName?: string; avatarColor?: string; language?: 'zh-CN' | 'en' },
  ) => patchJSON<User>(`/api/v1/users/${id}`, patch),

  remove: (id: string) => deleteEmpty(`/api/v1/users/${id}`),

  /** Marks user as most-recently-used; client calls this when switching profiles. */
  activate: (id: string) => postJSON<User>(`/api/v1/users/${id}:activate`),
};
