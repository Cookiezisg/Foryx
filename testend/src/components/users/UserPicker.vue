<script setup lang="ts">
/**
 * UserPicker — startup screen.
 *
 * Shown when 2+ profiles exist and no active user is selected.
 * Single-profile mode auto-selects without showing this screen.
 */
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useUsersStore } from '@/stores/users';

const { t } = useI18n();
const users = useUsersStore();
const showCreate = ref(false);
const newUsername = ref('');
const newDisplay = ref('');
const err = ref<string>('');

async function create() {
  err.value = '';
  try {
    const u = await users.create({
      username: newUsername.value.trim(),
      displayName: newDisplay.value.trim() || undefined,
    });
    showCreate.value = false;
    newUsername.value = '';
    newDisplay.value = '';
    await users.switchTo(u.id);
  } catch (e) {
    err.value = (e as Error).message;
  }
}

function avatarLetter(u: { displayName: string; username: string }): string {
  return (u.displayName || u.username || '?').slice(0, 1).toUpperCase();
}
</script>

<template>
  <div class="picker">
    <div class="picker-card">
      <h1>{{ t('users.pickerTitle') }}</h1>
      <p class="dim">{{ t('users.pickerSubtitle') }}</p>

      <div class="profile-grid">
        <button
          v-for="u in users.list"
          :key="u.id"
          class="profile-tile"
          @click="users.switchTo(u.id)"
        >
          <span class="avatar" :style="{ background: u.avatarColor || '#4f46e5' }">
            {{ avatarLetter(u) }}
          </span>
          <span class="profile-name">{{ u.displayName || u.username }}</span>
          <span class="profile-username dim xs">@{{ u.username }}</span>
        </button>

        <button class="profile-tile dashed" @click="showCreate = true">
          <span class="avatar plus">+</span>
          <span class="profile-name">{{ t('users.newProfile') }}</span>
        </button>
      </div>

      <div v-if="showCreate" class="create-form">
        <div class="form-row">
          <label>{{ t('users.usernameLabel') }}</label>
          <input v-model="newUsername" placeholder="alice" autofocus @keyup.enter="create" />
        </div>
        <div class="form-row">
          <label>{{ t('users.displayNameLabel') }}</label>
          <input v-model="newDisplay" placeholder="Alice Wang" @keyup.enter="create" />
        </div>
        <div v-if="err" class="error">{{ err }}</div>
        <div class="row-actions">
          <button class="btn ghost" @click="showCreate = false">{{ t('common.cancel') }}</button>
          <button class="btn primary" @click="create">{{ t('users.createConfirm') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.picker {
  position: fixed;
  inset: 0;
  background: var(--bg-1);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.picker-card {
  width: 480px;
  max-width: 90vw;
  padding: var(--sp-5);
  background: var(--bg-2);
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-2);
  box-shadow: var(--shadow-2);
}

h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 700;
}

.profile-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  gap: var(--sp-3);
  margin-top: var(--sp-4);
}

.profile-tile {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--sp-1);
  padding: var(--sp-3);
  border: 1px solid var(--border-1);
  border-radius: var(--radius-md);
  background: var(--bg-1);
  cursor: pointer;
}

.profile-tile:hover {
  border-color: var(--accent);
}

.profile-tile.dashed {
  border-style: dashed;
}

.avatar {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-weight: 600;
  font-size: 20px;
  margin-bottom: 4px;
}

.avatar.plus {
  background: var(--bg-hover);
  color: var(--fg-2);
}

.profile-name {
  font-weight: 500;
  font-size: var(--fs-sm);
}

.profile-username {
  font-family: var(--font-mono);
}

.create-form {
  margin-top: var(--sp-4);
  padding-top: var(--sp-3);
  border-top: 1px solid var(--border-1);
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
  padding: 8px 10px;
  font-size: var(--fs-sm);
}

.row-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--sp-2);
  margin-top: var(--sp-2);
}

.error {
  background: var(--status-err-bg);
  color: var(--status-err);
  padding: var(--sp-2);
  border-radius: var(--radius-sm);
  font-size: var(--fs-sm);
}
</style>
