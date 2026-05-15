<script setup lang="ts">
/**
 * Handlers list — paginated table of all user-forged stateful Handlers.
 * Click a row to navigate to /forge/handlers/{id}.
 */
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { hdAPI } from '@/api/handlers';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo, shortID, statusClass } from '@/utils/format';
import type { Handler } from '@/types/domain';

const router = useRouter();
const ui = useUIStore();
const items = ref<Handler[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    const page = await hdAPI.list(200);
    items.value = page.items;
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

function open(id: string) {
  router.push(`/forge/handlers/${id}`);
}
</script>

<template>
  <div class="view">
    <ViewHeader title="Handlers" :subtitle="`${items.length} forged · stateful, init-args + methods`">
      <template #actions>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">{{ loading ? '...' : 'refresh' }}</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <table class="table">
        <thead>
          <tr>
            <th style="width: 200px">name</th>
            <th>description</th>
            <th style="width: 120px">config</th>
            <th style="width: 100px">env</th>
            <th style="width: 60px">live</th>
            <th style="width: 100px">updated</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="h in items" :key="h.id" class="row-clickable" @click="open(h.id)">
            <td>
              <div class="cell-name">{{ h.name }}</div>
              <div class="dim xs mono">{{ shortID(h.id, 10) }}</div>
            </td>
            <td class="ellipsis-cell">{{ h.description }}</td>
            <td>
              <span v-if="h.configState" class="pill" :class="statusClass(h.configState)">{{ h.configState }}</span>
              <span v-else class="dim">—</span>
            </td>
            <td>
              <span v-if="h.envStatus" class="pill" :class="statusClass(h.envStatus)">{{ h.envStatus }}</span>
            </td>
            <td class="mono xs">{{ h.liveInstances ?? 0 }}</td>
            <td class="dim xs">{{ timeAgo(h.updatedAt) }}</td>
            <td><button class="btn ghost sm" @click.stop="ui.showRaw(h.name, h)">raw</button></td>
          </tr>
          <tr v-if="!loading && items.length === 0">
            <td colspan="7" class="empty-row">
              No handlers forged yet. Use chat agent's <code>create_handler</code> tool to forge one.
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: 0 var(--sp-3) var(--sp-3); }
.row-clickable { cursor: pointer; }
.row-clickable:hover td { background: var(--bg-hover); }
.cell-name { font-weight: 600; }
.ellipsis-cell { max-width: 500px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
.empty-row code { background: var(--bg-2); padding: 1px 6px; border-radius: 4px; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); margin: var(--sp-2) var(--sp-3); }
</style>
