<script setup lang="ts">
/**
 * App root — 4-column persistent layout.
 *
 *   [Col1 conv list] | [Col2 chat] | [Col3 tab nav] | [Col4 tab content]
 *
 * Col4 "expand" mode collapses col1+col2 into 40px icon rails so
 * content-heavy views (graph editor, SQL, wide tables) get the floor.
 */
import { onMounted, onUnmounted, computed } from 'vue';
import TopBar from '@/components/layout/TopBar.vue';
import ConvSidebar from '@/components/layout/ConvSidebar.vue';
import ChatPanel from '@/components/layout/ChatPanel.vue';
import TabNav from '@/components/layout/TabNav.vue';
import ResizableSplit from '@/components/layout/ResizableSplit.vue';
import ToastTray from '@/components/common/ToastTray.vue';
import RawJsonModal from '@/components/common/RawJsonModal.vue';
import CommandPalette from '@/components/common/CommandPalette.vue';
import { useUIStore } from '@/stores/ui';
import { useChatStore } from '@/stores/chat';
import { useNotificationsStore } from '@/stores/notifications';
import { useForgeStore } from '@/stores/forge';
import { useCatalogStore } from '@/stores/catalog';

const ui = useUIStore();
const chat = useChatStore();
const notifs = useNotificationsStore();
const forge = useForgeStore();
const catalog = useCatalogStore();

const expanded = computed(() => ui.expanded);

function onKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
    e.preventDefault();
    ui.openPalette();
  }
  if (e.key === 'Escape') {
    if (ui.palette) ui.closePalette();
    if (ui.rawJson.open) ui.closeRaw();
  }
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown);
  // Start global subscriptions: chat eventlog, notifications, forge.
  chat.startSSE();
  notifs.start();
  forge.start();
  catalog.refresh();
});

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown);
  chat.stopSSE();
  notifs.stop();
  forge.stop();
});
</script>

<template>
  <div class="app-root">
    <TopBar />

    <div class="layout" :class="{ expanded }">
      <ResizableSplit
        v-if="!expanded"
        :left-width="ui.colConv"
        :min-left="140"
        :max-left="380"
        @resize="(w: number) => (ui.colConv = w)"
      >
        <template #left>
          <ConvSidebar />
        </template>
        <template #right>
          <ResizableSplit
            :left-width="ui.colChat"
            :min-left="320"
            :max-left="900"
            @resize="(w: number) => (ui.colChat = w)"
          >
            <template #left>
              <ChatPanel />
            </template>
            <template #right>
              <ResizableSplit
                :left-width="ui.colNav"
                :min-left="180"
                :max-left="320"
                @resize="(w: number) => (ui.colNav = w)"
              >
                <template #left>
                  <TabNav />
                </template>
                <template #right>
                  <main class="tab-content">
                    <RouterView />
                  </main>
                </template>
              </ResizableSplit>
            </template>
          </ResizableSplit>
        </template>
      </ResizableSplit>

      <!-- expanded mode: 40px rail (icons-only conv + tab nav) + full-width content -->
      <div v-else class="expanded-layout">
        <aside class="rail">
          <ConvSidebar :rail="true" />
        </aside>
        <aside class="rail">
          <TabNav :rail="true" />
        </aside>
        <main class="tab-content expanded">
          <RouterView />
        </main>
      </div>
    </div>

    <CommandPalette v-if="ui.palette" />
    <RawJsonModal v-if="ui.rawJson.open" />
    <ToastTray />
  </div>
</template>

<style scoped>
.app-root {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.layout {
  flex: 1;
  display: flex;
  min-height: 0;
}

.expanded-layout {
  flex: 1;
  display: flex;
  min-width: 0;
}

.rail {
  width: 40px;
  flex-shrink: 0;
  background: var(--bg-1);
  border-right: 1px solid var(--border-1);
}

.tab-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-0);
  overflow: hidden;
}
</style>
