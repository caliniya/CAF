<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import type { Model } from '../api'

const props = defineProps<{
  open: boolean
  models: Model[]
  confirmLabel?: string
}>()

const emit = defineEmits<{
  confirm: [selected: Model[], available: Model[]]
  cancel: []
}>()

const dialog = ref<HTMLDialogElement | null>(null)
const availableModels = ref<Model[]>([])
const selectedIDs = ref<string[]>([])
const manualModelID = ref('')

watch(
  () => props.open,
  async (open) => {
    if (open) {
      availableModels.value = props.models.map((model) => ({ ...model }))
      selectedIDs.value = availableModels.value.map((model) => model.id)
      manualModelID.value = ''
      await nextTick()
      if (dialog.value && !dialog.value.open) dialog.value.showModal()
    } else if (dialog.value?.open) {
      dialog.value.close()
    }
  },
)

function addManualModel() {
  const id = manualModelID.value.trim()
  if (!id) return
  if (!availableModels.value.some((model) => model.id === id)) {
    availableModels.value.push({ id, name: id })
  }
  if (!selectedIDs.value.includes(id)) selectedIDs.value.push(id)
  manualModelID.value = ''
}

function selectAll() {
  selectedIDs.value = availableModels.value.map((model) => model.id)
}

function confirmSelection() {
  const selected = availableModels.value.filter((model) => selectedIDs.value.includes(model.id))
  emit('confirm', selected, availableModels.value)
}
</script>

<template>
  <dialog ref="dialog" class="picker" @click.self="emit('cancel')" @cancel.prevent="emit('cancel')">
    <form class="picker-body" @submit.prevent="confirmSelection">
      <header class="picker-head">
        <div>
          <h2>选择模型</h2>
          <p class="sub">勾选的模型会保存到这个供应商下</p>
        </div>
        <span class="count">{{ selectedIDs.length }} / {{ availableModels.length }}</span>
      </header>

      <div class="picker-tools">
        <button type="button" class="btn btn-ghost btn-sm" @click="selectAll">全选</button>
        <button type="button" class="btn btn-ghost btn-sm" @click="selectedIDs = []">清空</button>
      </div>

      <div class="picker-list">
        <label v-for="model in availableModels" :key="model.id" class="item">
          <input v-model="selectedIDs" type="checkbox" :value="model.id" />
          <span class="item-name">{{ model.name || model.id }}</span>
          <span v-if="model.name && model.name !== model.id" class="item-id">{{ model.id }}</span>
        </label>

        <p v-if="!availableModels.length" class="empty">
          没有探测到模型，可以在下面手动输入 ID
        </p>
      </div>

      <div class="picker-manual">
        <input
          v-model="manualModelID"
          type="text"
          placeholder="手动输入模型 ID"
          @keydown.enter.prevent="addManualModel"
        />
        <button type="button" class="btn" :disabled="!manualModelID.trim()" @click="addManualModel">
          添加
        </button>
      </div>

      <footer class="picker-foot">
        <button type="button" class="btn btn-ghost" @click="emit('cancel')">取消</button>
        <button type="submit" class="btn btn-primary" :disabled="!selectedIDs.length">
          {{ confirmLabel || '完成选择' }}
        </button>
      </footer>
    </form>
  </dialog>
</template>

<style scoped>
.picker {
  width: min(540px, calc(100vw - 40px));
  max-height: 82vh;
  padding: 0;
  border: 1px solid var(--outline);
  border-radius: 14px;
  background: var(--surface);
  color: var(--ink);
  box-shadow: 0 24px 70px rgb(22 29 50 / 26%);
}

.picker::backdrop {
  background: rgb(22 29 50 / 42%);
}

.picker-body {
  display: flex;
  flex-direction: column;
  max-height: 82vh;
}

.picker-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 20px 22px 10px;
}

.picker-head h2 {
  font-size: 16px;
  font-weight: 600;
}

.sub {
  margin-top: 3px;
  color: var(--muted);
  font-size: 12.5px;
}

.count {
  flex-shrink: 0;
  padding: 3px 9px;
  border-radius: 999px;
  background: var(--tonal);
  color: var(--accent);
  font-size: 11.5px;
}

.picker-tools {
  display: flex;
  gap: 6px;
  padding: 0 22px 8px;
}

.picker-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
  flex: 1;
  min-height: 120px;
  padding: 4px 16px;
  overflow-y: auto;
}

.item {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 8px 8px;
  border-radius: 8px;
  cursor: pointer;
}

.item:hover {
  background: var(--tonal);
}

.item input {
  flex-shrink: 0;
  width: 15px;
  height: 15px;
  accent-color: var(--accent);
  cursor: pointer;
}

.item-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.item-id {
  flex-shrink: 0;
  max-width: 45%;
  overflow: hidden;
  color: var(--muted);
  font-family: var(--font-mono);
  font-size: 11.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty {
  padding: 16px 8px;
  color: var(--muted);
  font-size: 12.5px;
  text-align: center;
}

.picker-manual {
  display: flex;
  gap: 8px;
  padding: 10px 22px;
  border-top: 1px solid var(--outline);
}

.picker-manual input {
  flex: 1;
  min-width: 0;
  height: 34px;
  padding: 0 10px;
  border: 1px solid var(--outline);
  border-radius: 8px;
  background: var(--surface);
  color: var(--ink);
  font-size: 13px;
}

.picker-manual input:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--tonal);
}

.picker-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 22px 18px;
}
</style>
