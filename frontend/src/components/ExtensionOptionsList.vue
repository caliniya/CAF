<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { ChevronRight, Puzzle } from '@lucide/vue'
import type { ExtensionOption } from '../api'
import { useExtensionStore } from '../stores/extensions'
const props = defineProps<{ clientId: string }>()
const ext = useExtensionStore()
const error = ref('')
const options = computed(() => ext.options.filter((option) => option.clientId === props.clientId))
function descriptor(option: ExtensionOption) { return ext.types.find((type) => type.type === option.type) }
async function toggle(option: ExtensionOption, event: Event) {
  if (!props.clientId || ext.pending.includes('extension:' + option.id)) return
  const current = option.clientId === props.clientId && option.enabled
  const input = event.target as HTMLInputElement
  input.checked = current
  error.value = ''
  try { await ext.enableOption(props.clientId, option.id, !current) }
  catch (err) { error.value = String(err) }
}
</script>
<template>
  <p v-if="ext.optionsError || error" class="list-error" role="alert">{{ error || ext.optionsError }}<button type="button" class="btn btn-ghost btn-sm" @click="ext.refreshOptions">重试</button></p>
  <p v-if="!ext.optionsLoaded" class="empty" role="status">正在加载扩展…</p>
  <p v-else-if="!options.length" class="empty">此客户端还没有扩展，点击「创建扩展」创建。</p>
  <ul v-else class="options-list" aria-label="扩展列表">
    <li v-for="option in options" :key="option.id" class="option-row" :aria-busy="ext.pending.includes('extension:' + option.id)">
      <RouterLink :to="{ name: 'ext-extension', params: { clientId, extensionId: option.id } }" class="option-link" :aria-label="'查看扩展 ' + option.name">
        <Puzzle :size="19" class="option-icon" aria-hidden="true" />
        <span class="option-identity"><strong>{{ option.name }}</strong><span>{{ descriptor(option)?.name ?? option.type }}</span></span>
        <span v-for="field in descriptor(option)?.information.filter((value) => value.inList)" :key="field.key" class="inline-information">{{ option.values[field.key] || '—' }}</span>
        <ChevronRight :size="16" class="option-chevron" aria-hidden="true" />
      </RouterLink>
      <input v-if="clientId" type="checkbox" class="switch" :checked="option.clientId === clientId && option.enabled" :disabled="ext.pending.includes('extension:' + option.id)" :aria-label="'启用扩展 ' + option.name" @change="toggle(option, $event)" />
    </li>
  </ul>
</template>
<style scoped>
.options-list { padding: 0; margin: 0; list-style: none; border: 1px solid var(--outline); border-radius: 10px; background: var(--surface); overflow: hidden; }
.option-row { display: flex; align-items: center; padding-right: 16px; }
.option-row + .option-row { border-top: 1px solid var(--outline); }
.option-row:hover { background: var(--page); }
.option-link { display: flex; align-items: center; flex: 1; min-width: 0; gap: 12px; padding: 18px 16px; color: var(--ink); text-decoration: none; }
.option-link:focus-visible { outline-offset: -2px; }
.option-icon { flex-shrink: 0; color: var(--accent); }
.option-identity { flex: 1; min-width: 0; display: grid; gap: 3px; }
.option-identity strong { font-size: 14px; font-weight: 500; overflow-wrap: anywhere; }
.option-identity > span { color: var(--muted); font-size: 11px; }
.inline-information { font-size: 12px; color: var(--muted); }
.option-chevron { flex-shrink: 0; color: var(--muted); }
.empty { padding: 30px 12px; color: var(--muted); font-size: 13px; text-align: center; }.list-error { color: var(--danger); font-size: 13px; margin-bottom: 12px; }
@media(max-width: 600px) { .option-link { flex-wrap: wrap; }.option-identity { flex-basis: calc(100% - 64px); }.inline-information { margin-left: 31px; }.option-chevron { margin-left: auto; } }
</style>
