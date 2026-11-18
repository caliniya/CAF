<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import { ChevronRight, Search } from '@lucide/vue'
import type { MemoryLibrary, MemoryLibraryDetails } from '../api'

const props = defineProps<{
  library: MemoryLibrary | null
  details: MemoryLibraryDetails | null
  loading: boolean
  error: string
}>()
const emit = defineEmits<{ close: []; retry: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const dataSection = ref<HTMLDetailsElement | null>(null)
const query = ref('')
const titleId = useId()
const metadata = computed(() => props.details ?? props.library)
const displayName = computed(() => metadata.value?.name === 'Main' ? '全局主记忆' : metadata.value?.name ?? '')
const entries = computed(() => Object.entries(props.details?.data ?? {}).sort(([a], [b]) => a.localeCompare(b)))
const matches = computed(() => {
  const keyword = query.value.trim().toLocaleLowerCase()
  return entries.value.filter(([, memory]) => !keyword || memory.data.toLocaleLowerCase().includes(keyword)
    || (memory.description ?? []).some(tag => tag.toLocaleLowerCase().includes(keyword)))
})

watch(() => props.library?.id, async (id) => {
  query.value = ''
  await nextTick()
  if (props.library?.id !== id) return
  if (id && dialog.value && !dialog.value.open) dialog.value.showModal()
  else if (!id && dialog.value?.open) dialog.value.close()
  if (dataSection.value) dataSection.value.open = true
}, { immediate: true })
watch(query, () => { if (dataSection.value) dataSection.value.open = true })
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="memory-details-dialog" :aria-labelledby="titleId" :aria-busy="loading"
      @click.self="emit('close')" @cancel.prevent="emit('close')" @keydown.stop>
      <div v-if="library" class="memory-details-body">
        <header class="memory-details-head">
          <div class="memory-title-row">
            <h2 :id="titleId">记忆信息</h2>
            <button type="button" class="btn btn-ghost btn-sm" @click="emit('close')">关闭</button>
          </div>
          <label class="memory-search">
            <Search :size="16" :stroke-width="1.7" aria-hidden="true" />
            <input v-model="query" type="search" aria-label="搜索记忆标签或内容" placeholder="搜索标签或内容" autofocus />
          </label>
        </header>
        <div class="memory-details-content">
          <dl class="memory-metadata">
            <div><dt>名称</dt><dd>{{ displayName }}</dd></div>
            <div><dt>作用域</dt><dd>{{ metadata?.scope === 'global' ? '全局记忆' : '工作区记忆' }}</dd></div>
            <div><dt>ID</dt><dd class="memory-id">{{ metadata?.id }}</dd></div>
            <div class="memory-description"><dt>介绍</dt><dd>{{ metadata?.description || '暂无介绍' }}</dd></div>
          </dl>
          <p v-if="loading" class="memory-empty" role="status">正在加载记忆…</p>
          <div v-else-if="error" class="memory-details-error" role="alert">
            <p>{{ error }}</p>
            <button type="button" class="btn btn-ghost btn-sm" @click="emit('retry')">重新加载</button>
          </div>
          <details v-else-if="details" ref="dataSection" class="memory-data" open>
            <summary class="memory-data-title">
              <ChevronRight class="fold-icon" :size="16" :stroke-width="1.8" aria-hidden="true" />
              <span>记忆数据</span>
              <span class="memory-count">{{ matches.length }} / {{ entries.length }}</span>
            </summary>
            <div class="memory-data-content">
              <details v-for="([id, memory], index) in matches" :key="id" class="memory-entry">
                <summary class="memory-entry-title">
                  <ChevronRight class="fold-icon" :size="14" :stroke-width="1.8" aria-hidden="true" />
                  <div class="memory-entry-preview">
                    <span class="memory-entry-number">记忆 {{ index + 1 }}</span>
                    <p class="memory-preview">{{ memory.data || '空内容' }}</p>
                    <div v-if="memory.description?.length" class="memory-tags">
                      <span v-for="(tag, tagIndex) in memory.description" :key="tagIndex" class="memory-tag">{{ tag }}</span>
                    </div>
                  </div>
                </summary>
                <div class="memory-entry-content">
                  <p class="memory-text">{{ memory.data || '空内容' }}</p>
                  <p class="memory-entry-id">{{ id }}</p>
                  <p v-if="memory.date" class="memory-entry-date">{{ memory.date }}</p>
                </div>
              </details>
              <p v-if="!matches.length" class="memory-empty">{{ entries.length ? '没有匹配的记忆' : '暂无记忆数据' }}</p>
            </div>
          </details>
        </div>
      </div>
    </dialog>
  </Teleport>
</template>

<style scoped>
.memory-details-dialog { width: min(680px, calc(100vw - 40px)); max-height: calc(100vh - 40px); padding: 0; border: 1px solid var(--outline); border-radius: 14px; background: var(--surface); color: var(--ink); box-shadow: 0 24px 70px rgb(22 29 50 / 26%); overflow: hidden; }
.memory-details-dialog::backdrop { background: rgb(22 29 50 / 42%); }
.memory-details-body { display: flex; flex-direction: column; max-height: calc(100vh - 42px); }
.memory-details-head { padding: 16px 20px; border-bottom: 1px solid var(--outline); }
.memory-title-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
h2 { font-size: 16px; font-weight: 600; }
.memory-search { display: flex; align-items: center; gap: 8px; margin-top: 14px; padding: 0 10px; height: 36px; border: 1px solid var(--outline); border-radius: 8px; color: var(--muted); }
.memory-search:focus-within { border-color: var(--accent); }
.memory-search input { width: 100%; min-width: 0; padding: 0; border: 0; outline: 0; background: transparent; color: var(--ink); font-size: 13px; }
.memory-details-content { min-height: 0; padding: 20px; overflow-y: auto; }
.memory-metadata { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin: 0; }
.memory-metadata > div { min-width: 0; }
.memory-metadata dt { margin-bottom: 5px; color: var(--muted); font-size: 12px; }
.memory-metadata dd { margin: 0; font-size: 13px; white-space: pre-wrap; overflow-wrap: anywhere; }
.memory-metadata > div:has(.memory-id), .memory-description { grid-column: 1 / -1; }
.memory-id, .memory-entry-id { font-family: var(--font-mono); }
.memory-data { margin-top: 22px; }
summary { list-style: none; cursor: pointer; }
summary::-webkit-details-marker { display: none; }
.fold-icon { flex-shrink: 0; color: var(--muted); transition: transform 120ms ease; }
details[open] > summary > .fold-icon { transform: rotate(90deg); }
.memory-data-title { display: flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 500; }
.memory-count { margin-left: auto; color: var(--muted); font-size: 12px; font-weight: 400; }
.memory-data-content { display: flex; flex-direction: column; gap: 8px; margin-top: 10px; }
.memory-entry { border: 1px solid var(--outline); border-radius: 9px; overflow: hidden; }
.memory-entry-title { display: flex; align-items: flex-start; gap: 8px; padding: 12px; }
.memory-entry-title > .fold-icon { margin-top: 2px; }
.memory-entry-preview { min-width: 0; flex: 1; }
.memory-entry-number { color: var(--muted); font-size: 11px; }
.memory-preview { display: -webkit-box; margin-top: 4px; overflow: hidden; -webkit-box-orient: vertical; -webkit-line-clamp: 2; font-size: 13px; line-height: 1.6; white-space: pre-wrap; overflow-wrap: anywhere; }
.memory-tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 7px; }
.memory-tag { max-width: 100%; padding: 2px 7px; border-radius: 999px; background: var(--tonal); color: var(--muted); font-size: 11px; overflow-wrap: anywhere; }
.memory-entry-content { padding: 0 12px 12px 34px; }
.memory-text { font-size: 13px; line-height: 1.75; white-space: pre-wrap; overflow-wrap: anywhere; }
.memory-entry-id { margin-top: 10px; color: var(--muted); font-size: 10.5px; overflow-wrap: anywhere; }
.memory-entry-date { margin-top: 4px; color: var(--muted); font-size: 11px; }
.memory-empty { padding: 20px 0; color: var(--muted); text-align: center; font-size: 12.5px; }
.memory-details-error { margin-top: 20px; color: var(--danger); font-size: 13px; overflow-wrap: anywhere; }
.memory-details-error button { margin-top: 8px; }
@media (max-width: 480px) { .memory-metadata { grid-template-columns: 1fr; } }
</style>
