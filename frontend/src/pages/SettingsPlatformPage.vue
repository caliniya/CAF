<script setup lang="ts">
import { computed, onMounted, ref, useId } from 'vue'
import { ChangePassword, ExportUserData, GetVaultStatus, Logout, SetVaultRemembered, type VaultStatus } from '../api'
import ConfirmationDialog from '../components/ConfirmationDialog.vue'
import ExtensionWindow from '../components/ExtensionWindow.vue'
import InternalUsersSettings from '../components/InternalUsersSettings.vue'

const profile = ref<VaultStatus>(), mode = ref<'change' | 'export' | null>(null)
const current = ref(''), next = ref(''), repeat = ref(''), include = ref(false)
const busy = ref(false), signingOut = ref(false), error = ref(''), notice = ref(''), pickerNotice = ref('')
const formId = useId()
const canSave = computed(() => !!current.value && (mode.value === 'export' ||
  ([...next.value].length >= 4 && next.value === repeat.value && current.value !== next.value)))
function show(value: typeof mode.value) {
  mode.value = value; current.value = next.value = repeat.value = error.value = notice.value = pickerNotice.value = ''; include.value = false
}
onMounted(async () => {
  busy.value = true
  try { profile.value = await GetVaultStatus(); notice.value = profile.value.warning ?? '' }
  catch (e) { error.value = String(e) } finally { busy.value = false }
})
async function setDefault() {
  if (busy.value || !profile.value) return
  const on = !profile.value.remembered; busy.value = true; error.value = notice.value = ''
  try { await SetVaultRemembered(on); profile.value.remembered = on }
  catch (e) { error.value = String(e) } finally { busy.value = false }
}
async function submit() {
  if (!canSave.value || busy.value) return
  busy.value = true; error.value = pickerNotice.value = ''
  try {
    if (mode.value === 'change') { await ChangePassword(current.value, next.value); show(null); notice.value = '密码已更改。下次手动登录或换机时请使用新密码。' }
    else if (mode.value === 'export') {
      const file = await ExportUserData(current.value, include.value)
      if (!file) { pickerNotice.value = '未选择目录，没有导出文件。'; return }
      show(null); notice.value = '已导出到：' + file
    }
  } catch (e) { error.value = String(e) } finally { busy.value = false }
}
async function signOut() {
  if (busy.value) return
  busy.value = true; error.value = ''
  try { await Logout(); window.location.hash = '/chat'; window.location.reload() }
  catch (e) { error.value = String(e); busy.value = false }
}
</script>
<template>
  <section class="page"><header><h1>跨平台设置</h1><p v-if="profile">当前身份：{{ profile.role === 'owner' ? '主用户' : '内部用户' }}</p></header>
    <InternalUsersSettings v-if="profile?.role === 'owner'" />
    <div class="row"><div><h2>默认登录用户</h2><p>正常关闭后，下次在此设备上自动进入当前用户。主动退出后需重新输入该用户的密码。</p></div>
      <label class="option"><input type="checkbox" :checked="!!profile?.remembered" :disabled="busy || !profile" @click.prevent="setDefault" />在此设备启用</label></div>
    <div class="row"><div><h2>登录密码</h2><p>改密不会影响已有对话或模型权限。</p></div><button class="btn" :disabled="busy" @click="show('change')">更改密码</button></div>
    <div class="row"><div><h2>导出当前用户的数据</h2><p>保存可直接阅读的 ZIP，包括自己的对话、记忆、配置和内部工作区。</p></div><button class="btn" :disabled="busy" @click="show('export')">导出原始数据</button></div>
    <div class="row"><div><h2>当前会话</h2><p>退出后，其他人可以用自己的密码登录。</p></div><button class="btn" :disabled="busy" @click="error = ''; signingOut = true">退出登录</button></div>
    <p v-if="notice" role="status">{{ notice }}</p><p v-if="error && !signingOut && !mode" role="alert">{{ error }}</p>
    <ExtensionWindow :open="!!mode" :title="mode === 'export' ? '导出当前用户的数据' : '更改登录密码'" :busy="busy" @close="show(null)">
      <form v-if="mode" :id="formId" @submit.prevent="submit">
        <p v-if="mode === 'export'">ZIP 没有密码保护。外部工作区只导出路径，不复制其中的文件。</p>
        <label class="field">当前密码<input v-model="current" type="password" autocomplete="current-password" :disabled="busy" required /></label>
        <template v-if="mode === 'change'"><label class="field">新密码（至少 4 个字符）<input v-model="next" type="password" autocomplete="new-password" :disabled="busy" required /></label>
          <label class="field">再次输入新密码<input v-model="repeat" type="password" autocomplete="new-password" :disabled="busy" required /></label>
          <p v-if="repeat && repeat !== next">两次输入的新密码不一致。</p></template>
        <label v-else class="option"><input v-model="include" type="checkbox" :disabled="busy" /><span>{{ profile?.role === 'owner' ? '同时导出明文模型 API Key 和 QQ Token' : '同时导出自己的明文 QQ Token' }}</span></label>
        <p v-if="pickerNotice" role="status">{{ pickerNotice }}</p><p v-if="error" role="alert">{{ error }}</p>
      </form>
      <template #footer><button class="btn" type="button" :disabled="busy" @click="show(null)">取消</button><button class="btn btn-primary" type="submit" :form="formId" :disabled="busy || !canSave">{{ busy ? '处理中…' : mode === 'export' ? '选择目录并导出' : '保存新密码' }}</button></template>
    </ExtensionWindow>
    <ConfirmationDialog :open="signingOut" title="退出当前用户？" message="进行中的对话和 QQ 连接会停止。已保存的记录会保留；即使重开 CAF，也需要重新输入密码才能进入当前用户。" confirm-label="退出登录" :busy="busy" :error="error" @cancel="signingOut = false; error = ''" @confirm="signOut" />
  </section>
</template>
<style scoped>
.page { box-sizing: border-box; flex: 1; min-width: 0; overflow: auto; min-height: 0; padding: 26px max(24px, calc((100% - 680px)/2)); }
header { margin-bottom: 14px; } h1 { font-size: 21px; } h2 { font-size: 14px; } h1,h2,p { margin: 0; } p { color: var(--muted); font-size: 13px; line-height: 1.5; margin-top: 5px; overflow-wrap: anywhere; }
.row { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px 18px; padding: 18px 0; border-bottom: 1px solid var(--outline); } .row > div { flex: 1 1 240px; } .row > :last-child { flex-shrink: 0; }
form, .field { display: grid; gap: 6px; } form { gap: 14px; } .field, .option { font-size: 13px; } .field input { box-sizing: border-box; width: 100%; padding: 8px 10px; border: 1px solid var(--outline); border-radius: 6px; color: var(--ink); background: var(--surface); } .option { display: flex; align-items: center; gap: 8px; } .option input { flex: 0 0 auto; margin: 0; } [role=alert] { color: var(--danger); }
</style>
