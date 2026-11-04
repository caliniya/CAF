<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import { useChatStore } from './stores/chat'
import SidebarTasks from './components/SidebarTasks.vue'

const route = useRoute()
const router = useRouter()
const chat = useChatStore()

const inSettings = computed(() => route.path.startsWith('/settings'))

// ===== 侧边栏拖拽调宽 =====
const MIN_SIDEBAR = 180
const MAX_SIDEBAR = 420
const WIDTH_KEY = 'caf.sidebar-width'

function readWidth(): number {
  try {
    const saved = Number(localStorage.getItem(WIDTH_KEY))
    if (Number.isFinite(saved) && saved >= MIN_SIDEBAR && saved <= MAX_SIDEBAR) return saved
  } catch {
    /* localStorage 不可用时用默认值 */
  }
  return 220
}

const sidebarWidth = ref(readWidth())
const resizing = ref(false)

function startResize(event: MouseEvent) {
  resizing.value = true
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  window.addEventListener('mousemove', onResizeMove)
  window.addEventListener('mouseup', stopResize)
  event.preventDefault()
}

function onResizeMove(event: MouseEvent) {
  if (!resizing.value) return
  // 侧边栏贴在窗口左侧，鼠标 x 坐标即目标宽度
  sidebarWidth.value = Math.min(MAX_SIDEBAR, Math.max(MIN_SIDEBAR, event.clientX))
}

function stopResize() {
  if (!resizing.value) return
  resizing.value = false
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  window.removeEventListener('mousemove', onResizeMove)
  window.removeEventListener('mouseup', stopResize)
  try {
    localStorage.setItem(WIDTH_KEY, String(sidebarWidth.value))
  } catch {
    /* 存不了就算了，不影响本次使用 */
  }
}

onUnmounted(stopResize)

async function newConversation() {
  if (chat.sending) return
  chat.startNewChat()
  await router.push('/chat')
}

function toggleSettings() {
  router.push(inSettings.value ? '/chat' : '/settings/general')
}
</script>

<template>
  <div class="app">
    <aside class="sidebar" :style="{ width: `${sidebarWidth}px` }">
      <div class="brand">
        <span class="brand-name">CAF</span>
        <span class="brand-version">v0.1.0</span>
      </div>

      <!-- 主界面：新建对话 + 会话列表 -->
      <template v-if="!inSettings">
        <button type="button" class="new-chat" :disabled="chat.sending" @click="newConversation">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <line x1="12" y1="5" x2="12" y2="19" />
            <line x1="5" y1="12" x2="19" y2="12" />
          </svg>
          <span>新建对话</span>
        </button>

        <SidebarTasks />
      </template>

      <!-- 设置：分区导航 -->
      <nav v-else class="nav" aria-label="设置">
        <RouterLink to="/settings/models" class="nav-item">模型配置</RouterLink>
        <RouterLink to="/settings/general" class="nav-item">通用设置</RouterLink>
      </nav>

      <div class="sidebar-foot">
        <button type="button" class="foot-btn" :class="{ active: inSettings }" @click="toggleSettings">
          <svg v-if="inSettings" viewBox="0 0 24 24" aria-hidden="true">
            <line x1="19" y1="12" x2="5" y2="12" />
            <polyline points="12 19 5 12 12 5" />
          </svg>
          <svg v-else viewBox="0 0 24 24" aria-hidden="true">
            <circle cx="12" cy="12" r="3" />
            <path
              d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
            />
          </svg>
          <span>{{ inSettings ? '返回对话' : '设置' }}</span>
        </button>
      </div>

      <!-- 拖拽手柄：贴在右边框上 -->
      <span
        class="resizer"
        :class="{ active: resizing }"
        role="separator"
        aria-orientation="vertical"
        aria-label="调整侧边栏宽度"
        @mousedown="startResize"
      ></span>
    </aside>

    <main class="main">
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.app {
  display: flex;
  height: 100vh;
  overflow: hidden;
}

.sidebar {
  position: relative;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  width: 220px;
  padding: 14px 12px 12px;
  border-right: 1px solid var(--outline);
  background: var(--page);
}

/* 拖拽手柄：压在右边框上，不占布局宽度 */
.resizer {
  position: absolute;
  top: 0;
  right: -3px;
  bottom: 0;
  z-index: 5;
  width: 7px;
  cursor: col-resize;
}

.resizer::after {
  content: '';
  position: absolute;
  inset: 0 3px;
  border-radius: 1px;
  background: transparent;
  transition: background-color 120ms ease;
}

.resizer:hover::after,
.resizer.active::after {
  background: color-mix(in srgb, var(--accent) 50%, transparent);
}

.brand {
  display: flex;
  align-items: baseline;
  gap: 7px;
  padding: 2px 10px 16px;
}

.brand-name {
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.01em;
}

.brand-version {
  font-size: 11px;
  color: var(--muted);
}

/* ===== 设置区的分区导航 ===== */
.nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 8px;
}

.nav-item {
  display: flex;
  align-items: center;
  height: 34px;
  padding: 0 10px;
  border-radius: 8px;
  color: var(--ink);
  font-size: 13.5px;
  text-decoration: none;
  transition: background-color 120ms ease, color 120ms ease;
}

.nav-item:hover {
  background: var(--tonal);
}

.nav-item.router-link-active {
  background: var(--tonal);
  color: var(--accent);
  font-weight: 500;
}

.sidebar-foot {
  margin-top: auto;
  padding-top: 10px;
}

.main {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  background: var(--surface);
}
</style>
