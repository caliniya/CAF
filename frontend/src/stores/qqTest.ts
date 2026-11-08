import { defineStore } from 'pinia'
import {
  ClearQQTestLog,
  GetQQTestState,
  SendQQTestMessage,
  StartQQTestAdapter,
  StopQQTestAdapter,
  type QQTestConfig,
  type QQTestLogEntry,
  type QQTestState,
} from '../api'

export const useQQTestStore = defineStore('qq-test', {
  state: () => ({
    config: {
      address: '0.0.0.0:6701',
      path: '/onebot/v11/ws',
      accessToken: '',
      selfId: '',
      timeoutSeconds: 30,
    } as QQTestConfig,
    status: null as QQTestState | null,
    entries: [] as QQTestLogEntry[],
    targetType: 'private' as 'private' | 'group',
    targetId: '',
    draft: '',
    changing: false,
    sending: false,
    refreshing: false,
    clearing: false,
    revision: 0,
    error: '',
    refreshError: '',
    sendError: '',
    sendResult: '',
    copiedUrl: '',
  }),
  getters: {
    connectionLabel(state): string {
      if (!state.status) return '正在读取状态'
      if (!state.status.running) return '未启动'
      if (!state.status.info.connected) return '等待连接'
      if (state.status.info.online === false) return 'QQ 离线'
      return '已连接'
    },
    canSend(state): boolean {
      return !!state.status?.running && state.status.info.connected
        && state.status.info.online !== false && !state.changing && !state.sending
        && /^\d+$/.test(state.targetId.trim()) && /[1-9]/.test(state.targetId.trim())
        && !!state.draft.trim()
    },
  },
  actions: {
    applyStatus(status: QQTestState, replaceEntries = false) {
      const first = this.status === null
      if (replaceEntries) {
        this.entries = status.entries ?? []
      } else {
        const byId = new Map(this.entries.map((entry) => [entry.id, entry]))
        for (const entry of status.entries ?? []) byId.set(entry.id, entry)
        this.entries = [...byId.values()].sort((a, b) => a.id - b.id).slice(-200)
      }
      this.status = status
      if (first && status.running) {
        this.config.address = status.address
        this.config.path = status.path
        this.config.selfId = status.expectedSelfId
        this.config.timeoutSeconds = status.timeoutSeconds
      }
    },
    async refresh() {
      if (this.refreshing || this.changing || this.clearing) return
      this.refreshing = true
      const revision = this.revision
      try {
        const status = await GetQQTestState(this.status?.lastId ?? 0)
        if (revision === this.revision) {
          this.applyStatus(status)
          this.refreshError = ''
        }
      } catch (err) {
        if (revision === this.revision) this.refreshError = String(err)
      } finally {
        this.refreshing = false
      }
    },
    async start() {
      if (this.changing || this.clearing) return
      this.changing = true
      this.revision++
      this.error = ''
      try {
        this.applyStatus(await StartQQTestAdapter({ ...this.config }), true)
      } catch (err) {
        this.error = String(err)
      } finally {
        this.changing = false
      }
    },
    async stop() {
      if (this.changing || this.clearing) return
      this.changing = true
      this.revision++
      this.error = ''
      try {
        this.applyStatus(await StopQQTestAdapter(), true)
      } catch (err) {
        this.error = String(err)
      } finally {
        this.changing = false
      }
    },
    async send() {
      if (!this.canSend) return
      this.sending = true
      this.sendError = ''
      this.sendResult = ''
      const text = this.draft
      try {
        const result = await SendQQTestMessage(this.targetType, this.targetId.trim(), text)
        this.sendResult = '发送成功 · 消息 ID ' + result.messageId
        if (this.draft === text) this.draft = ''
      } catch (err) {
        this.sendError = String(err)
      } finally {
        this.sending = false
        await this.refresh()
      }
    },
    async clearLog() {
      if (this.clearing || this.changing) return
      this.clearing = true
      this.revision++
      this.error = ''
      try {
        this.applyStatus(await ClearQQTestLog(), true)
      } catch (err) {
        this.error = String(err)
      } finally {
        this.clearing = false
      }
    },
    selectTarget(entry: QQTestLogEntry) {
      if (!entry.targetId || (entry.messageType !== 'private' && entry.messageType !== 'group')) return
      this.targetType = entry.messageType
      this.targetId = entry.targetId
      this.sendError = ''
      this.sendResult = ''
    },
  },
})
