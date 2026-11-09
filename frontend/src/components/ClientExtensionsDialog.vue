<script setup lang="ts">
import { onUnmounted, ref, useId, watch } from 'vue'
import { Plus } from '@lucide/vue'
import type { ExtensionClient } from '../api'
import { useExtensionStore } from '../stores/extensions'
import ExtensionWindow from './ExtensionWindow.vue'
import ExtensionOptionsList from './ExtensionOptionsList.vue'
import ExtensionTypeCatalog from './ExtensionTypeCatalog.vue'
import ExtensionEditorDialog from './ExtensionEditorDialog.vue'
const props = defineProps<{ open: boolean; client?: ExtensionClient }>()
const emit = defineEmits<{ close: [] }>()
const ext = useExtensionStore()
let stop: (() => void) | undefined
const page = ref<'instances' | 'catalog'>('instances')
const createOpen = ref(false)
const panelId = useId()
function onTabKeydown(event: KeyboardEvent) {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  page.value = event.key === 'Home' ? 'catalog' : event.key === 'End' ? 'instances' : page.value === 'instances' ? 'catalog' : 'instances'
  document.getElementById(panelId + '-' + page.value + '-tab')?.focus()
}
watch(() => props.open, (open) => { stop?.(); stop = open ? ext.watchOptions() : undefined; page.value = 'instances'; createOpen.value = false }, { immediate:true })
watch(() => props.client?.id, () => { page.value = 'instances'; createOpen.value = false })
onUnmounted(() => stop?.())
</script>
<template>
  <ExtensionWindow :open="open" :title="(client?.name ?? '') + ' · 扩展'" wide @close="emit('close')">
    <div class="extensions-toolbar"><div role="tablist" aria-label="扩展页面" class="extension-tabs" @keydown="onTabKeydown">
      <button :id="panelId + '-catalog-tab'" type="button" role="tab" :aria-selected="page === 'catalog'" :aria-controls="panelId" :tabindex="page === 'catalog' ? 0 : -1" class="tab-button" :class="{ active: page === 'catalog' }" @click="page = 'catalog'">所有扩展</button>
      <button :id="panelId + '-instances-tab'" type="button" role="tab" :aria-selected="page === 'instances'" :aria-controls="panelId" :tabindex="page === 'instances' ? 0 : -1" class="tab-button" :class="{ active: page === 'instances' }" @click="page = 'instances'">已创建扩展</button>
    </div><button type="button" class="btn btn-primary" @click="createOpen = true"><Plus :size="14" aria-hidden="true" />新建扩展</button></div>
    <div :id="panelId" role="tabpanel" :aria-labelledby="panelId + (page === 'instances' ? '-instances-tab' : '-catalog-tab')">
      <ExtensionOptionsList v-if="client && page === 'instances'" :key="client.id" :client-id="client.id" />
      <template v-else><p v-if="ext.optionsError" class="catalog-error" role="alert">{{ ext.optionsError }}<button type="button" class="btn btn-ghost btn-sm" @click="ext.refreshOptions">重试</button></p><ExtensionTypeCatalog :types="ext.types" /></template>
    </div>
  </ExtensionWindow>
  <ExtensionEditorDialog v-if="client" :open="createOpen" :client-id="client.id" @saved="page = 'instances'" @close="createOpen = false" />
</template>
<style scoped>
.extensions-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 20px; }.extension-tabs { display: flex; align-items: center; gap: 4px; }.tab-button { height: 32px; padding: 0 10px; border: 0; border-radius: 7px; background: transparent; color: var(--muted); font-size: 13px; }.tab-button:hover,.tab-button.active { background: var(--tonal); color: var(--accent); }.catalog-error { color: var(--danger); font-size: 13px; margin-bottom: 14px; }@media(max-width:600px) { .extensions-toolbar { flex-wrap: wrap; }.tab-button { padding: 0 8px; font-size: 12px; } }
</style>
