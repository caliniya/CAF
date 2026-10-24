<script setup lang="ts">
import { useConfigsStore } from '../stores/configs'

const store = useConfigsStore()
</script>

<template>
  <div class="notice">
    <template v-if="store.enables">
      <strong>当前使用：</strong>
      <span>{{ store.enables.Name || store.enables.ServerAddress }} · 模型 </span>
      <span v-if="store.enabledModel" class="badge badge-active">
        {{ store.enabledModel }}
      </span>
      <span v-else class="muted">未选择</span>
    </template>
    <template v-else>
      尚未启用配置：点击「启用此配置」，或直接点击某个模型徽章。
    </template>
  </div>
</template>
