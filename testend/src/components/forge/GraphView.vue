<script setup lang="ts">
/**
 * GraphView — read-only cytoscape rendering of a workflow Graph.
 *
 * Colour-codes nodes by NodeType (trigger / function / handler / mcp /
 * skill / llm / http / condition / loop / parallel / approval / wait /
 * variable) so testers can see the DAG shape at a glance. No editing —
 * graph mutations go through the forging path (chat agent + accept-pending).
 */
import { onMounted, onUnmounted, ref, shallowRef, watch } from 'vue';
import cytoscape, { type Core, type ElementDefinition } from 'cytoscape';
import type { Graph, NodeType } from '@/types/domain';

const props = defineProps<{ graph: Graph | undefined | null }>();

const container = ref<HTMLDivElement | null>(null);
const cy = shallowRef<Core | null>(null);

const NODE_COLOR: Record<NodeType, string> = {
  trigger: '#7c3aed',
  function: '#0ea5e9',
  handler: '#10b981',
  workflow: '#0ea5e9' as any, // tolerate stale enum values
  mcp: '#f59e0b',
  skill: '#a855f7',
  llm: '#f43f5e',
  http: '#06b6d4',
  condition: '#f97316',
  loop: '#fbbf24',
  parallel: '#ec4899',
  approval: '#dc2626',
  wait: '#9ca3af',
  variable: '#64748b',
} as Record<string, string>;

function toElements(g: Graph): ElementDefinition[] {
  const out: ElementDefinition[] = [];
  for (const n of g.nodes ?? []) {
    out.push({
      group: 'nodes',
      data: { id: n.id, label: `${n.type}\n${n.id}`, kind: n.type, raw: n },
      position: n.position,
    });
  }
  for (const e of g.edges ?? []) {
    out.push({ group: 'edges', data: { id: e.id, source: e.from, target: e.to } });
  }
  return out;
}

function render() {
  if (!container.value) return;
  if (cy.value) {
    cy.value.destroy();
    cy.value = null;
  }
  const g = props.graph;
  if (!g) return;
  cy.value = cytoscape({
    container: container.value,
    elements: toElements(g),
    style: [
      {
        selector: 'node',
        style: {
          shape: 'round-rectangle',
          'background-color': (ele: any) => NODE_COLOR[ele.data('kind') as NodeType] ?? '#94a3b8',
          label: 'data(label)',
          color: '#fff',
          'text-valign': 'center',
          'text-halign': 'center',
          'text-wrap': 'wrap',
          'text-max-width': '90px',
          'font-size': 10,
          width: 120,
          height: 50,
          'border-width': 1,
          'border-color': 'rgba(0,0,0,0.2)',
        },
      },
      {
        selector: 'edge',
        style: {
          width: 2,
          'line-color': '#94a3b8',
          'target-arrow-color': '#94a3b8',
          'target-arrow-shape': 'triangle',
          'curve-style': 'bezier',
        },
      },
    ],
    layout: g.nodes?.some((n) => n.position) ? { name: 'preset' } : { name: 'breadthfirst', directed: true, spacingFactor: 1.4 },
    wheelSensitivity: 0.2,
  });
}

onMounted(render);
onUnmounted(() => cy.value?.destroy());
watch(() => props.graph, render, { deep: true });
</script>

<template>
  <div ref="container" class="cy"></div>
</template>

<style scoped>
.cy {
  width: 100%;
  height: 100%;
  min-height: 400px;
  background: var(--bg-2);
  border-radius: var(--radius-sm);
}
</style>
