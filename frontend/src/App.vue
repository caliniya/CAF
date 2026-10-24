<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { useConfigsStore } from './stores/configs'

const route = useRoute()
const store = useConfigsStore()

// 后端就绪 = 配置成功读取过且没有未恢复的错误，用于侧边栏状态指示。
const ready = computed(() => store.loaded && !store.error)
const pageTitle = computed(() => (route.meta.title as string | undefined) ?? 'CAF')
</script>

<template>
  <div class="app">
    <aside class="sidebar" aria-label="导航">
      <div class="stack">
        <h1>CAF</h1>
        <p class="muted">Agent 框架</p>
      </div>

      <nav class="nav">
        <RouterLink to="/chat" class="nav-link">对话</RouterLink>
        <RouterLink to="/configs" class="nav-link">模型配置</RouterLink>
      </nav>

      <div class="sidebar-meta">
        <span>v0.1.0</span>
        <span class="status-line">
          <span class="dot" :class="{ ready }"></span>
          {{ ready ? '后端就绪' : '连接中' }}
        </span>
      </div>
    </aside>

    <main class="content stack">
      <header class="page-header">
        <h2>{{ pageTitle }}</h2>
      </header>
      <RouterView />
    </main>
  </div>
</template>
