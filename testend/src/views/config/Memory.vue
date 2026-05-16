<script setup lang="ts">
/**
 * Memory — cross-conversation long-term facts (V1.2 §2 final-sweep).
 * One row per entry; type/source/pinned badges. Pinned entries inject
 * full Content into every system prompt; non-pinned only show in the
 * memory index (name + description).
 *
 * Memory ——跨对话长期事实（V1.2 §2 final-sweep）。每条一行；type /
 * source / pinned 三个 badge。pinned 把全文塞每个 system prompt；非 pinned
 * 只在 memory index 显示 name + description。
 */
import { onMounted, ref, computed } from 'vue';
import { memoryAPI } from '@/api/resources';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo } from '@/utils/format';
import type { Memory } from '@/types/domain';

const ui = useUIStore();
const items = ref<Memory[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

const showAdd = ref(false);
const editing = ref<Memory | null>(null);

interface DraftMemory {
  name: string;
  type: Memory['type'];
  description: string;
  content: string;
  pinned: boolean;
}

const draft = ref<DraftMemory>({
  name: '',
  type: 'user',
  description: '',
  content: '',
  pinned: false,
});

const pinnedCount = computed(() => items.value.filter((m) => m.pinned).length);
const aiCount = computed(() => items.value.filter((m) => m.source === 'ai').length);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    items.value = await memoryAPI.list();
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

function startEdit(m: Memory) {
  editing.value = m;
  draft.value = {
    name: m.name,
    type: m.type,
    description: m.description,
    content: m.content,
    pinned: m.pinned,
  };
  showAdd.value = true;
}

function startAdd() {
  editing.value = null;
  draft.value = { name: '', type: 'user', description: '', content: '', pinned: false };
  showAdd.value = true;
}

async function save() {
  try {
    if (editing.value) {
      await memoryAPI.update(editing.value.name, {
        type: draft.value.type,
        description: draft.value.description,
        content: draft.value.content,
        pinned: draft.value.pinned,
      });
      ui.toast('ok', `memory 更新: ${draft.value.name}`);
    } else {
      await memoryAPI.create({
        name: draft.value.name,
        type: draft.value.type,
        description: draft.value.description,
        content: draft.value.content,
        pinned: draft.value.pinned,
      });
      ui.toast('ok', `memory 创建: ${draft.value.name}`);
    }
    showAdd.value = false;
    editing.value = null;
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function togglePin(m: Memory) {
  try {
    if (m.pinned) {
      await memoryAPI.unpin(m.name);
      ui.toast('ok', `${m.name} unpinned`);
    } else {
      await memoryAPI.pin(m.name);
      ui.toast('ok', `${m.name} pinned`);
    }
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function doDelete(m: Memory) {
  if (!confirm(`删除 memory "${m.name}"?`)) return;
  try {
    await memoryAPI.remove(m.name);
    ui.toast('ok', 'deleted');
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

function typeClass(t: string) {
  return `pill type-${t}`;
}

function sourceClass(s: string) {
  return `pill source-${s}`;
}
</script>

<template>
  <div class="view">
    <ViewHeader
      title="Memory"
      :subtitle="`${items.length} entries · ${pinnedCount} pinned · ${aiCount} from AI`"
    >
      <template #actions>
        <button class="btn primary sm" @click="startAdd">+ New</button>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">refresh</button>
      </template>
    </ViewHeader>

    <section v-if="showAdd" class="add-form">
      <h4>{{ editing ? `Edit ${editing.name}` : 'New memory' }}</h4>

      <label class="field-label">name</label>
      <input
        v-model="draft.name"
        :disabled="!!editing"
        placeholder="lowercase_with_underscores"
      />

      <label class="field-label">type</label>
      <select v-model="draft.type">
        <option value="user">user (about the user)</option>
        <option value="feedback">feedback (preferences / corrections)</option>
        <option value="project">project (current work)</option>
        <option value="reference">reference (pointers to external systems)</option>
      </select>

      <label class="field-label">description</label>
      <input
        v-model="draft.description"
        placeholder="One-line summary (shown in index)"
      />

      <label class="field-label">content</label>
      <textarea v-model="draft.content" rows="6" placeholder="Full markdown body"></textarea>

      <label class="field-label">pinned</label>
      <div class="row">
        <input type="checkbox" v-model="draft.pinned" id="pin" />
        <label for="pin" class="dim small">pinned memories' full content sits in every system prompt</label>
      </div>

      <button
        class="btn primary"
        @click="save"
        :disabled="!draft.name || !draft.description || !draft.content"
      >
        {{ editing ? 'Save' : 'Create' }}
      </button>
    </section>

    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <table class="table">
        <thead>
          <tr>
            <th style="width: 200px">name</th>
            <th style="width: 110px">type</th>
            <th style="width: 90px">source</th>
            <th>description</th>
            <th style="width: 110px">updated</th>
            <th style="width: 70px">accessed</th>
            <th style="width: 220px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="m in items" :key="m.id">
            <td>
              <div class="mono">{{ m.name }}</div>
              <div class="dim xs">
                <span v-if="m.pinned" class="pill pinned">📌 pinned</span>
              </div>
            </td>
            <td><span :class="typeClass(m.type)">{{ m.type }}</span></td>
            <td><span :class="sourceClass(m.source)">{{ m.source }}</span></td>
            <td class="dim small">{{ m.description }}</td>
            <td class="dim xs">{{ timeAgo(m.updatedAt) }}</td>
            <td class="dim xs">{{ m.accessCount ?? 0 }}</td>
            <td>
              <button class="btn ghost sm" @click="togglePin(m)">
                {{ m.pinned ? 'unpin' : 'pin' }}
              </button>
              <button class="btn ghost sm" @click="startEdit(m)">edit</button>
              <button class="btn danger sm" @click="doDelete(m)">delete</button>
              <button class="btn ghost sm" @click="ui.showRaw(m.name, m)">raw</button>
            </td>
          </tr>
          <tr v-if="!loading && items.length === 0">
            <td colspan="7" class="empty-row">
              No memories yet. Click + New, or let the LLM write one via write_memory.
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
.add-form {
  background: var(--bg-1);
  border-bottom: 1px solid var(--border-1);
  padding: var(--sp-3);
  display: grid;
  grid-template-columns: 140px 1fr;
  gap: var(--sp-1) var(--sp-2);
  align-items: start;
}
.add-form h4 { grid-column: 1 / -1; margin: 0 0 var(--sp-2); }
.add-form input, .add-form select, .add-form textarea { width: 100%; }
.add-form button { grid-column: 2; justify-self: start; margin-top: var(--sp-2); }
.field-label { font-size: var(--fs-xs); color: var(--fg-2); justify-self: end; text-align: right; padding-top: 6px; }
.row { display: flex; align-items: center; gap: var(--sp-2); }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
.pill.type-user { background: #2563eb; color: white; }
.pill.type-feedback { background: #f59e0b; color: #1f2937; }
.pill.type-project { background: #10b981; color: white; }
.pill.type-reference { background: #8b5cf6; color: white; }
.pill.source-user { background: var(--bg-2); color: var(--fg-2); }
.pill.source-ai { background: #6366f1; color: white; }
.pill.pinned { background: #f59e0b; color: #1f2937; }
</style>
