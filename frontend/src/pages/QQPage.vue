<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useQQTestStore } from '../stores/qqTest'

const test = useQQTestStore()
let refreshTimer: number | undefined
const onlineLabel = computed(() => {
  if (!test.status?.info.connected) return '未知'
  if (test.status.info.online === true) return '在线'
  if (test.status.info.online === false) return '离线'
  return '等待状态上报'
})
const eventLabels: Record<string, string> = {
  started: '启动', stopped: '停止', message: '消息', request: '申请',
  notice: '通知', status: '状态', error: '错误', send: '发送', send_error: '发送失败',
}

function formatTime(value: string) {
  return new Date(value).toLocaleTimeString('zh-CN', { hour12: false })
}

async function copyUrl(value: string) {
  let copied = false
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
      copied = true
    }
  } catch {
    // 桌面运行时不支持 clipboard 时使用文本框复制。
  }
  if (!copied) {
    const input = document.createElement('textarea')
    input.value = value
    input.style.position = 'fixed'
    input.style.opacity = '0'
    document.body.appendChild(input)
    input.select()
    copied = document.execCommand('copy')
    input.remove()
  }
  test.copiedUrl = copied ? value : ''
}

onMounted(() => {
  void test.refresh()
  refreshTimer = window.setInterval(() => { void test.refresh() }, 1000)
})

onUnmounted(() => window.clearInterval(refreshTimer))
</script>

<template>
  <section class="qq-page" aria-labelledby="qq-page-title">
    <header class="qq-head">
      <div>
        <h1 id="qq-page-title">QQ 连接测试</h1>
        <p>启动反向 WebSocket，连接后收发测试消息。</p>
      </div>
      <span class="connection-badge" :class="{ connected: test.status?.info.connected, offline: test.status?.info.online === false }">
        <span class="connection-dot" aria-hidden="true"></span>
        {{ test.connectionLabel }}
      </span>
    </header>

    <div class="test-content">
      <form class="test-panel" @submit.prevent="test.start">
        <header class="panel-head">
          <h2>反向 WebSocket</h2>
          <span class="panel-note">OneBot 11</span>
        </header>
        <fieldset class="config-fields" :disabled="test.status?.running || test.changing">
          <label class="field">
            <span>监听地址</span>
            <input v-model="test.config.address" required type="text" placeholder="0.0.0.0:6701" spellcheck="false" />
          </label>
          <label class="field">
            <span>路径</span>
            <input v-model="test.config.path" required type="text" placeholder="/onebot/v11/ws" spellcheck="false" />
          </label>
          <label class="field">
            <span>访问令牌 <small>可选</small></span>
            <input v-model="test.config.accessToken" type="password" autocomplete="off" placeholder="与 OneBot 端填写一致" />
          </label>
          <label class="field">
            <span>机器人 QQ <small>可选</small></span>
            <input v-model="test.config.selfId" type="text" inputmode="numeric" placeholder="留空自动绑定首次连接的账号" spellcheck="false" />
          </label>
          <label class="field">
            <span>发送超时（秒）</span>
            <input v-model.number="test.config.timeoutSeconds" required type="number" min="1" max="300" />
          </label>
        </fieldset>

        <div class="adapter-actions">
          <button v-if="!test.status?.running" type="submit" class="btn btn-primary" :disabled="test.changing || test.clearing || !test.status">
            {{ test.changing ? '启动中…' : '启动适配器' }}
          </button>
          <button v-else type="button" class="btn btn-danger-outline" :disabled="test.changing || test.clearing" @click="test.stop">
            {{ test.changing ? '停止中…' : '停止适配器' }}
          </button>
          <span class="hint">切换页面后继续运行，退出应用时停止。</span>
        </div>
        <p v-if="test.error || test.refreshError" class="error-text" role="alert">{{ test.error || test.refreshError }}</p>

        <div v-if="test.status?.running" class="connection-details">
          <dl class="status-facts">
            <div><dt>监听</dt><dd>{{ test.status.address }}</dd></div>
            <div><dt>账号</dt><dd>{{ test.status.info.self_id || '等待连接' }}<span v-if="test.status.info.nickname"> · {{ test.status.info.nickname }}</span></dd></div>
            <div><dt>QQ 状态</dt><dd>{{ onlineLabel }}</dd></div>
            <div><dt>认证</dt><dd>{{ test.status.authEnabled ? '已启用令牌' : '未设置令牌' }}</dd></div>
          </dl>
          <p v-if="test.status.info.last_error" class="connection-error">{{ test.status.info.last_error }}</p>
          <p class="url-title">在 OneBot 端填写反向 WebSocket URL：</p>
          <div v-for="url in test.status.urls" :key="url" class="connection-url">
            <code>{{ url }}</code>
            <button type="button" class="btn btn-ghost btn-sm" @click="copyUrl(url)">
              {{ test.copiedUrl === url ? '已复制' : '复制' }}
            </button>
          </div>
          <p class="hint">NapCat 在 WSL 中时，使用能访问的 Windows IP；令牌填写到连接配置中。</p>
        </div>
      </form>

      <form class="test-panel" @submit.prevent="test.send">
        <header class="panel-head">
          <h2>手动发送</h2>
        </header>
        <div class="send-target">
          <label class="field target-type">
            <span>会话类型</span>
            <select v-model="test.targetType" :disabled="test.sending">
              <option value="private">私聊</option>
              <option value="group">群聊</option>
            </select>
          </label>
          <label class="field target-id">
            <span>{{ test.targetType === 'group' ? '群号' : '对方 QQ' }}</span>
            <input v-model="test.targetId" required type="text" inputmode="numeric" :disabled="test.sending"
              :placeholder="test.targetType === 'group' ? '填写群号' : '填写接收人的 QQ 号'" spellcheck="false" />
          </label>
        </div>
        <label class="field message-field">
          <span>消息正文</span>
          <textarea v-model="test.draft" required rows="3" :disabled="test.sending" placeholder="填写要发送的测试消息"></textarea>
        </label>
        <div class="send-actions">
          <span class="hint">{{ test.status?.info.connected ? '直接发送填写的文字。' : '等待反向连接建立后即可发送。' }}</span>
          <button type="submit" class="btn btn-primary" :disabled="!test.canSend">
            {{ test.sending ? '发送中…' : '发送消息' }}
          </button>
        </div>
        <p v-if="test.sendError" class="error-text" role="alert">{{ test.sendError }}</p>
        <p v-if="test.sendResult" class="success-text" role="status">{{ test.sendResult }}</p>
      </form>

      <section class="test-panel event-panel" aria-labelledby="qq-events-title">
        <header class="panel-head">
          <h2 id="qq-events-title">事件记录 <span class="event-count">{{ test.entries.length }}</span></h2>
          <div class="event-actions">
            <button type="button" class="btn btn-ghost btn-sm" :disabled="test.refreshing" @click="test.refresh">刷新</button>
            <button type="button" class="btn btn-ghost btn-sm" :disabled="test.clearing || !test.entries.length" @click="test.clearLog">清空</button>
          </div>
        </header>
        <p v-if="!test.entries.length" class="events-empty">连接状态、收到的消息和发送结果将在这里显示。</p>
        <div v-else class="event-list">
          <div v-for="entry in [...test.entries].reverse()" :key="entry.id" class="event-row"
            :class="{ failed: entry.kind === 'error' || entry.kind === 'send_error' }">
            <details class="event-details">
              <summary>
                <time>{{ formatTime(entry.time) }}</time>
                <span class="event-kind">{{ eventLabels[entry.kind] || entry.kind }}</span>
                <span class="event-summary">{{ entry.summary }}</span>
              </summary>
              <pre v-if="entry.detail">{{ entry.detail }}</pre>
            </details>
            <button v-if="entry.targetId && entry.messageType" type="button" class="btn btn-ghost btn-sm reply-button"
              :disabled="test.sending" @click="test.selectTarget(entry)">发到此会话</button>
          </div>
        </div>
        <p class="log-note">保留最近 200 条记录。</p>
      </section>
    </div>
  </section>
</template>

<style scoped>
.qq-page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.qq-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 24px 28px 18px;
  border-bottom: 1px solid var(--outline);
}

.qq-head h1 {
  font-size: 20px;
  font-weight: 600;
}

.qq-head p {
  margin-top: 4px;
  color: var(--muted);
  font-size: 12px;
}

.connection-badge {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
  padding: 4px 9px;
  border-radius: 999px;
  background: var(--page);
  color: var(--muted);
  font-size: 11.5px;
}

.connection-badge.connected {
  background: var(--success-soft);
  color: var(--success);
}

.connection-badge.offline {
  background: var(--page);
  color: var(--warning);
}

.connection-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.test-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  max-width: 960px;
  margin: 0 auto;
  padding: 20px 28px 28px;
}

.test-panel {
  min-width: 0;
  padding: 16px 18px;
  border: 1px solid var(--outline);
  border-radius: 12px;
  background: var(--surface);
}

.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.panel-head h2 {
  font-size: 14px;
  font-weight: 600;
}

.panel-note,
.event-count {
  color: var(--muted);
  font-size: 11px;
  font-weight: 400;
}

.event-count {
  margin-left: 6px;
}

.config-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 16px;
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.field > span {
  font-size: 12px;
}

.field small {
  margin-left: 4px;
  color: var(--muted);
  font-size: 10.5px;
}

.field input,
.field select,
.field textarea {
  width: 100%;
  min-width: 0;
  padding: 7px 9px;
  border: 1px solid var(--outline);
  border-radius: 7px;
  background: var(--surface);
  color: var(--ink);
  font-size: 12.5px;
}

.field input:disabled,
.field select:disabled,
.field textarea:disabled {
  background: var(--page);
  color: var(--muted);
}

.field input::placeholder,
.field textarea::placeholder {
  color: var(--muted);
}

.field textarea {
  min-height: 88px;
  resize: vertical;
}

.adapter-actions,
.send-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 14px;
}

.hint {
  color: var(--muted);
  font-size: 11.5px;
  line-height: 1.6;
}

.connection-details {
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--outline);
}

.status-facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 14px;
  margin: 0 0 14px;
}

.status-facts > div {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 11.5px;
}

.status-facts dt {
  color: var(--muted);
}

.status-facts dd {
  min-width: 0;
  margin: 0;
}

.url-title {
  margin-bottom: 7px;
  font-size: 12px;
}

.connection-url {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 6px;
  padding: 6px 8px;
  border-radius: 7px;
  background: var(--page);
}

.connection-url code {
  min-width: 0;
  font: 11px/1.6 var(--font-mono);
  overflow-wrap: anywhere;
}

.send-target {
  display: flex;
  gap: 12px;
}

.target-type {
  flex: 0 0 100px;
}

.target-id {
  flex: 1;
}

.message-field {
  margin-top: 12px;
}

.error-text,
.connection-error,
.success-text {
  margin-top: 10px;
  font-size: 12px;
  white-space: pre-wrap;
}

.error-text {
  color: var(--danger);
}

.connection-error {
  margin-bottom: 12px;
  color: var(--warning);
}

.success-text {
  color: var(--success);
}

.event-actions {
  display: flex;
  gap: 4px;
}

.events-empty {
  padding: 24px 8px;
  color: var(--muted);
  font-size: 12px;
  text-align: center;
}

.event-list {
  max-height: 440px;
  overflow-y: auto;
}

.event-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 0;
  border-top: 1px solid var(--outline);
}

.event-details {
  flex: 1;
  min-width: 0;
}

.event-details summary {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 8px;
  cursor: pointer;
  font-size: 12px;
}

.event-details time {
  color: var(--muted);
  font-size: 10.5px;
}

.event-kind {
  flex-shrink: 0;
  color: var(--accent);
  font-size: 11px;
}

.failed .event-kind,
.failed .event-summary {
  color: var(--danger);
}

.event-summary {
  flex: 1;
  min-width: 140px;
  white-space: pre-wrap;
}

.event-details pre {
  max-height: 260px;
  margin: 10px 0 0;
  padding: 10px;
  border-radius: 7px;
  background: var(--page);
  color: var(--muted);
  font: 11px/1.65 var(--font-mono);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  overflow-y: auto;
}

.reply-button {
  flex-shrink: 0;
}

.log-note {
  margin-top: 10px;
  color: var(--muted);
  font-size: 10.5px;
}

@media (max-width: 800px) {
  .qq-head {
    padding: 20px 18px 16px;
  }

  .test-content {
    padding: 16px 18px 24px;
  }

  .config-fields,
  .status-facts {
    grid-template-columns: minmax(0, 1fr);
  }

  .adapter-actions {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
