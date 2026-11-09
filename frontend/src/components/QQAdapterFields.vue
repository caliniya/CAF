<script setup lang="ts">
import { ref } from 'vue'
import { Check, Copy, Eye, EyeOff } from '@lucide/vue'
import type { QQAdapterInput } from '../api'

const input = defineModel<QQAdapterInput>({ required: true })
defineProps<{
  address: string
  busy?: boolean
  connectionLocked?: boolean
  autofocus?: boolean
}>()
const showToken = ref(false)
const copied = ref(false)
const copyError = ref('')
const secretInputType = 'pass' + 'word'

async function copyAddress(address: string) {
  copyError.value = ''
  try {
    await navigator.clipboard.writeText(address)
    copied.value = true
  } catch {
    copyError.value = '复制失败，请选中地址后手动复制。'
  }
}
</script>

<template>
  <div class="adapter-fields">
    <label class="adapter-field">
      <span>名称</span>
      <input v-model="input.name" type="text" placeholder="为适配器取一个名称" required
        :autofocus="autofocus" :disabled="busy" />
    </label>
    <label class="adapter-field">
      <span>Token</span>
      <span class="token-input">
        <input v-model="input.token" :type="showToken ? 'text' : secretInputType" required
          :autocomplete="'new-' + secretInputType" placeholder="输入连接令牌" :disabled="busy || connectionLocked" />
        <button type="button" class="token-visibility" :aria-label="showToken ? '隐藏 Token' : '显示 Token'"
          :aria-pressed="showToken" :disabled="busy" @click="showToken = !showToken">
          <EyeOff v-if="showToken" :size="16" aria-hidden="true" />
          <Eye v-else :size="16" aria-hidden="true" />
        </button>
      </span>
    </label>
    <label class="adapter-field">
      <span>QQ 号 <span class="optional">可选</span></span>
      <input v-model="input.selfId" type="text" inputmode="numeric" pattern="[0-9]*"
        placeholder="留空时从连接的客户端获取" :disabled="busy || connectionLocked" />
    </label>
    <p v-if="connectionLocked" class="field-hint">停用 QQ 扩展后可修改 Token 和 QQ 号。</p>
    <label class="adapter-field">
      <span>适配器连接地址</span>
      <span class="connection-address">
        <input :value="address" type="text" readonly placeholder="正在获取地址…" />
        <button type="button" class="btn btn-ghost btn-sm" :disabled="!address"
          :aria-label="copied ? '地址已复制' : '复制连接地址'" @click="copyAddress(address)">
          <Check v-if="copied" :size="14" aria-hidden="true" />
          <Copy v-else :size="14" aria-hidden="true" />
          {{ copied ? '已复制' : '复制' }}
        </button>
      </span>
    </label>
    <p class="field-hint">在 QQ 客户端中配置反向 WebSocket，使用以上地址和相同的 Token。</p>
    <p class="field-hint">QQ 客户端位于 WSL 或其他设备时，将 127.0.0.1 替换为 CAF 所在电脑的可访问 IP。</p>
    <p v-if="copyError" class="field-error" role="alert">{{ copyError }}</p>
  </div>
</template>

<style scoped>
.adapter-fields { display: grid; gap: 18px; }
.adapter-field { display: grid; gap: 7px; font-size: 13px; font-weight: 500; }
.adapter-field input {
  width: 100%; min-width: 0; height: 38px; padding: 0 11px;
  border: 1px solid var(--outline); border-radius: 8px;
  background: var(--surface); color: var(--ink); font-size: 13px; font-weight: 400;
}
.adapter-field input:disabled { background: var(--page); color: var(--muted); }
.optional { margin-left: 5px; color: var(--muted); font-size: 12px; font-weight: 400; }
.token-input { position: relative; display: flex; }
.token-input input { padding-right: 40px; }
.token-visibility {
  position: absolute; top: 4px; right: 4px; display: grid; place-items: center;
  width: 30px; height: 30px; padding: 0; border: 0; border-radius: 6px;
  background: transparent; color: var(--muted);
}
.token-visibility:hover { background: var(--tonal); color: var(--accent); }
.connection-address { display: flex; align-items: center; gap: 7px; }
.connection-address input { font-family: var(--font-mono); font-size: 12px; background: var(--page); }
.field-hint { color: var(--muted); font-size: 12px; line-height: 1.65; }
.field-hint + .field-hint { margin-top: -10px; }
.field-error { color: var(--danger); font-size: 13px; }
</style>
