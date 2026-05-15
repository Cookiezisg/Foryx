<script setup lang="ts">
/**
 * SQL — read-only SELECT console against the live SQLite DB. Schema sidebar
 * lists every user table with row counts + column definitions; clicking a
 * table inserts a `SELECT * FROM <name> LIMIT 100;` template into the editor.
 */
import { onMounted, ref } from 'vue';
import { devAPI } from '@/api/dev';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { bytes } from '@/utils/format';

interface Col {
  name: string;
  type: string;
  notNull: boolean;
  pk: boolean;
  default?: string;
}
interface Table { name: string; rowCount: number; columns: Col[] }

const ui = useUIStore();
const schema = ref<Table[]>([]);
const expanded = ref<Set<string>>(new Set());
const filter = ref('');

const sql = ref('SELECT * FROM conversations LIMIT 50;');
const result = ref<{ columns: string[]; rows: unknown[][] } | null>(null);
const queryErr = ref<string | null>(null);
const busy = ref(false);

async function loadSchema() {
  try {
    schema.value = await devAPI.schema();
  } catch (e) {
    ui.toast('err', `加载 schema 失败: ${(e as Error).message}`);
  }
}

onMounted(loadSchema);

async function run() {
  busy.value = true;
  queryErr.value = null;
  result.value = null;
  try {
    const r = await devAPI.sql(sql.value);
    if (r.error) {
      queryErr.value = r.error;
    } else {
      result.value = { columns: r.columns, rows: r.rows };
    }
  } catch (e) {
    queryErr.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}

function pickTable(name: string) {
  sql.value = `SELECT * FROM ${name} LIMIT 100;`;
}

function toggleTable(name: string) {
  if (expanded.value.has(name)) expanded.value.delete(name);
  else expanded.value.add(name);
  expanded.value = new Set(expanded.value);
}

function filtered(): Table[] {
  const q = filter.value.trim().toLowerCase();
  if (!q) return schema.value;
  return schema.value.filter((t) => t.name.toLowerCase().includes(q));
}
</script>

<template>
  <div class="view">
    <ViewHeader title="SQL" subtitle="read-only SELECT against the live DB" />
    <div class="split">
      <aside class="sidebar scroll">
        <input v-model="filter" placeholder="filter tables…" class="sm" />
        <div v-for="t in filtered()" :key="t.name" class="t-row">
          <button class="t-head" @click="toggleTable(t.name)">
            <span class="caret">{{ expanded.has(t.name) ? '▾' : '▸' }}</span>
            <span class="t-name mono">{{ t.name }}</span>
            <span class="dim xs">{{ t.rowCount }}</span>
            <button class="btn ghost icon sm" @click.stop="pickTable(t.name)" title="insert SELECT *">↓</button>
          </button>
          <ul v-if="expanded.has(t.name)" class="t-cols">
            <li v-for="c in t.columns" :key="c.name">
              <span class="mono xs">{{ c.name }}</span>
              <span class="dim xs">{{ c.type }}</span>
              <span v-if="c.pk" class="pill info xs">pk</span>
              <span v-if="c.notNull" class="pill xs">!null</span>
            </li>
          </ul>
        </div>
      </aside>
      <main class="main">
        <div class="editor">
          <textarea v-model="sql" rows="6" class="mono" />
          <div class="row-actions">
            <button class="btn primary" :disabled="busy" @click="run">{{ busy ? '...' : 'Run' }}</button>
            <button class="btn ghost" @click="sql = ''">clear</button>
            <span v-if="result" class="dim small">{{ result.rows.length }} rows · {{ bytes(JSON.stringify(result.rows).length) }}</span>
          </div>
        </div>
        <div v-if="queryErr" class="error">{{ queryErr }}</div>
        <div v-if="result" class="result-scroll">
          <table class="table sm">
            <thead><tr><th v-for="c in result.columns" :key="c">{{ c }}</th></tr></thead>
            <tbody>
              <tr v-for="(row, i) in result.rows" :key="i" @click="ui.showRaw(`row ${i}`, row)" class="row-clickable">
                <td v-for="(v, j) in row" :key="j" class="mono xs cell">{{ v === null ? '∅' : String(v) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </main>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.split { flex: 1; display: flex; min-height: 0; }
.sidebar { width: 280px; border-right: 1px solid var(--border-1); padding: var(--sp-2); flex-shrink: 0; }
.sidebar input { width: 100%; margin-bottom: var(--sp-2); }
.t-row { margin-bottom: 4px; }
.t-head {
  display: flex; align-items: center; gap: 4px; width: 100%;
  padding: 4px; background: transparent; cursor: pointer; text-align: left;
}
.t-head:hover { background: var(--bg-hover); }
.t-name { font-size: var(--fs-sm); flex: 1; }
.caret { color: var(--fg-3); width: 10px; }
.t-cols { list-style: none; margin: 0; padding: 4px 0 4px 20px; }
.t-cols li { display: flex; align-items: center; gap: 6px; font-size: var(--fs-xs); padding: 1px 0; }
.main { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.editor { padding: var(--sp-2) var(--sp-3); border-bottom: 1px solid var(--border-1); }
.editor textarea { font-family: var(--font-mono); width: 100%; resize: vertical; }
.row-actions { display: flex; gap: var(--sp-2); align-items: center; margin-top: var(--sp-1); }
.result-scroll { flex: 1; overflow: auto; padding: var(--sp-3); }
.cell { max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row-clickable { cursor: pointer; }
.row-clickable:hover td { background: var(--bg-hover); }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); margin: var(--sp-2) var(--sp-3); }
.pill.xs { font-size: 9px; padding: 0 4px; }
</style>
