/**
 * Catalog store — caches the latest Capability Catalog snapshot.
 * Used by the topbar pill + OBSERVE › Catalog view.
 */

import { defineStore } from 'pinia';
import { ref } from 'vue';
import { catalogAPI } from '@/api/resources';
import type { Catalog } from '@/types/domain';

export const useCatalogStore = defineStore('catalog', () => {
  const current = ref<Catalog | null>(null);
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function refresh() {
    loading.value = true;
    error.value = null;
    try {
      current.value = await catalogAPI.get();
    } catch (e) {
      error.value = (e as Error).message;
    } finally {
      loading.value = false;
    }
  }

  async function forceRebuild() {
    loading.value = true;
    error.value = null;
    try {
      current.value = await catalogAPI.refresh();
    } catch (e) {
      error.value = (e as Error).message;
    } finally {
      loading.value = false;
    }
  }

  return { current, loading, error, refresh, forceRebuild };
});
