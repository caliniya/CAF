<script setup lang="ts">
import { computed } from 'vue'
import { ChevronRight } from '@lucide/vue'
import type { ToolParameterSchema } from '../api'

const props = defineProps<{
  name: string
  schema: ToolParameterSchema | boolean
  required?: boolean
}>()

const definition = computed<ToolParameterSchema>(() => typeof props.schema === 'boolean' ? {} : props.schema)
const typeNames: Record<string, string> = {
  string: '字符串', number: '数字', integer: '整数', boolean: '布尔值',
  object: '对象', array: '数组', null: '空值',
}

function typeLabel(schema: ToolParameterSchema | boolean): string {
  if (typeof schema === 'boolean') return schema ? '任意类型' : '不允许'
  const type = schema.type ?? (schema.properties ? 'object' : schema.items ? 'array' : undefined)
  if (Array.isArray(type)) return type.map((value) => typeNames[value] ?? value).join(' / ')
  if (type === 'array' && schema.items && !Array.isArray(schema.items)) {
    return '数组 · ' + typeLabel(schema.items)
  }
  return type ? typeNames[type] ?? type : '未限定类型'
}

function displayValue(value: unknown): string {
  return JSON.stringify(value, null, 2) ?? String(value)
}

const facts = computed(() => {
  const schema = definition.value
  const result: { label: string; value: string }[] = []
  const labels: Record<string, string> = {
    default: '默认值', enum: '可选值', const: '固定值', examples: '示例',
    minimum: '最小值', maximum: '最大值', exclusiveMinimum: '必须大于', exclusiveMaximum: '必须小于',
    multipleOf: '数值步长', minLength: '最短长度', maxLength: '最长长度',
    minItems: '最少项数', maxItems: '最多项数', uniqueItems: '元素唯一',
    minProperties: '最少属性数', maxProperties: '最多属性数', pattern: '匹配表达式', format: '格式',
  }
  for (const [key, label] of Object.entries(labels)) {
    if (Object.hasOwn(schema, key)) result.push({ label, value: displayValue(schema[key]) })
  }
  if (typeof schema.additionalProperties === 'boolean') {
    result.push({ label: '额外属性', value: schema.additionalProperties ? '允许' : '不允许' })
  }
  return result
})

const children = computed(() => {
  const schema = definition.value
  const result: { name: string; schema: ToolParameterSchema | boolean; required?: boolean }[] = []
  for (const [name, child] of Object.entries(schema.properties ?? {})) {
    result.push({ name, schema: child, required: schema.required?.includes(name) ?? false })
  }
  if (Array.isArray(schema.items)) {
    schema.items.forEach((item, index) => result.push({ name: '第 ' + (index + 1) + ' 项', schema: item }))
  } else if (schema.items !== undefined) {
    result.push({ name: '数组元素', schema: schema.items })
  }
  if (schema.additionalProperties && typeof schema.additionalProperties === 'object') {
    result.push({ name: '额外属性', schema: schema.additionalProperties })
  }
  for (const [key, label] of [['oneOf', '任选一项'], ['anyOf', '至少一项'], ['allOf', '同时满足']] as const) {
    const variants = schema[key]
    if (!Array.isArray(variants)) continue
    variants.forEach((variant, index) => {
      if (typeof variant === 'boolean' || (variant && typeof variant === 'object')) {
        result.push({ name: label + ' · ' + (index + 1), schema: variant as ToolParameterSchema | boolean })
      }
    })
  }
  return result
})
</script>

<template>
  <details class="parameter-details">
    <summary class="parameter-summary">
      <ChevronRight class="fold-chevron" :size="12" :stroke-width="2" aria-hidden="true" />
      <code class="parameter-name">{{ name }}</code>
      <span v-if="required !== undefined" class="parameter-status" :class="{ required }">
        {{ required ? '必填' : '可选' }}
      </span>
      <span class="parameter-type">{{ typeLabel(schema) }}</span>
    </summary>
    <div class="parameter-body">
      <p class="parameter-description">{{ definition.description || '暂无参数说明' }}</p>
      <dl v-if="facts.length" class="parameter-facts">
        <div v-for="fact in facts" :key="fact.label">
          <dt>{{ fact.label }}</dt>
          <dd><pre>{{ fact.value }}</pre></dd>
        </div>
      </dl>
      <div v-if="children.length" class="parameter-children">
        <ToolParameterDetails v-for="child in children" :key="child.name"
          :name="child.name" :schema="child.schema" :required="child.required" />
      </div>
    </div>
  </details>
</template>

<style scoped>
.parameter-details {
  min-width: 0;
  border: 1px solid var(--outline);
  border-radius: 8px;
  background: var(--surface);
}

.parameter-summary {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  padding: 9px 10px;
  list-style: none;
  cursor: pointer;
}

.parameter-summary::-webkit-details-marker {
  display: none;
}

.parameter-summary:hover {
  background: var(--tonal);
  border-radius: 7px;
}

.fold-chevron {
  flex-shrink: 0;
  color: var(--muted);
  transition: transform 150ms ease;
}

.parameter-details[open] > .parameter-summary .fold-chevron {
  transform: rotate(90deg);
}

.parameter-name {
  color: var(--ink);
  font: 12px var(--font-mono);
  overflow-wrap: anywhere;
}

.parameter-status {
  flex-shrink: 0;
  padding: 0 5px;
  border-radius: 4px;
  background: var(--page);
  color: var(--muted);
  font-size: 10px;
}

.parameter-status.required {
  background: var(--tonal);
  color: var(--accent);
}

.parameter-type {
  margin-left: auto;
  color: var(--muted);
  font-size: 11px;
}

.parameter-body {
  padding: 0 10px 10px;
}

.parameter-description {
  color: var(--muted);
  font-size: 12px;
  line-height: 1.65;
  white-space: pre-wrap;
}

.parameter-facts {
  display: grid;
  gap: 7px;
  margin: 10px 0 0;
}

.parameter-facts > div {
  display: grid;
  grid-template-columns: 72px minmax(0, 1fr);
  gap: 8px;
  align-items: start;
}

.parameter-facts dt {
  color: var(--muted);
  font-size: 11px;
}

.parameter-facts dd {
  min-width: 0;
  margin: 0;
}

.parameter-facts pre {
  max-height: 160px;
  margin: 0;
  font: 11px/1.6 var(--font-mono);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  overflow-y: auto;
}

.parameter-children {
  display: grid;
  gap: 6px;
  margin-top: 10px;
}
</style>
