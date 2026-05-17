<script setup lang="ts">
/**
 * UserSwitcher — header avatar dropdown.
 *
 * Shows active profile; click → dropdown to switch or manage profiles.
 */
import { ref, computed } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useUsersStore } from '@/stores/users';

const { t } = useI18n();
const users = useUsersStore();
const router = useRouter();
const open = ref(false);

const active = computed(() => users.active);
const inactive = computed(() => users.list.filter((u) => u.id !== users.activeId));

function avatarLetter(u: { displayName: string; username: string }): string {
  return (u.displayName || u.username || '?').slice(0, 1).toUpperCase();
}

function close() {
  open.value = false;
}

async function pick(id: string) {
  close();
  await users.switchTo(id);
}

function manage() {
  close();
  router.push('/config/profile');
}
</script>

<template>
  <div v-if="active" class="user-switcher" v-click-outside="close">
    <button class="trigger" @click="open = !open" :title="active.displayName">
      <span class="avatar sm" :style="{ background: active.avatarColor || '#4f46e5' }">
        {{ avatarLetter(active) }}
      </span>
    </button>

    <div v-if="open" class="dropdown">
      <header class="header">
        <span class="avatar md" :style="{ background: active.avatarColor || '#4f46e5' }">
          {{ avatarLetter(active) }}
        </span>
        <div class="header-text">
          <strong>{{ active.displayName }}</strong>
          <span class="dim xs">@{{ active.username }}</span>
        </div>
      </header>

      <div v-if="inactive.length > 0" class="section">
        <div class="section-label dim xs">{{ t('users.switcherTitle') }}</div>
        <button
          v-for="u in inactive"
          :key="u.id"
          class="profile-row"
          @click="pick(u.id)"
        >
          <span class="avatar sm" :style="{ background: u.avatarColor || '#4f46e5' }">
            {{ avatarLetter(u) }}
          </span>
          <span class="profile-name">{{ u.displayName }}</span>
          <span class="profile-username dim xs">@{{ u.username }}</span>
        </button>
      </div>

      <div class="section">
        <button class="profile-row" @click="manage">
          <span class="action-icon">⚙</span>
          <span>{{ t('users.manageProfile') }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
// click-outside directive — collapses dropdown on outside click.
// click-outside directive：点空白处折叠 dropdown。
import type { Directive } from 'vue';
const clickOutside: Directive<HTMLElement, () => void> = {
  mounted(el, binding) {
    const handler = (e: Event) => {
      if (!el.contains(e.target as Node)) binding.value();
    };
    el.__clickOutside__ = handler;
    document.addEventListener('click', handler);
  },
  unmounted(el) {
    if (el.__clickOutside__) document.removeEventListener('click', el.__clickOutside__);
  },
};
declare global {
  interface HTMLElement {
    __clickOutside__?: (e: Event) => void;
  }
}
export default { directives: { 'click-outside': clickOutside } };
</script>

<style scoped>
.user-switcher {
  position: relative;
}

.trigger {
  background: transparent;
  border: none;
  padding: 0;
  cursor: pointer;
}

.avatar {
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-weight: 600;
}

.avatar.sm {
  width: 28px;
  height: 28px;
  font-size: 12px;
}

.avatar.md {
  width: 40px;
  height: 40px;
  font-size: 16px;
}

.dropdown {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  width: 260px;
  background: var(--bg-2);
  border: 1px solid var(--border-2);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-2);
  z-index: 100;
  overflow: hidden;
}

.header {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: var(--sp-3);
  border-bottom: 1px solid var(--border-1);
}

.header-text {
  display: flex;
  flex-direction: column;
}

.section {
  padding: 4px 0;
  border-bottom: 1px solid var(--border-1);
}

.section:last-child {
  border-bottom: none;
}

.section-label {
  padding: 4px var(--sp-3);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.profile-row {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  width: 100%;
  padding: 8px var(--sp-3);
  background: transparent;
  border: none;
  cursor: pointer;
  text-align: left;
  font-size: var(--fs-sm);
  color: var(--fg-1);
}

.profile-row:hover {
  background: var(--bg-hover);
}

.action-icon {
  width: 28px;
  text-align: center;
  color: var(--fg-3);
}

.profile-name {
  flex: 1;
}

.profile-username {
  font-family: var(--font-mono);
}
</style>
