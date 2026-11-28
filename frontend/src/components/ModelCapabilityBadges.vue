<script setup lang="ts">
import { imageInputState, type Model } from '../api'
defineProps<{ model: Model }>()
</script>

<template>
  <span v-if="imageInputState(model) === 'supported'" class="capability" :title="model.vision_confirmed ? '你已手动确认此模型可接收图片' : '此模型支持图片输入'">视觉</span>
  <span v-if="model.native_web_search === true" class="capability"
    :title="model.native_search_mode ? '对话时自动启用原生搜索，模型按需查询。' : '供应商声明支持原生搜索，CAF 尚未适配此接口。'">{{ model.native_search_mode ? '原生搜索' : '搜索待适配' }}</span>
</template>

<style scoped>
.capability { flex: none; padding: 1px 6px; border-radius: 5px; background: var(--tonal); color: var(--accent); font-size: 11px; line-height: 1.6; white-space: nowrap; }
</style>
