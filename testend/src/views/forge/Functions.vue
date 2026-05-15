<script setup lang="ts">
/**
 * Functions list — paginated table of all user-forged Python functions.
 * Click a row to navigate to /forge/functions/{id} (FunctionDetail).
 */
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { fnAPI } from '@/api/functions';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo, shortID, statusClass } from '@/utils/format';
import type { Function as Fn } from '@/types/domain';

const router = useRouter();
const ui = useUIStore();
const items = ref<Fn[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    const page = await fnAPI.list(200);
    items.value = page.items;
  } catch (e) {
    err.value = (e as Error).message;
    ui.toast('err', `加载函数失败: ${err.value}`);
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

function open(id: string) {
  router.push(`/forge/functions/${id}`);
}

function envCls(s?: string): string {
  return statusClass(s);
}
</script>

<template>
  <div class="view">
    <ViewHeader title="Functions" :subtitle="`${items.length} forged · stateless Python`">
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
            <th style="width: 120px">env</th>
            <th style="width: 120px">active version</th>
            <th style="width: 100px">updated</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="f in items" :key="f.id" class="row-clickable" @click="open(f.id)">
            <td>
              <div class="cell-name">{{ f.name }}</div>
              <div class="dim xs mono">{{ shortID(f.id, 10) }}</div>
            </td>
            <td class="ellipsis-cell">{{ f.description }}</td>
            <td>
              <span v-if="f.envStatus" class="pill" :class="envCls(f.envStatus)">{{ f.envStatus }}</span>
              <span v-else class="dim">—</span>
            </td>
            <td class="mono xs">{{ shortID(f.activeVersionId, 8) }}</td>
            <td class="dim xs">{{ timeAgo(f.updatedAt) }}</td>
            <td><button class="btn ghost sm" @click.stop="ui.showRaw(f.name, f)">raw</button></td>
          </tr>
          <tr v-if="!loading && items.length === 0">
            <td colspan="6" class="empty-row">
              No functions forged yet. Use the chat agent's <code>create_function</code> tool to forge one.
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
