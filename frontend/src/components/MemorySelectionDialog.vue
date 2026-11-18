<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { Info } from '@lucide/vue'
import type { MemoryLibrary, Workspace } from '../api'
import { memoryScopeLabel } from '../memory'

const props = defineProps<{
  open: boolean
  libraries: MemoryLibrary[]
  enabledIds: string[]
  loading?: boolean
  saving?: boolean
  managing?: boolean
  workspaceId?: string
  workspaces?: Workspace[]
  error?: string
  loadFailed?: boolean
}>()
const emit = defineEmits<{
  confirm: [ids: string[]]; cancel: []; retry: []; create: []
  rename: [library: MemoryLibrary]; delete: [library: MemoryLibrary]; scope: [library: MemoryLibrary]
  info: [library: MemoryLibrary]
}>()
const dialog = ref<HTMLDialogElement | null>(null)
const selectedIDs = ref<string[]>([])
const menuID = ref('')
const menuPosition = ref({ left: '0px', top: '0px' })
const busy = computed(() => props.saving || props.managing)
let selectionReady = false

const displayName = (library: MemoryLibrary) => library.name === 'Main' ? '全局主记忆' : library.name

watch(() => [props.open, props.loading] as const, async ([open, loading], previous) => {
  if (open) {
    if (loading) selectionReady = false
    if (!previous?.[0] || (!loading && !selectionReady)) {
      selectedIDs.value = loading ? [] : props.enabledIds.filter((id) => props.libraries.some((library) => library.id === id))
      selectionReady = !loading
    }
    await nextTick()
    if (props.open && dialog.value && !dialog.value.open) dialog.value.showModal()
  } else {
    menuID.value = ''
    selectionReady = false
    if (dialog.value?.open) dialog.value.close()
  }
}, { immediate: true })

watch(() => props.libraries, (libraries) => {
  if (props.open && !props.loading) selectedIDs.value = selectedIDs.value.filter((id) => libraries.some((library) => library.id === id))
  menuID.value = ''
})
watch(busy, (value) => { if (value) menuID.value = '' })

function cancel() { if (!busy.value) emit('cancel') }
function confirm() {
  if (!props.loading && !busy.value && !props.loadFailed) emit('confirm', [...selectedIDs.value])
}
function toggleMenu(id: string, event: MouseEvent) {
  if (menuID.value === id) { menuID.value = ''; return }
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const width = 170, height = 132
  menuPosition.value = {
    left: `${Math.max(8, Math.min(bounds.right - width, window.innerWidth - width - 8))}px`,
    top: `${Math.max(8, bounds.bottom + height + 4 < window.innerHeight ? bounds.bottom + 4 : bounds.top - height - 4)}px`,
  }
  menuID.value = id
}
function closeMenuOnOutside(event: PointerEvent) {
  if (!(event.target as HTMLElement).closest('.item-actions')) menuID.value = ''
}
function onEscape(event: KeyboardEvent) {
  if (event.key === 'Escape' && menuID.value) {
    event.preventDefault()
    menuID.value = ''
  }
}
</script>

<template>
  <dialog ref="dialog" class="picker" aria-labelledby="memory-picker-title" @click.self="cancel" @cancel.prevent="cancel"
    @pointerdown="closeMenuOnOutside" @keydown.stop="onEscape">
    <form class="picker-body" @submit.prevent="confirm">
      <header class="picker-head">
        <div>
          <h2 id="memory-picker-title">记忆设置</h2>
          <p class="sub">勾选当前对话使用的记忆，不勾选即禁用</p>
        </div>
        <span class="count">{{ selectedIDs.length }} / {{ libraries.length }}</span>
      </header>
      <div class="picker-tools">
        <button type="button" class="btn btn-ghost btn-sm" :disabled="loading || busy || loadFailed" @click="selectedIDs = libraries.map((library) => library.id)">全选</button>
        <button type="button" class="btn btn-ghost btn-sm" :disabled="loading || busy || loadFailed" @click="selectedIDs = []">全部禁用</button>
        <button type="button" class="btn btn-ghost btn-sm create-memory" :disabled="loading || busy || loadFailed" @click="emit('create')">创建记忆</button>
      </div>
      <div class="picker-list" :aria-busy="loading">
        <p v-if="loading" class="empty">正在加载记忆…</p>
        <template v-else>
          <div v-for="library in libraries" :key="library.id" class="item">
            <label class="item-select">
              <input v-model="selectedIDs" type="checkbox" :value="library.id" :disabled="busy || loadFailed" />
              <span class="item-content">
                <span class="item-name">{{ displayName(library) }}</span>
                <span class="scope">{{ memoryScopeLabel(library, workspaces) }}</span>
                <span v-if="library.description && library.name !== 'Main'" class="item-description">{{ library.description }}</span>
              </span>
            </label>
            <div class="item-actions" :class="{ 'item-actions--open': menuID === library.id }">
              <button type="button" class="memory-row-button memory-info-button" :aria-label="`${displayName(library)}的信息`"
                title="记忆信息" aria-haspopup="dialog" :disabled="busy || loadFailed" @click="menuID = ''; emit('info', library)">
                <Info :size="15" :stroke-width="1.7" aria-hidden="true" />
              </button>
              <button type="button" class="memory-row-button memory-menu-button" :aria-label="`${displayName(library)}的选项`"
                aria-haspopup="menu" :aria-expanded="menuID === library.id" :disabled="busy || loadFailed"
                @click="toggleMenu(library.id, $event)"><span aria-hidden="true">⋯</span></button>
              <div v-if="menuID === library.id" class="memory-menu" role="menu" :style="menuPosition">
                <button type="button" role="menuitem" :disabled="library.name === 'Main'"
                  @click="menuID = ''; emit('rename', library)">重命名</button>
                <button type="button" class="delete-memory" role="menuitem" :disabled="library.name === 'Main'"
                  @click="menuID = ''; emit('delete', library)">删除</button>
                <button type="button" role="menuitem"
                  :disabled="library.name === 'Main' || (library.scope === 'global' && !workspaceId)"
                  :title="library.name === 'Main' ? '全局主记忆的名称和作用范围固定，无法删除' : (!workspaceId ? '当前对话未使用工作区' : '')"
                  @click="menuID = ''; emit('scope', library)">更改作用范围</button>
              </div>
            </div>
          </div>
          <p v-if="!libraries.length" class="empty">暂无可用记忆</p>
        </template>
      </div>
      <p class="note">全局记忆可在所有工作区使用，写入需要完全访问权限。</p>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <button v-if="loadFailed" type="button" class="btn btn-ghost" :disabled="busy" @click="emit('retry')">重新加载</button>
      <footer class="picker-foot">
        <button type="button" class="btn btn-ghost" :disabled="busy" @click="cancel">取消</button>
        <button type="submit" class="btn btn-primary" :disabled="loading || busy || loadFailed">{{ saving ? '正在保存…' : '保存设置' }}</button>
      </footer>
    </form>
  </dialog>
</template>

<style scoped>
.picker { width: min(540px, calc(100vw - 40px)); max-height: 82vh; padding: 0; border: 1px solid var(--outline); border-radius: 14px; background: var(--surface); color: var(--ink); box-shadow: 0 24px 70px rgb(22 29 50 / 26%); }
.picker::backdrop { background: rgb(22 29 50 / 42%); }
.picker-body { display: flex; flex-direction: column; max-height: 82vh; }
.picker-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 20px 22px 10px; }
.picker-head h2 { font-size: 16px; font-weight: 600; }
.sub { margin-top: 3px; color: var(--muted); font-size: 12.5px; }
.count { flex-shrink: 0; padding: 3px 9px; border-radius: 999px; background: var(--tonal); color: var(--accent); font-size: 11.5px; }
.picker-tools { display: flex; gap: 6px; padding: 0 22px 8px; }
.create-memory { margin-left: auto; }
.picker-list { display: flex; flex-direction: column; gap: 1px; min-height: 120px; padding: 4px 16px; overflow-y: auto; }
.item { display: flex; align-items: center; gap: 9px; padding: 10px 8px; border-radius: 8px; }
.item:hover { background: var(--tonal); }
.item-select { display: flex; align-items: center; gap: 9px; flex: 1; min-width: 0; cursor: pointer; }
.item input { flex-shrink: 0; width: 15px; height: 15px; accent-color: var(--accent); }
.item-content { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.item-name { font-size: 13px; overflow-wrap: anywhere; }
.item-description { color: var(--muted); font-size: 12px; overflow-wrap: anywhere; }
.scope { flex-shrink: 0; font-size: 11px; color: var(--muted); overflow-wrap: anywhere; }
.item-actions { display: flex; align-items: center; gap: 3px; flex-shrink: 0; }
.memory-row-button { display: grid; place-items: center; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 50%; background: transparent; color: var(--muted); cursor: pointer; opacity: 0; pointer-events: none; transition: opacity 120ms ease, background-color 120ms ease; }
.item:hover .memory-row-button, .item:focus-within .memory-row-button, .item-actions--open .memory-row-button { opacity: 1; pointer-events: auto; }
.memory-row-button:hover:enabled, .memory-row-button:focus-visible, .memory-row-button[aria-expanded="true"] { background: rgb(128 128 128 / 13%); }
.memory-row-button:disabled { cursor: default; }
.memory-menu-button { font-size: 21px; line-height: 1; }
.memory-menu { position: fixed; z-index: 2; width: 170px; padding: 5px; border: 1px solid var(--outline); border-radius: 9px; background: var(--surface); box-shadow: 0 8px 24px rgb(22 29 50 / 18%); }
.memory-menu button { display: block; width: 100%; padding: 9px 10px; border: 0; border-radius: 5px; background: transparent; color: var(--ink); text-align: left; font-size: 12.5px; cursor: pointer; }
.memory-menu button:hover:enabled { background: var(--tonal); }
.memory-menu .delete-memory { color: var(--danger); }
.memory-menu button:disabled { opacity: .4; cursor: default; }
.empty { padding: 16px 8px; color: var(--muted); font-size: 12.5px; text-align: center; }
.note, .error { padding: 0 22px; font-size: 12px; line-height: 1.6; }
.note { color: var(--muted); margin: 12px 0 0; }
.error { color: var(--danger, #b42318); margin-top: 8px; }
.picker-foot { display: flex; justify-content: flex-end; gap: 8px; padding: 12px 22px 18px; }
@media (hover: none) { .memory-row-button { opacity: 1; pointer-events: auto; } }
</style>
