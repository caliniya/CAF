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
  <div data-placeholder="ModelSelectionDialog"></div>
</template>
