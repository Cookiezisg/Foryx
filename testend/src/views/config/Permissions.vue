<script setup lang="ts">
/**
 * Permissions — V1.2 §3 final-sweep settings.json editor + tool danger
 * level inspector + rule test. Single page with 3 tabs: Rules / Tools /
 * Test. Saves write the whole settings.json atomically.
 *
 * Permissions ——V1.2 §3 settings.json 编辑器 + tool 危险等级看板 + 规则
 * 测试。单页 3 tab：Rules / Tools / Test。保存原子写整个 settings.json。
 */
import { onMounted, ref, computed } from 'vue';
import { permissionsAPI, type Settings, type ToolRow, type Action } from '@/api/resources';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';

const ui = useUIStore();

const tab = ref<'rules' | 'tools' | 'test'>('rules');

const settings = ref<Settings>({ permissions: {} });
const tools = ref<ToolRow[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

// rule textareas as multi-line strings; convert to/from arrays on save/load
const denyText = ref('');
const askText = ref('');
const allowText = ref('');

// test panel state
const testTool = ref('');
const testArgsJSON = ref('{}');
const testDestructive = ref(false);
const testResult = ref<{ action: Action; reason: string } | null>(null);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    settings.value = await permissionsAPI.getSettings();
    settings.value.permissions ??= {};
    denyText.value = (settings.value.permissions.deny ?? []).join('\n');
    askText.value = (settings.value.permissions.ask ?? []).join('\n');
    allowText.value = (settings.value.permissions.allow ?? []).join('\n');
    tools.value = await permissionsAPI.listTools();
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

function parseLines(s: string): string[] {
  return s
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean);
}

async function save() {
  try {
    const next: Settings = {
      permissions: {
        defaultMode: settings.value.permissions.defaultMode ?? 'ask',
        deny: parseLines(denyText.value),
        ask: parseLines(askText.value),
        allow: parseLines(allowText.value),
      },
      hooks: settings.value.hooks,
      protectedPaths: settings.value.protectedPaths,
    };
    await permissionsAPI.putSettings(next);
    ui.toast('ok', 'settings saved + reloaded');
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function reload() {
  try {
    await permissionsAPI.reload();
    ui.toast('ok', 'settings reloaded from disk');
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function runTest() {
  try {
    let args: unknown;
    try {
      args = JSON.parse(testArgsJSON.value);
    } catch {
      ui.toast('err', 'args is not valid JSON');
      return;
    }
    testResult.value = await permissionsAPI.test({
      toolName: testTool.value,
      args,
      destructive: testDestructive.value,
    });
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

const dangerLevelLabel = (l: string) => l.replace(/_/g, ' ');
const dangerLevelClass = (l: string) => `pill level-${l}`;
const actionClass = (a: string) => `pill action-${a}`;

const grouped = computed(() => {
  const groups: Record<string, ToolRow[]> = {
    read_only: [],
    workspace_write: [],
    danger_full_access: [],
  };
  for (const r of tools.value) {
    (groups[r.dangerLevel] ??= []).push(r);
  }
  return groups;
});
</script>

<template>
  <div class="view">
    <ViewHeader title="Permissions" :subtitle="`${tools.length} tools registered`">
      <template #actions>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">refresh</button>
        <button class="btn ghost sm" @click="reload">reload from disk</button>
      </template>
    </ViewHeader>

    <nav class="tabs">
      <button
        v-for="t in (['rules', 'tools', 'test'] as const)"
        :key="t"
        class="tab"
        :class="{ active: tab === t }"
        @click="tab = t"
      >
        {{ t }}
      </button>
    </nav>

    <div v-if="err" class="error">⨯ {{ err }}</div>

    <!-- Rules tab -->
    <section v-if="tab === 'rules'" class="scroll">
      <div class="field">
        <label>defaultMode</label>
        <select v-model="settings.permissions.defaultMode">
          <option value="ask">ask (default)</option>
          <option value="allow">allow</option>
          <option value="deny">deny</option>
          <option value="bypass">bypass</option>
        </select>
      </div>
      <div class="field">
        <label>deny rules (one per line)</label>
        <textarea v-model="denyText" rows="6" placeholder='Bash(rm -rf *)
Read(.env)'></textarea>
      </div>
      <div class="field">
        <label>ask rules</label>
        <textarea v-model="askText" rows="6" placeholder='Bash(git push *)
Write(~/**)'></textarea>
      </div>
      <div class="field">
        <label>allow rules</label>
        <textarea v-model="allowText" rows="6" placeholder='Bash(npm *)
Read(./**)'></textarea>
      </div>
      <button class="btn primary" @click="save">Save settings.json</button>
    </section>

    <!-- Tools tab -->
    <section v-if="tab === 'tools'" class="scroll">
      <div v-for="(rows, level) in grouped" :key="level" class="level-block">
        <h4>
          <span :class="dangerLevelClass(level)">{{ dangerLevelLabel(level) }}</span>
          <span class="dim small">{{ rows.length }} tools</span>
        </h4>
        <table class="table">
          <thead>
            <tr><th style="width: 220px">name</th><th>description</th></tr>
          </thead>
          <tbody>
            <tr v-for="r in rows" :key="r.name">
              <td class="mono">{{ r.name }}</td>
              <td class="dim small ellipsis">{{ r.description }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- Test tab -->
    <section v-if="tab === 'test'" class="scroll">
      <p class="dim small">
        Test how the current rules would decide a tool call. No side
        effects — gate.Evaluate is called with a synthetic session ID.
      </p>
      <div class="field">
        <label>tool name</label>
        <input v-model="testTool" placeholder="Bash" />
      </div>
      <div class="field">
        <label>args (JSON)</label>
        <textarea v-model="testArgsJSON" rows="4"></textarea>
      </div>
      <div class="field row">
        <label class="inline">
          <input type="checkbox" v-model="testDestructive" />
          destructive (LLM self-declared)
        </label>
      </div>
      <button class="btn primary" @click="runTest" :disabled="!testTool">Test</button>

      <div v-if="testResult" class="test-result">
        <h4>Result</h4>
        <p>
          <strong>Action:</strong>
          <span :class="actionClass(testResult.action)">{{ testResult.action }}</span>
        </p>
        <p><strong>Reason:</strong> <span class="dim">{{ testResult.reason }}</span></p>
      </div>
    </section>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.tabs { display: flex; gap: var(--sp-1); padding: 0 var(--sp-3); border-bottom: 1px solid var(--border-1); }
.tab { background: transparent; border: 0; padding: var(--sp-2) var(--sp-3); cursor: pointer; color: var(--fg-2); }
.tab.active { color: var(--fg-1); border-bottom: 2px solid var(--accent); }
.scroll { flex: 1; overflow: auto; padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-3); }
.field { display: flex; flex-direction: column; gap: var(--sp-1); }
.field label { font-size: var(--fs-xs); color: var(--fg-2); }
.field textarea, .field input, .field select { width: 100%; font-family: var(--mono); }
.field.row { flex-direction: row; align-items: center; }
.field .inline { display: flex; align-items: center; gap: var(--sp-1); }
.level-block { display: flex; flex-direction: column; gap: var(--sp-2); }
.level-block h4 { display: flex; align-items: center; gap: var(--sp-2); margin: 0; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); margin: var(--sp-2); }
.ellipsis { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 600px; }
.test-result { background: var(--bg-1); border: 1px solid var(--border-1); padding: var(--sp-3); border-radius: var(--radius-md); }
.test-result h4 { margin: 0 0 var(--sp-2); }
.pill.level-read_only { background: #10b981; color: white; }
.pill.level-workspace_write { background: #f59e0b; color: #1f2937; }
.pill.level-danger_full_access { background: #ef4444; color: white; }
.pill.action-allow { background: #10b981; color: white; }
.pill.action-ask { background: #f59e0b; color: #1f2937; }
.pill.action-deny { background: #ef4444; color: white; }
</style>
