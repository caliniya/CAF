<script setup lang="ts">
import { ref, useId } from 'vue'
import { Eye, EyeOff } from '@lucide/vue'
import type { ExtensionParameter } from '../api'
const props = defineProps<{ fields: ExtensionParameter[]; values: Record<string, string>; busy?: boolean; running?: boolean }>()
const emit = defineEmits<{ change: [key: string, value: string] }>()
const visible = ref<string[]>([])
const inputPrefix = useId()
const secretType = 'pass' + 'word'
function inputType(field: ExtensionParameter) { return field.kind === 'integer' ? 'number' : field.kind === 'secret' && !visible.value.includes(field.key) ? secretType : 'text' }
function update(event: Event, key: string) { emit('change', key, (event.target as HTMLInputElement).value) }
function toggle(key: string) { visible.value = visible.value.includes(key) ? visible.value.filter((value) => value !== key) : [...visible.value, key] }
function disabled(field: ExtensionParameter) { return props.busy || (props.running && !field.editableWhileRunning) || (!!field.enabledWhen && props.values[field.enabledWhen] !== 'true') }
function booleanChange(event: Event, key: string) { emit('change', key, String((event.target as HTMLInputElement).checked)) }
function pairValues(field: ExtensionParameter) { return (props.values[field.key] ?? field.default ?? '').split(/[,，\s]+/) }
function pairChange(event: Event, field: ExtensionParameter, index: number) {
  const values = pairValues(field)
  values[index] = (event.target as HTMLInputElement).value
  emit('change', field.key, values.slice(0, 2).join(','))
}
</script>
<template>
  <div class="parameter-fields">
    <div v-for="(field, index) in fields" :key="field.key" class="parameter-field">
      <label class="field-label" :for="inputPrefix + field.key">{{ field.label }}<span v-if="!field.required && field.kind !== 'boolean'" class="optional">可选</span>
        <input v-if="field.kind === 'boolean'" :id="inputPrefix + field.key" class="boolean-input" type="checkbox" :checked="values[field.key] === 'true'" :disabled="disabled(field)" @change="booleanChange($event, field.key)" />
      </label>
      <div v-if="field.kind === 'integer-pair'" class="parameter-pair" :aria-label="field.label">
        <label v-for="pairIndex in [0, 1]" :key="pairIndex"><span>{{ field.inputLabels?.[pairIndex] ?? (pairIndex + 1) }}</span><input :id="inputPrefix + field.key + (pairIndex === 0 ? '' : '-second')" type="number" :value="pairValues(field)[pairIndex] ?? ''" :min="field.minimum" :max="field.maximum" step="1" :required="field.required" :disabled="disabled(field)" :aria-label="field.label + ' · ' + (field.inputLabels?.[pairIndex] ?? (pairIndex + 1))" @input="pairChange($event, field, pairIndex)" /></label>
      </div>
      <span v-else-if="field.kind !== 'boolean'" class="parameter-input" :class="{ secret: field.kind === 'secret' }"><input :id="inputPrefix + field.key" :value="values[field.key] ?? ''" :type="inputType(field)" :required="field.required" :aria-label="field.label" :placeholder="field.placeholder"
        :min="field.minimum" :max="field.maximum" :step="field.kind === 'integer' ? 1 : undefined" :autofocus="index === 0"
        :autocomplete="field.kind === 'secret' ? 'new-' + secretType : 'off'" :disabled="disabled(field)" @input="update($event, field.key)" />
        <button v-if="field.kind === 'secret'" type="button" class="secret-toggle" :aria-label="visible.includes(field.key) ? '隐藏 ' + field.label : '显示 ' + field.label" @click="toggle(field.key)"><EyeOff v-if="visible.includes(field.key)" :size="15" /><Eye v-else :size="15" /></button>
      </span>
      <span v-if="field.help" class="field-help">{{ field.help }}</span>
    </div>
    <p v-if="running" class="field-help">停用扩展后可修改连接参数。</p>
  </div>
</template>
<style scoped>
.parameter-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px 24px; width: 100%; }.parameter-field { display: grid; align-content: start; gap: 7px; min-width: 0; font-size: 13px; }.optional { margin-left: 7px; font-size: 11px; color: var(--muted); }.parameter-input { position: relative; display: flex; }
.parameter-field:has(.parameter-pair), .parameter-fields > p { grid-column: 1 / -1; }.parameter-pair { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }.parameter-pair label { display: grid; gap: 6px; min-width: 0; }.parameter-pair label > span { color: var(--muted); font-size: 12px; }
@media(max-width: 640px) { .parameter-fields { grid-template-columns: minmax(0, 1fr); } }
input { width: 100%; min-width: 0; height: 38px; padding: 0 11px; border: 1px solid var(--outline); border-radius: 8px; background: var(--surface); }input:disabled { background: var(--page); color: var(--muted); }.secret input { padding-right: 40px; }
.secret-toggle { position: absolute; top: 4px; right: 4px; width: 30px; height: 30px; display: grid; place-items: center; border: 0; border-radius: 6px; background: transparent; color: var(--muted); }.secret-toggle:hover { background: var(--tonal); }
.field-help { color: var(--muted); font-size: 12px; line-height: 1.6; }
.field-label { display: flex; align-items: center; gap: 5px; }.boolean-input { width: 34px; height: 19px; padding: 0; margin-left: auto; appearance: none; border-radius: 999px; background: var(--outline); cursor: pointer; transition: background .15s; }.boolean-input::before { content: ''; display: block; width: 13px; height: 13px; border-radius: 50%; margin: 2px; background: var(--surface); transition: transform .15s; }.boolean-input:checked { background: var(--accent); border-color: var(--accent); }.boolean-input:checked::before { transform: translateX(15px); }.boolean-input:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
</style>
