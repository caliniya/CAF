<script setup lang="ts">
import { ref, watch } from 'vue'

const KEY = 'caf.settings'

interface Prefs {
  restoreLast: boolean
  notifyDone: boolean
}

function defaults(): Prefs {
  return { restoreLast: true, notifyDone: false }
}

function load(): Prefs {
  try {
    const raw = localStorage.getItem(KEY)
    if (raw) return { ...defaults(), ...(JSON.parse(raw) as Partial<Prefs>) }
  } catch {
    /* 读不出来就用默认值 */
  }
  return defaults()
}

const settings = ref<Prefs>(load())

watch(
  settings,
  (value) => {
    try {
      localStorage.setItem(KEY, JSON.stringify(value))
    } catch {
      /* 存不了不影响使用 */
    }
  },
  { deep: true },
)
</script>

<template>
  <section class="page">
    <header class="head">
      <h1>通用设置</h1>
      <p class="sub">这些偏好保存在本机，不会同步到其他设备。</p>
    </header>

    <div class="groups">
      <section class="group">
        <h2>对话</h2>

        <label class="line">
          <span class="line-text">
            <span class="line-title">启动时恢复上次对话</span>
            <span class="line-desc">打开应用后直接回到上次停留的会话</span>
          </span>
          <input v-model="settings.restoreLast" type="checkbox" class="switch" />
        </label>

        <label class="line">
          <span class="line-text">
            <span class="line-title">生成完成时提醒</span>
            <span class="line-desc">回复结束时弹出系统通知</span>
          </span>
          <input v-model="settings.notifyDone" type="checkbox" class="switch" />
        </label>
      </section>

      <section class="group">
        <h2>外观</h2>

        <div class="line">
          <span class="line-text">
            <span class="line-title">主题</span>
            <span class="line-desc">深色主题还在开发中，当前固定浅色</span>
          </span>
          <select class="select" disabled>
            <option>浅色</option>
          </select>
        </div>

        <div class="line">
          <span class="line-text">
            <span class="line-title">界面字体</span>
            <span class="line-desc">使用自带的 LXGW 字体，缺失时回退到系统字体</span>
          </span>
          <span class="value">LXGW</span>
        </div>
      </section>

      <section class="group">
        <h2>数据</h2>

        <div class="line">
          <span class="line-text">
            <span class="line-title">数据目录</span>
            <span class="line-desc">
              配置、会话记录与日志都写在这里。便携模式下位于程序目录的
              <code>data</code> 文件夹，可用环境变量 <code>CAF_DATA_DIR</code> 覆盖。
            </span>
          </span>
        </div>
      </section>
    </div>
  </section>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  padding: 26px 30px;
  overflow-y: auto;
}

.head {
  width: 100%;
  max-width: 680px;
  margin: 0 auto;
  padding-bottom: 18px;
}

.head h1 {
  font-size: 21px;
  font-weight: 600;
}

.sub {
  margin-top: 4px;
  color: var(--muted);
  font-size: 12.5px;
}

.groups {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  max-width: 680px;
  margin: 0 auto;
}

.group {
  padding: 4px 18px;
  border: 1px solid var(--outline);
  border-radius: 14px;
  background: var(--surface);
}

.group h2 {
  padding: 14px 0 6px;
  color: var(--muted);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.05em;
}

.line {
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 12px 0;
  border-top: 1px solid var(--outline);
}

.line-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}

.line-title {
  color: var(--ink);
  font-size: 13.5px;
}

.line-desc {
  color: var(--muted);
  font-size: 12px;
  line-height: 1.55;
}

.line-desc code {
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--page);
  font-family: var(--font-mono);
  font-size: 11.5px;
}

.value {
  flex-shrink: 0;
  color: var(--muted);
  font-size: 12.5px;
}

.select {
  flex-shrink: 0;
  height: 30px;
  padding: 0 8px;
  border: 1px solid var(--outline);
  border-radius: 8px;
  background: var(--surface);
  color: var(--muted);
  font-size: 12.5px;
  cursor: not-allowed;
}

/* 开关：checkbox 改造 */
.switch {
  position: relative;
  flex-shrink: 0;
  width: 36px;
  height: 20px;
  margin: 0;
  border-radius: 999px;
  background: var(--outline);
  appearance: none;
  cursor: pointer;
  transition: background-color 150ms ease;
}

.switch::after {
  content: '';
  position: absolute;
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 3px rgb(0 0 0 / 22%);
  transition: transform 150ms ease;
}

.switch:checked {
  background: var(--accent);
}

.switch:checked::after {
  transform: translateX(16px);
}
</style>
