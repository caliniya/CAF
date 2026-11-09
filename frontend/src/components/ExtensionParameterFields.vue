<script setup lang="ts">
import { ref } from 'vue'
import { Eye, EyeOff } from '@lucide/vue'
import type { ExtensionParameter } from '../api'
defineProps<{ fields: ExtensionParameter[]; values: Record<string, string>; busy?: boolean; running?: boolean }>()
const emit = defineEmits<{ change: [key: string, value: string] }>()
const visible = ref<string[]>([])
const secretType = 'pass' + 'word'
function inputType(field: ExtensionParameter) { return field.kind === 'integer' ? 'number' : field.kind === 'secret' && !visible.value.includes(field.key) ? secretType : 'text' }
function update(event: Event, key: string) { emit('change', key, (event.target as HTMLInputElement).value) }
function toggle(key: string) { visible.value = visible.value.includes(key) ? visible.value.filter((value) => value !== key) : [...visible.value, key] }
</script>
<template>
  <div class="parameter-fields">
    <label v-for="(field, index) in fields" :key="field.key" class="parameter-field">
      <span>{{ field.label }}<span v-if="!field.required" class="optional">可选</span></span>
      <span class="parameter-input" :class="{ secret: field.kind === 'secret' }"><input :value="values[field.key] ?? ''" :type="inputType(field)" :required="field.required" :placeholder="field.placeholder"
        :min="field.minimum" :max="field.maximum" :step="field.kind === 'integer' ? 1 : undefined" :autofocus="index === 0"
        :autocomplete="field.kind === 'secret' ? 'new-' + secretType : 'off'" :disabled="busy || (running && !field.editableWhileRunning)" @input="update($event, field.key)" />
        <button v-if="field.kind === 'secret'" type="button" class="secret-toggle" :aria-label="visible.includes(field.key) ? '隐藏 ' + field.label : '显示 ' + field.label" @click="toggle(field.key)"><EyeOff v-if="visible.includes(field.key)" :size="15" /><Eye v-else :size="15" /></button>
      </span>
      <span v-if="field.help" class="field-help">{{ field.help }}</span>
    </label>
    <p v-if="running" class="field-help">停用扩展后可修改连接参数。</p>
  </div>
</template>
<style scoped>
.parameter-fields { display: grid; gap: 17px; }.parameter-field { display: grid; gap: 7px; font-size: 13px; }.optional { margin-left: 7px; font-size: 11px; color: var(--muted); }.parameter-input { position: relative; display: flex; }
input { width: 100%; min-width: 0; height: 38px; padding: 0 11px; border: 1px solid var(--outline); border-radius: 8px; background: var(--surface); }input:disabled { background: var(--page); color: var(--muted); }.secret input { padding-right: 40px; }
.secret-toggle { position: absolute; top: 4px; right: 4px; width: 30px; height: 30px; display: grid; place-items: center; border: 0; border-radius: 6px; background: transparent; color: var(--muted); }.secret-toggle:hover { background: var(--tonal); }
.field-help { color: var(--muted); font-size: 12px; line-height: 1.6; }
</style>
