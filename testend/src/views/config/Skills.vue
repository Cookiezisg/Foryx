<script setup lang="ts">
/**
 * Skills — Anthropic Agent Skills installed under ~/.forgify/skills/.
 * Each skill is a directory with a SKILL.md (frontmatter + body) that the
 * agent can invoke. Side panel previews the skill body.
 */
import { onMounted, ref } from 'vue';
import { skillAPI } from '@/api/resources';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo } from '@/utils/format';
import type { Skill } from '@/types/domain';

const ui = useUIStore();
const items = ref<Skill[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

const selected = ref<Skill | null>(null);
const body = ref<string>('');
const bodyLoading = ref(false);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    items.value = await skillAPI.list();
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

async function doRescan() {
  try {
    const r = await skillAPI.refresh();
    ui.toast('ok', `scanned ${r.count} skills`);
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function open(s: Skill) {
  selected.value = s;
  body.value = '';
  bodyLoading.value = true;
  try {
    const r = await skillAPI.body(s.name);
    body.value = r.body;
  } catch (e) {
    body.value = `(failed to load body: ${(e as Error).message})`;
  } finally {
    bodyLoading.value = false;
  }
}

onMounted(refresh);
</script>

<template>
  <div class="view">
    <ViewHeader title="Skills" :subtitle="`${items.length} installed in ~/.forgify/skills/`">
      <template #actions>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">refresh</button>
        <button class="btn primary sm" @click="doRescan">rescan</button>
      </template>
    </ViewHeader>
    <div class="split">
      <div class="col-list scroll">
        <div v-if="err" class="error">⨯ {{ err }}</div>
        <table class="table">
          <thead><tr><th>name</th><th style="width: 100px">updated</th></tr></thead>
          <tbody>
            <tr v-for="s in items" :key="s.name" class="row-clickable" :class="{ active: selected?.name === s.name }" @click="open(s)">
              <td>
                <div class="cell-name">{{ s.name }}</div>
                <div class="dim xs ellipsis">{{ s.description }}</div>
              </td>
              <td class="dim xs">{{ (s as any).updatedAt ? timeAgo((s as any).updatedAt) : '—' }}</td>
            </tr>
            <tr v-if="items.length === 0 && !loading">
              <td colspan="2" class="empty-row">No skills installed. Drop SKILL.md folders into <code>~/.forgify/skills/</code>.</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="col-detail scroll">
        <div v-if="!selected" class="dim center">Pick a skill to preview its body.</div>
        <div v-else>
          <header class="d-head">
            <strong>{{ selected.name }}</strong>
            <span class="dim xs">{{ selected.dirPath }}</span>
            <button class="btn ghost sm" @click="ui.showRaw(selected.name, selected)">raw</button>
          </header>
          <p class="dim small">{{ selected.description }}</p>
          <div v-if="bodyLoading" class="dim">loading body…</div>
          <pre v-else class="code-block mono">{{ body }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.split { flex: 1; display: flex; min-height: 0; }
.col-list { width: 380px; border-right: 1px solid var(--border-1); flex-shrink: 0; }
.col-detail { flex: 1; padding: var(--sp-3); }
.scroll { overflow: auto; }
.row-clickable { cursor: pointer; }
.row-clickable:hover td { background: var(--bg-hover); }
.row-clickable.active td { background: var(--bg-active); }
.cell-name { font-weight: 600; }
.ellipsis { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 280px; }
.d-head { display: flex; align-items: center; gap: var(--sp-2); margin-bottom: var(--sp-2); }
.center { padding: var(--sp-6); text-align: center; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
.empty-row code { background: var(--bg-2); padding: 1px 6px; border-radius: 4px; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
</style>
