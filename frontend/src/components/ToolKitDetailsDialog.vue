<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import { ChevronRight, X } from '@lucide/vue'
import type { ToolKitDescriptor } from '../api'
import ToolParameterDetails from './ToolParameterDetails.vue'

const props = defineProps<{ id: string; kit: ToolKitDescriptor | null }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const titleId = useId()

const tools = computed(() => props.kit?.tools.map((tool) => ({
  ...tool,
  parameterEntries: Object.entries(tool.parameters?.properties ?? {}).map(([name, schema]) => ({
    name, schema, required: tool.parameters?.required?.includes(name) ?? false,
  })),
})) ?? [])

watch(
  () => props.kit,
  async (kit) => {
    await nextTick()
    if (props.kit !== kit) return
    if (kit && dialog.value && !dialog.value.open) {
      dialog.value.showModal()
    } else if (!kit && dialog.value?.open) {
      dialog.value.close()
    }
  },
  { immediate: true },
)
</script>

<template>
  <Teleport to="body">
    <dialog :id="id" ref="dialog" class="toolkit-details" :aria-labelledby="titleId"
      @click.self="emit('close')" @cancel.prevent="emit('close')" @close="emit('close')" @keydown.stop>
      <div v-if="kit" class="toolkit-dialog-body">
        <header class="details-head">
          <div>
            <p class="details-eyebrow">模块详情 · {{ tools.length }} 个工具</p>
            <h2 :id="titleId">{{ kit.name }}</h2>
          </div>
          <button type="button" class="details-close" aria-label="关闭模块详情" autofocus @click="emit('close')">
            <X :size="16" :stroke-width="1.8" />
          </button>
        </header>
        <div class="details-body">
          <p v-if="kit.description" class="kit-description">{{ kit.description }}</p>
          <p v-if="!tools.length" class="details-empty">此模块暂无工具</p>
          <details v-for="tool in tools" :key="tool.id" class="tool-detail" open>
            <summary class="tool-summary">
              <ChevronRight class="fold-chevron" :size="13" :stroke-width="2" aria-hidden="true" />
              <span class="tool-title">{{ tool.name || tool.id }}</span>
              <code class="tool-id">{{ tool.id }}</code>
            </summary>
            <div class="tool-detail-body">
              <p class="tool-description">{{ tool.description || '暂无工具说明' }}</p>
              <details class="parameter-group">
                <summary class="parameter-group-summary">
                  <ChevronRight class="fold-chevron" :size="12" :stroke-width="2" aria-hidden="true" />
                  <span>参数</span>
                  <span class="parameter-count">{{ tool.parameterEntries.length }} 项</span>
                </summary>
                <div class="parameter-list">
                  <ToolParameterDetails v-for="parameter in tool.parameterEntries" :key="parameter.name"
                    :name="parameter.name" :schema="parameter.schema" :required="parameter.required" />
                  <p v-if="!tool.parameterEntries.length" class="details-empty">
                    {{ tool.parameters ? '此工具无需参数' : '暂无参数定义' }}
                  </p>
                </div>
              </details>
            </div>
          </details>
        </div>
      </div>
    </dialog>
  </Teleport>
</template>

<style scoped>
.toolkit-details {
  width: min(640px, calc(100vw - 40px));
  max-height: calc(100vh - 40px);
  padding: 0;
  border: 1px solid var(--outline);
  border-radius: 12px;
  background: var(--surface);
  color: var(--ink);
  box-shadow: 0 24px 70px rgb(22 29 50 / 26%);
  overflow: hidden;
}

.toolkit-details::backdrop {
  background: rgb(22 29 50 / 24%);
}

.toolkit-dialog-body {
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 42px);
}

.details-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  flex-shrink: 0;
  padding: 14px 16px;
  border-bottom: 1px solid var(--outline);
}

.details-eyebrow {
  margin-bottom: 3px;
  color: var(--muted);
  font-size: 11px;
}

.details-head h2 {
  font-size: 15px;
  font-weight: 600;
}

.details-close {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--muted);
  cursor: pointer;
}

.details-close:hover {
  background: var(--tonal);
  color: var(--accent);
}

.details-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 0;
  padding: 12px;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.kit-description,
.tool-description {
  color: var(--muted);
  font-size: 12px;
  line-height: 1.65;
  white-space: pre-wrap;
}

.kit-description {
  padding: 0 4px;
}

.tool-detail {
  flex-shrink: 0;
  border: 1px solid var(--outline);
  border-radius: 9px;
}

.tool-summary,
.parameter-group-summary {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 7px;
  padding: 10px;
  list-style: none;
  cursor: pointer;
}

.tool-summary::-webkit-details-marker,
.parameter-group-summary::-webkit-details-marker {
  display: none;
}

.tool-summary:hover,
.parameter-group-summary:hover {
  border-radius: 8px;
  background: var(--tonal);
}

.fold-chevron {
  flex-shrink: 0;
  color: var(--muted);
  transition: transform 150ms ease;
}

.tool-detail[open] > .tool-summary .fold-chevron,
.parameter-group[open] > .parameter-group-summary .fold-chevron {
  transform: rotate(90deg);
}

.tool-title {
  font-size: 13px;
  font-weight: 600;
}

.tool-id {
  margin-left: auto;
  color: var(--muted);
  font: 10.5px var(--font-mono);
}

.tool-detail-body {
  padding: 0 10px 10px;
}

.parameter-group {
  margin-top: 10px;
  border-radius: 8px;
  background: var(--page);
}

.parameter-group-summary {
  padding: 8px;
  color: var(--ink);
  font-size: 12px;
}

.parameter-count {
  margin-left: auto;
  color: var(--muted);
  font-size: 11px;
}

.parameter-list {
  display: grid;
  gap: 6px;
  padding: 0 7px 7px;
}

.details-empty {
  padding: 8px 4px;
  color: var(--muted);
  font-size: 12px;
}
</style>
