<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, useId, watch } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import { Check, ChevronDown, MessageCircle, Monitor } from '@lucide/vue'
import { useChatStore } from './stores/chat'
import SidebarTasks from './components/SidebarTasks.vue'
import { APP_AREAS, readAppArea, saveAppArea, type AppArea } from './navigation'

const route = useRoute()
const router = useRouter()
const chat = useChatStore()

const inSettings = computed(() => route.path.startsWith('/settings'))
const selectedArea = ref<AppArea>(readAppArea())
const areaOptions = [
  { id: 'agent', ...APP_AREAS.agent },
  { id: 'qq', ...APP_AREAS.qq },
] as const
const activeArea = computed(() => APP_AREAS[selectedArea.value])
const areaMenuOpen = ref(false)
const areaSwitcher = ref<HTMLElement | null>(null)
const areaTrigger = ref<HTMLButtonElement | null>(null)
const areaMenu = ref<HTMLElement | null>(null)
const areaMenuId = useId() + '-area-menu'

watch(
  () => route.meta.area,
  (area) => {
    if (area !== 'agent' && area !== 'qq') return
    selectedArea.value = area
    saveAppArea(area)
  },
  { immediate: true },
)

watch(() => route.fullPath, () => { areaMenuOpen.value = false })

function closeAreaMenu(restoreFocus = false) {
  areaMenuOpen.value = false
  if (restoreFocus) areaTrigger.value?.focus()
}

async function openAreaMenu() {
  areaMenuOpen.value = true
  await nextTick()
  areaMenu.value?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus()
}

function toggleAreaMenu() {
  if (areaMenuOpen.value) closeAreaMenu()
  else void openAreaMenu()
}

function selectArea(area: AppArea) {
  closeAreaMenu(true)
  void router.push(APP_AREAS[area].path)
}

function onAreaMenuKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    closeAreaMenu(true)
    return
  }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const buttons = Array.from(areaMenu.value?.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]') ?? [])
  if (!buttons.length) return
  const current = buttons.findIndex((button) => button === document.activeElement)
  const next = event.key === 'Home' ? 0
    : event.key === 'End' ? buttons.length - 1
    : (current + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length
  buttons[next]?.focus()
}

function closeAreaMenuOnOutside(event: PointerEvent) {
  if (event.target instanceof Node && !areaSwitcher.value?.contains(event.target)) closeAreaMenu()
}

function onAreaFocusOut(event: FocusEvent) {
  if (!(event.relatedTarget instanceof Node) || !areaSwitcher.value?.contains(event.relatedTarget)) closeAreaMenu()
}

onMounted(() => document.addEventListener('pointerdown', closeAreaMenuOnOutside))
onUnmounted(() => document.removeEventListener('pointerdown', closeAreaMenuOnOutside))

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
  closeAreaMenu()
  void router.push(inSettings.value ? activeArea.value.path : '/settings/general')
}
</script>

<template>
  <div class="app">
    <aside class="sidebar" :style="{ width: `${sidebarWidth}px` }">
      <div class="brand">
        <span class="brand-name">CAF</span>
        <span class="brand-version">v0.2</span>
        <div ref="areaSwitcher" class="area-switcher" @focusout="onAreaFocusOut"
          @keydown.esc.stop.prevent="closeAreaMenu(true)">
          <button ref="areaTrigger" type="button" class="area-trigger" :class="{ open: areaMenuOpen }"
            :aria-label="'切换界面，当前为 ' + activeArea.name" aria-haspopup="menu"
            :aria-expanded="areaMenuOpen" :aria-controls="areaMenuId" @click="toggleAreaMenu"
            @keydown.down.prevent="openAreaMenu" @keydown.up.prevent="openAreaMenu">
            <span>{{ activeArea.name }}</span>
            <ChevronDown :size="12" :stroke-width="2" aria-hidden="true" />
          </button>
          <div v-if="areaMenuOpen" :id="areaMenuId" ref="areaMenu" class="menu area-menu"
            role="menu" aria-label="切换界面" @keydown.stop="onAreaMenuKeydown">
            <button v-for="area in areaOptions" :key="area.id" type="button" class="menu-row"
              :class="{ on: selectedArea === area.id }" role="menuitemradio"
              :aria-checked="selectedArea === area.id" @click="selectArea(area.id)">
              <span class="menu-row-name">{{ area.name }}</span>
              <Check v-if="selectedArea === area.id" :size="14" :stroke-width="2" aria-hidden="true" />
            </button>
          </div>
        </div>
      </div>

      <!-- 设置页共用导航，返回时回到所选界面。 -->
      <nav v-if="inSettings" class="nav" aria-label="设置">
        <RouterLink to="/settings/models" class="nav-item">模型配置</RouterLink>
        <RouterLink to="/settings/general" class="nav-item">通用设置</RouterLink>
      </nav>

      <!-- Agent：新建对话 + 会话列表 -->
      <template v-else-if="selectedArea === 'agent'">
        <button type="button" class="new-chat" :disabled="chat.sending" @click="newConversation">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <line x1="12" y1="5" x2="12" y2="19" />
            <line x1="5" y1="12" x2="19" y2="12" />
          </svg>
          <span>新建对话</span>
        </button>

        <SidebarTasks />
      </template>

      <nav v-else class="nav" aria-label="QQ">
        <RouterLink to="/qq" class="nav-item">
          <MessageCircle :size="15" :stroke-width="1.7" aria-hidden="true" />
          <span>消息</span>
        </RouterLink>
        <RouterLink to="/qq/clients" class="nav-item">
          <Monitor :size="15" :stroke-width="1.7" aria-hidden="true" />
          <span>客户端</span>
        </RouterLink>
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
          <span>{{ inSettings ? '返回 ' + activeArea.name : '设置' }}</span>
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
      <RouterView v-slot="{ Component }">
        <KeepAlive include="ChatPage">
          <component :is="Component" />
        </KeepAlive>
      </RouterView>
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
  align-items: center;
  gap: 10px;
  padding: 0 4px 14px;
}

.brand-name {
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.01em;
}

.brand-version {
  flex-shrink: 0;
  color: var(--muted);
  font-size: 11px;
}

.area-switcher {
  position: relative;
  margin-left: auto;
}

.area-trigger {
  display: flex;
  align-items: center;
  gap: 7px;
  min-height: 30px;
  padding: 4px 8px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--ink);
  font-size: 12.5px;
  cursor: pointer;
  transition: background-color 120ms ease;
}

.area-trigger:hover,
.area-trigger.open {
  background: var(--tonal);
}

.area-trigger svg {
  color: var(--muted);
}

.area-menu {
  top: calc(100% + 6px);
  right: 0;
  min-width: 148px;
  width: 148px;
  max-width: calc(100vw - 32px);
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
  gap: 8px;
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
