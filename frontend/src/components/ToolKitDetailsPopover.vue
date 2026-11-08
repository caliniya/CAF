<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, useId, watch } from 'vue'
import { ChevronRight, X } from '@lucide/vue'
import type { ToolKitDescriptor } from '../api'
import ToolParameterDetails from './ToolParameterDetails.vue'

const props = defineProps<{ id: string; kit: ToolKitDescriptor; anchor: HTMLElement }>()
const emit = defineEmits<{ close: []; keepOpen: []; leave: [] }>()
const panel = ref<HTMLElement | null>(null)
const titleId = useId()
const position = ref({ left: '12px', top: '12px', maxHeight: 'calc(100vh - 24px)' })
const ready = ref(false)
let resizeObserver: ResizeObserver | undefined
let unmounted = false

const tools = computed(() => props.kit.tools.map((tool) => ({
  ...tool,
  parameterEntries: Object.entries(tool.parameters?.properties ?? {}).map(([name, schema]) => ({
    name, schema, required: tool.parameters?.required?.includes(name) ?? false,
  })),
})))

function updatePosition() {
  const element = panel.value
  if (unmounted || !element) return
  if (!props.anchor.isConnected) {
    emit('close')
    return
  }
  const bounds = props.anchor.getBoundingClientRect()
  const width = element.offsetWidth
  const height = element.offsetHeight
  const margin = 12
  const gap = 10
  const rightSpace = window.innerWidth - bounds.right - margin - gap
  const leftSpace = bounds.left - margin - gap
  let maxHeight = Math.min(640, window.innerHeight - margin * 2)
  let preferredLeft = bounds.right + gap
  let preferredTop = bounds.top
  if (rightSpace >= width || leftSpace >= width) {
    if (rightSpace < width) preferredLeft = bounds.left - width - gap
  } else {
    // 窄窗口改为向上或向下展开，避免覆盖模块开关。
    const aboveSpace = bounds.top - margin - gap
    const belowSpace = window.innerHeight - bounds.bottom - margin - gap
    maxHeight = Math.max(100, Math.min(maxHeight, Math.max(aboveSpace, belowSpace)))
    preferredLeft = bounds.left
    preferredTop = belowSpace >= aboveSpace
      ? bounds.bottom + gap
      : bounds.top - gap - Math.min(height, maxHeight)
  }
  const left = Math.max(margin, Math.min(preferredLeft, window.innerWidth - width - margin))
  const top = Math.max(margin, Math.min(preferredTop, window.innerHeight - Math.min(height, maxHeight) - margin))
  position.value = { left: left + 'px', top: top + 'px', maxHeight: maxHeight + 'px' }
  ready.value = true
}

watch(() => [props.anchor, props.kit], async () => {
  await nextTick()
  updatePosition()
}, { immediate: true, flush: 'post' })

onMounted(() => {
  window.addEventListener('resize', updatePosition)
  document.addEventListener('scroll', updatePosition, true)
  resizeObserver = new ResizeObserver(updatePosition)
  if (panel.value) resizeObserver.observe(panel.value)
  updatePosition()
})

onUnmounted(() => {
  unmounted = true
  window.removeEventListener('resize', updatePosition)
  document.removeEventListener('scroll', updatePosition, true)
  resizeObserver?.disconnect()
})
</script>

<template>
  <Teleport to="body">
    <section :id="id" ref="panel" class="toolkit-details" :class="{ ready }" :style="position"
      role="dialog" :aria-labelledby="titleId"
      @mouseenter="emit('keepOpen')" @mouseleave="emit('leave')"
      @focusin="emit('keepOpen')" @focusout="emit('leave')"
      @keydown.esc.stop.prevent="emit('close')">
      <header class="details-head">
        <div>
          <p class="details-eyebrow">模块详情 · {{ tools.length }} 个工具</p>
          <h2 :id="titleId">{{ kit.name }}</h2>
        </div>
        <button type="button" class="details-close" aria-label="关闭模块详情" @click="emit('close')">
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
    </section>
  </Teleport>
</template>

<style scoped>
.toolkit-details {
  position: fixed;
  z-index: 210;
  display: flex;
  flex-direction: column;
  width: min(420px, calc(100vw - 24px));
  max-height: min(640px, calc(100vh - 24px));
  border: 1px solid var(--outline);
  border-radius: 12px;
  background: var(--surface);
  color: var(--ink);
  box-shadow: 0 16px 48px rgb(22 29 50 / 24%);
  visibility: hidden;
}

.toolkit-details.ready {
  visibility: visible;
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
