<script setup lang="ts">
import { nextTick, ref, useId, watch } from 'vue'
import { X } from '@lucide/vue'
const props = defineProps<{ open: boolean; title: string; busy?: boolean; wide?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const titleId = useId()
function close() { if (!props.busy) emit('close') }
watch(() => props.open, async (open) => {
  await nextTick()
  if (props.open !== open) return
  if (open && dialog.value && !dialog.value.open) dialog.value.showModal()
  else if (!open && dialog.value?.open) dialog.value.close()
}, { immediate: true })
</script>
<template>
  <Teleport to="body">
    <dialog ref="dialog" class="extension-window" :class="{ wide }" :aria-labelledby="titleId" :aria-busy="busy"
      @cancel.prevent="close" @click.self="close" @keydown.stop>
      <header class="window-head"><h2 :id="titleId">{{ title }}</h2><button type="button" class="btn btn-ghost window-close" :disabled="busy" aria-label="关闭窗口" @click="close"><X :size="17" aria-hidden="true" /></button></header>
      <div class="window-content"><slot /></div>
      <footer v-if="$slots.footer" class="window-footer"><slot name="footer" /></footer>
    </dialog>
  </Teleport>
</template>
<style scoped>
.extension-window { width: min(540px, calc(100vw - 40px)); max-height: calc(100vh - 40px); padding: 0; border: 1px solid var(--outline); border-radius: 14px; background: var(--surface); color: var(--ink); box-shadow: 0 24px 70px rgb(22 29 50 / 26%); }
.extension-window.wide { width: min(820px, calc(100vw - 40px)); }
.extension-window::backdrop { background: rgb(22 29 50 / 42%); }
.window-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 17px 20px; border-bottom: 1px solid var(--outline); }
.window-head h2 { font-size: 16px; font-weight: 600; }.window-close { flex-shrink: 0; width: 30px; padding: 0; }
.window-content { padding: 20px; }.window-footer { display: flex; align-items: center; justify-content: flex-end; gap: 8px; padding: 16px 20px; border-top: 1px solid var(--outline); }
</style>
