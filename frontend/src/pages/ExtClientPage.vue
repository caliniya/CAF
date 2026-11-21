<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Pencil, Plus } from '@lucide/vue'
import type { ExtensionClient } from '../api'
import { useExtensionClientPage } from '../composables/useExtensionClientPage'
import ExtensionOptionsList from '../components/ExtensionOptionsList.vue'
import ExtensionClientEditorDialog from '../components/ExtensionClientEditorDialog.vue'
import ExtensionEditorDialog from '../components/ExtensionEditorDialog.vue'

const route = useRoute()
const router = useRouter()
const id = computed(() => String(route.params.clientId ?? ''))
const { ext, client, loading, error, load } = useExtensionClientPage(id)
const editingClient = ref<ExtensionClient>()
const createOpen = ref(false)
const busy = computed(() => ext.pending.includes(id.value))

watch(id, () => { editingClient.value = undefined; createOpen.value = false })
function edit() {
  if (client.value && !loading.value && !busy.value) editingClient.value = { ...client.value }
}
function deleted(clientId: string) {
  if (clientId === id.value) void router.replace({ name: 'ext-clients' })
}
</script>

<template>
  <section class="extension-page" aria-labelledby="client-title">
    <header class="extension-page-header">
      <button type="button" class="btn btn-ghost extension-page-back" aria-label="返回客户端列表" @click="router.push({ name: 'ext-clients' })"><ArrowLeft :size="17" aria-hidden="true" /></button>
      <div class="extension-page-heading"><h1 id="client-title">{{ client?.name ?? '客户端' }}</h1><p>ID：{{ id }}</p></div>
      <button type="button" class="btn" :disabled="loading || !client || !!error || busy" @click="edit"><Pencil :size="15" aria-hidden="true" />编辑客户端</button>
    </header>
    <div class="extension-page-content">
      <div v-if="error" class="extension-page-error" role="alert">{{ error }}<button type="button" class="btn btn-ghost btn-sm" :disabled="loading" @click="load">重试</button></div>
      <p v-if="loading" class="extension-page-hint" role="status">正在加载客户端…</p>
      <div v-else-if="client && !error" class="extension-page-body">
        <div class="extension-page-toolbar"><h2>已创建扩展</h2><button type="button" class="btn btn-primary" :disabled="busy" @click="createOpen = true"><Plus :size="15" aria-hidden="true" />创建扩展</button></div>
        <ExtensionOptionsList :key="id" :client-id="id" />
      </div>
      <p v-else-if="!error" class="extension-page-hint">找不到此客户端，请返回客户端列表。</p>
    </div>
    <ExtensionClientEditorDialog v-if="editingClient" :open="true" :client="editingClient" @close="editingClient = undefined" @deleted="deleted" />
    <ExtensionEditorDialog :key="id" :open="createOpen" :client-id="id" @close="createOpen = false" />
  </section>
</template>

<style src="../styles/ExtensionPages.css"></style>
