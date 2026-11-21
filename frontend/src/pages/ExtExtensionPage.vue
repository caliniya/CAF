<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Pencil } from '@lucide/vue'
import type { ExtensionOption } from '../api'
import { useExtensionClientPage } from '../composables/useExtensionClientPage'
import ExtensionEditorDialog from '../components/ExtensionEditorDialog.vue'

const route = useRoute()
const router = useRouter()
const clientId = computed(() => String(route.params.clientId ?? ''))
const id = computed(() => String(route.params.extensionId ?? ''))
const { ext, client, loading, error, load } = useExtensionClientPage(clientId)
const option = computed(() => ext.options.find((value) => value.clientId === clientId.value && value.id === id.value))
const descriptor = computed(() => ext.types.find((value) => value.type === option.value?.type))
const busy = computed(() => ext.pending.includes('extension:' + id.value))
const editing = ref<ExtensionOption>()

watch([clientId, id], () => { editing.value = undefined })
function edit() {
  if (option.value && !loading.value && !busy.value) editing.value = { ...option.value, values: { ...option.value.values } }
}
function back() { void router.push({ name: 'ext-client', params: { clientId: clientId.value } }) }
function deleted(extensionId: string) {
  if (extensionId === id.value) void router.replace({ name: 'ext-client', params: { clientId: clientId.value } })
}
</script>

<template>
  <section class="extension-page" aria-labelledby="extension-title">
    <header class="extension-page-header">
      <button type="button" class="btn btn-ghost extension-page-back" aria-label="返回客户端" @click="back"><ArrowLeft :size="17" aria-hidden="true" /></button>
      <div class="extension-page-heading"><h1 id="extension-title">{{ option?.name ?? '扩展' }}</h1><p>{{ client?.name ?? clientId }}<span v-if="option"> · {{ descriptor?.name ?? option.type }}</span></p></div>
      <button type="button" class="btn" :disabled="loading || !option || !!error || !!ext.optionsError || busy" @click="edit"><Pencil :size="15" aria-hidden="true" />编辑扩展</button>
    </header>
    <div class="extension-page-content">
      <div v-if="error || ext.optionsError" class="extension-page-error" role="alert">{{ error || ext.optionsError }}<button type="button" class="btn btn-ghost btn-sm" :disabled="loading" @click="load">重试</button></div>
      <p v-if="loading || (!ext.optionsLoaded && !ext.optionsError)" class="extension-page-hint" role="status">正在加载扩展…</p>
      <div v-else-if="option && !error" class="extension-page-body">
        <p v-if="descriptor?.description" class="extension-description">{{ descriptor.description }}</p>
        <dl class="extension-information">
          <div><dt>ID</dt><dd>{{ option.id }}</dd></div>
          <div><dt>扩展类型</dt><dd>{{ descriptor?.name ?? option.type }}</dd></div>
          <div><dt>所属客户端</dt><dd>{{ client?.name ?? option.clientName ?? clientId }}</dd></div>
          <div><dt>启用状态</dt><dd><span class="extension-enabled" :class="{ on: option.enabled }">{{ option.enabled ? '已启用' : '已停用' }}</span></dd></div>
          <div v-for="field in descriptor?.information" :key="field.key"><dt>{{ field.label }}</dt><dd>{{ option.values[field.key] || '—' }}</dd></div>
        </dl>
        <p v-if="option.lastError" class="extension-page-error" role="alert">{{ option.lastError }}</p>
      </div>
      <p v-else-if="!error && !ext.optionsError" class="extension-page-hint">找不到此扩展，或它不属于当前客户端。</p>
    </div>
    <ExtensionEditorDialog v-if="editing" :open="true" :option="editing" :client-id="clientId" @close="editing = undefined" @deleted="deleted" />
  </section>
</template>

<style src="../styles/ExtensionPages.css"></style>
<style scoped>
.extension-description { margin-bottom: 20px; color: var(--muted); font-size: 13px; line-height: 1.75; }
.extension-information { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 24px 32px; margin: 0 0 20px; padding: 24px; border: 1px solid var(--outline); border-radius: 12px; background: var(--surface); }
.extension-information div { display: grid; align-content: start; gap: 7px; min-width: 0; }
.extension-information dt { color: var(--muted); font-size: 12px; }
.extension-information dd { margin: 0; font-size: 13px; white-space: pre-wrap; overflow-wrap: anywhere; user-select: text; }
.extension-enabled { display: inline-flex; padding: 2px 8px; border-radius: 5px; background: var(--page); color: var(--muted); font-size: 12px; }
.extension-enabled.on { background: var(--success-soft); color: var(--success); }
@media(max-width: 640px) { .extension-information { grid-template-columns: minmax(0, 1fr); gap: 20px; padding: 20px; } }
</style>
