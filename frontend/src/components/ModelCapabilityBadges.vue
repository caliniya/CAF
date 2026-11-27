<script setup lang="ts">
import { imageInputState, type Model } from '../api'
defineProps<{ model: Model }>()
</script>

<template>
  <span v-if="imageInputState(model) === 'supported'" class="capability" :title="model.vision_confirmed ? '你已手动确认此模型可接收图片' : '此模型支持图片输入'">视觉</span>
  <span v-if="model.native_web_search === true" class="capability" title="此模型在该供应商的对话接口支持原生搜索">原生搜索</span>
</template>

<style scoped>
.capability { flex: none; padding: 1px 6px; border-radius: 5px; background: var(--tonal); color: var(--accent); font-size: 11px; line-height: 1.6; white-space: nowrap; }
</style>
