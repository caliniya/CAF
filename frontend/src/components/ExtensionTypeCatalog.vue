<script setup lang="ts">
import { ChevronRight, Puzzle } from '@lucide/vue'
import type { ExtensionDescriptor } from '../api'
defineProps<{ types: ExtensionDescriptor[]; selectable?: boolean; busy?: boolean }>()
const emit = defineEmits<{ select: [type: string] }>()
</script>

<template>
  <div class="type-catalog" :class="{ selectable }">
    <p class="catalog-intro">{{ selectable ? '选择扩展类型，下一步填写创建参数。' : '查看可用于此客户端的扩展及其参数说明。' }}</p>
    <p v-if="!types.length" class="catalog-empty">暂无可用扩展。</p>
    <template v-for="type in types" :key="type.type">
      <button v-if="selectable" type="button" class="type-choice" :disabled="busy" @click="emit('select', type.type)">
        <span class="type-icon"><Puzzle :size="20" aria-hidden="true" /></span>
        <span class="choice-body"><strong>{{ type.name }}</strong><span>{{ type.description || '暂无扩展介绍' }}</span></span>
        <ChevronRight :size="16" class="choice-chevron" aria-hidden="true" />
      </button>
      <details v-else class="type-detail" open>
        <summary class="type-summary"><ChevronRight :size="13" class="fold-chevron" aria-hidden="true" /><Puzzle :size="15" aria-hidden="true" /><span class="type-title">{{ type.name }}</span><code>{{ type.type }}</code></summary>
        <div class="type-detail-body">
          <p class="type-description">{{ type.description || '暂无扩展介绍' }}</p>
          <details class="parameter-group">
            <summary><ChevronRight :size="12" class="fold-chevron" aria-hidden="true" />创建参数<span>{{ type.parameters.length }} 项</span></summary>
            <dl class="catalog-fields"><div v-for="field in type.parameters" :key="field.key"><dt>{{ field.label }}<span>{{ field.required ? '必填' : '可选' }}</span></dt><dd>{{ field.help || field.placeholder || (field.kind === 'secret' ? '连接凭据' : field.kind === 'integer' ? '整数' : '文本') }}<span v-if="field.minimum || field.maximum">（{{ field.minimum }}–{{ field.maximum }}）</span></dd></div></dl>
          </details>
          <details v-if="type.information.length" class="parameter-group"><summary><ChevronRight :size="12" class="fold-chevron" aria-hidden="true" />运行信息<span>{{ type.information.length }} 项</span></summary><dl class="catalog-fields"><div v-for="field in type.information" :key="field.key"><dt>{{ field.label }}</dt><dd>{{ field.inList ? '在扩展列表和信息窗口中显示' : '在信息窗口中显示' }}</dd></div></dl></details>
        </div>
      </details>
    </template>
  </div>
</template>

<style scoped>
.type-catalog { display: grid; gap: 10px; }.catalog-intro,.catalog-empty { color: var(--muted); font-size: 12px; line-height: 1.65; }.catalog-intro { margin-bottom: 4px; }.type-detail { border: 1px solid var(--outline); border-radius: 9px; background: var(--surface); overflow: hidden; }.type-summary { display: flex; align-items: center; gap: 8px; padding: 12px; cursor: pointer; list-style: none; }.type-summary::-webkit-details-marker,.parameter-group summary::-webkit-details-marker { display: none; }.type-title { font-size: 13px; font-weight: 500; }.type-summary code { margin-left: auto; padding: 1px 5px; border-radius: 4px; background: var(--page); color: var(--muted); font-size: 10px; }.fold-chevron { flex-shrink: 0; color: var(--muted); transition: transform 120ms ease; }details[open] > summary > .fold-chevron { transform: rotate(90deg); }.type-detail-body { display: grid; gap: 10px; padding: 0 12px 12px 33px; }.type-description { color: var(--muted); font-size: 12px; line-height: 1.65; white-space: pre-wrap; }.parameter-group { border: 1px solid var(--outline); border-radius: 7px; }.parameter-group > summary { display: flex; align-items: center; gap: 6px; padding: 8px 10px; cursor: pointer; list-style: none; font-size: 12px; }.parameter-group > summary span { margin-left: auto; color: var(--muted); font-size: 11px; }.catalog-fields { display: grid; gap: 12px; padding: 0 12px 12px; margin: 0; }.catalog-fields dt { display: flex; align-items: center; gap: 7px; font-size: 12px; }.catalog-fields dt span { color: var(--muted); font-size: 10px; }.catalog-fields dd { margin: 4px 0 0; color: var(--muted); font-size: 11px; line-height: 1.6; }
.type-choice { width: 100%; display: flex; align-items: center; gap: 12px; padding: 16px; border: 1px solid var(--outline); border-radius: 9px; background: var(--surface); text-align: left; }.type-choice:hover:not(:disabled) { border-color: var(--accent); background: var(--tonal); }.type-icon { display: grid; place-items: center; width: 36px; height: 36px; border-radius: 8px; background: var(--tonal); color: var(--accent); flex-shrink: 0; }.choice-body { display: grid; gap: 5px; flex: 1; min-width: 0; }.choice-body strong { font-size: 13px; font-weight: 500; }.choice-body > span { color: var(--muted); font-size: 12px; line-height: 1.65; }.choice-chevron { flex-shrink: 0; color: var(--muted); }
</style>
