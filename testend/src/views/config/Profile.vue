<script setup lang="ts">
/**
 * Profile management — list profiles, create / delete, see which is active.
 *
 * Per-profile password / lock is V1.5 territory.
 */
import { ref, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useUsersStore } from '@/stores/users';
import { useUIStore } from '@/stores/ui';
import type { SupportedLocale } from '@/i18n';

const { t } = useI18n();
const users = useUsersStore();
const ui = useUIStore();

const showCreate = ref(false);
const newUsername = ref('');
const newDisplay = ref('');
const newColor = ref('#4f46e5');
const err = ref('');

async function setLang(userId: string, lang: SupportedLocale) {
  try {
    await users.setLanguage(userId, lang);
    ui.toast('ok', t('users.toastLanguageChanged'));
  } catch {
    /* setLanguage already shows toast on error */
  }
}

onMounted(() => {
  users.refresh();
});

async function doCreate() {
  err.value = '';
  try {
    await users.create({
      username: newUsername.value.trim(),
      displayName: newDisplay.value.trim() || undefined,
      avatarColor: newColor.value || undefined,
    });
    showCreate.value = false;
    newUsername.value = '';
    newDisplay.value = '';
    ui.toast('ok', t('users.toastCreated'));
  } catch (e) {
    err.value = (e as Error).message;
  }
}

async function doDelete(id: string, username: string) {
  if (!confirm(t('users.deleteConfirm', { name: username }))) return;
  try {
    await users.remove(id);
    ui.toast('ok', t('users.toastDeleted'));
  } catch (e) {
    ui.toast('err', t('users.toastDeleteFailed', { msg: (e as Error).message }));
  }
}

function avatarLetter(u: { displayName: string; username: string }): string {
  return (u.displayName || u.username || '?').slice(0, 1).toUpperCase();
}
</script>

<template>
  <div class="view">
    <header class="view-header">
      <h2>{{ t('users.profilePageTitle') }} <span class="dim sm">({{ users.list.length }})</span></h2>
      <button class="btn primary sm" @click="showCreate = !showCreate">{{ t('users.profileNewBtn') }}</button>
    </header>

    <p class="dim sm">{{ t('users.profilePageHint') }}</p>

    <div v-if="showCreate" class="create-form">
      <div class="form-row">
        <label>{{ t('users.usernameRule') }}</label>
        <input v-model="newUsername" placeholder="alice" />
      </div>
      <div class="form-row">
        <label>{{ t('users.displayNameLabel') }}</label>
        <input v-model="newDisplay" placeholder="Alice" />
      </div>
      <div class="form-row">
        <label>{{ t('users.avatarColor') }}</label>
        <input type="color" v-model="newColor" style="width: 80px" />
      </div>
      <div v-if="err" class="error">{{ err }}</div>
      <div class="row-actions">
        <button class="btn ghost" @click="showCreate = false">{{ t('common.cancel') }}</button>
        <button class="btn primary" @click="doCreate">{{ t('common.create') }}</button>
      </div>
    </div>

    <div class="profile-list">
      <div v-for="u in users.list" :key="u.id" class="profile-row" :class="{ active: u.id === users.activeId }">
        <span class="avatar" :style="{ background: u.avatarColor || '#4f46e5' }">
          {{ avatarLetter(u) }}
        </span>
        <div class="profile-info">
          <strong>{{ u.displayName }}</strong>
          <span class="dim xs">@{{ u.username }} · {{ u.id }}</span>
        </div>
        <div class="profile-actions">
          <label class="lang-toggle">
            <span class="dim xs">{{ t('users.languageLabel') }}</span>
            <select
              :value="u.language || 'zh-CN'"
              @change="(e) => setLang(u.id, (e.target as HTMLSelectElement).value as SupportedLocale)"
              class="lang-select"
            >
              <option value="zh-CN">{{ t('users.languageZhCN') }}</option>
              <option value="en">{{ t('users.languageEn') }}</option>
            </select>
          </label>
          <span v-if="u.id === users.activeId" class="pill ok sm">{{ t('users.activeBadge') }}</span>
          <button v-else class="btn ghost sm" @click="users.switchTo(u.id)">{{ t('users.switchBtn') }}</button>
          <button
            class="btn ghost sm danger"
            :disabled="users.list.length <= 1"
            :title="users.list.length <= 1 ? t('users.cannotDeleteLast') : ''"
            @click="doDelete(u.id, u.username)"
          >{{ t('common.delete') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.view {
  padding: var(--sp-3);
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  height: 100%;
  overflow-y: auto;
}

.view-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.create-form {
  border: 1px solid var(--border-1);
  border-radius: var(--radius-md);
  padding: var(--sp-3);
  background: var(--bg-2);
  margin-bottom: var(--sp-2);
}

.form-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: var(--sp-2);
}

.form-row label {
  font-size: var(--fs-xs);
  color: var(--fg-2);
}

.form-row input {
  padding: 6px 10px;
}

.row-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--sp-2);
}

.profile-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.profile-row {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: var(--sp-2) var(--sp-3);
  border: 1px solid var(--border-1);
  border-radius: var(--radius-sm);
  background: var(--bg-1);
}

.profile-row.active {
  border-color: var(--accent);
  background: var(--accent-bg);
}

.avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-weight: 600;
}

.profile-info {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.profile-actions {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
}

.pill.ok {
  background: var(--accent-bg);
  color: var(--accent);
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  font-size: var(--fs-xs);
}

.btn.danger {
  color: var(--status-err);
}

.lang-toggle {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-right: var(--sp-2);
}

.lang-select {
  font-size: var(--fs-xs);
  padding: 2px 6px;
}

.error {
  background: var(--status-err-bg);
  color: var(--status-err);
  padding: var(--sp-2);
  border-radius: var(--radius-sm);
  margin-bottom: var(--sp-2);
}
</style>
