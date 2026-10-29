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

function confirmSelection() {
  const selected = availableModels.value.filter((model) => selectedIDs.value.includes(model.id))
  emit('confirm', selected, availableModels.value)
}
</script>

<template>
  <dialog ref="dialog" class="model-picker" @click.self="emit('cancel')" @cancel.prevent="emit('cancel')">
    <form class="stack model-picker-content" @submit.prevent="confirmSelection">
      <header class="model-picker-header">
        <div>
          <h2>选择模型</h2>
          <p class="muted">仅勾选的模型会保存</p>
        </div>
        <span class="badge">已选 {{ selectedIDs.length }} 项</span>
      </header>

      <div class="row model-picker-tools">
        <button type="button" class="btn-secondary btn-small" @click="selectedIDs = availableModels.map((model) => model.id)">
          全选
        </button>
        <button type="button" class="btn-secondary btn-small" @click="selectedIDs = []">
          清空
        </button>
      </div>

      <div class="model-picker-list">
        <label v-for="model in availableModels" :key="model.id" class="check-line model-picker-item">
          <input v-model="selectedIDs" type="checkbox" :value="model.id" />
          <span>{{ model.name || model.id }}</span>
          <span class="muted mono">{{ model.id }}</span>
        </label>
        <p v-if="!availableModels.length" class="muted">未探测到模型，可手动添加模型 ID。</p>
      </div>

      <div class="row model-picker-manual">
        <input
          v-model="manualModelID"
          type="text"
          placeholder="手动添加模型 ID"
          @keydown.enter.prevent="addManualModel"
        />
        <button type="button" class="btn-secondary" :disabled="!manualModelID.trim()" @click="addManualModel">
          添加
        </button>
      </div>

      <footer class="row model-picker-actions">
        <button type="button" class="btn-secondary" @click="emit('cancel')">取消</button>
        <button type="submit" class="btn" :disabled="selectedIDs.length === 0">
          {{ confirmLabel || '完成选择' }}
        </button>
      </footer>
    </form>
  </dialog>
</template>