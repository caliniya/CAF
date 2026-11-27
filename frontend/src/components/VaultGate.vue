<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { GetVaultStatus, CreateVault, UnlockVault, SetVaultRemembered, StartVaultSession, Logout, type VaultStatus } from '../api'
const emit = defineEmits<{ ready: [] }>()
const state = ref<VaultStatus>()
const pass = ref('')
const confirmation = ref('')
const remember = ref(false)
const busy = ref(false)
const error = ref('')
const creating = computed(() => state.value?.configured === false)
async function leave() {
  if (busy.value) return
  try { await Logout(); await check() } catch (e) { error.value = String(e) }
}
const canSubmit = computed(() => !!pass.value && (!creating.value || (
  [...pass.value].length >= 4 && pass.value === confirmation.value
)))

// Only release the main UI after both the model and QQ configs have migrated.
async function enter() {
  await StartVaultSession()
  emit('ready')
}

async function check() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    state.value = await GetVaultStatus()
    if (state.value.unlocked) await enter()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
onMounted(check)

async function submit() {
  if (!canSubmit.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    if (creating.value) await CreateVault(pass.value)
    else await UnlockVault(pass.value)
    pass.value = ''
    confirmation.value = ''
    state.value = await GetVaultStatus()
    if (remember.value) {
      try {
        await SetVaultRemembered(true)
      } catch {
        error.value = '密钥库已解锁，但系统未能记住解锁密钥。可继续使用，下次启动时需输入登录密码。'
        return
      }
    }
    await enter()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <main class="vault"><section class="card" aria-labelledby="vault-title">
    <span class="brand">CAF</span>
    <h1 id="vault-title">{{ !state ? '打开 CAF' : state.unlocked ? '准备就绪' : creating ? '创建主用户' : '登录 CAF' }}</h1>
    <p v-if="creating && !state?.unlocked">这是这份数据的第一个用户。设置密码后，可在跨平台设置中创建内部用户，并选择与他们共享的模型。</p>
    <p v-else-if="state && !state.unlocked">无需用户名。输入自己的密码即可打开自己的对话和设置。</p>
    <p v-if="state?.warning && !state.unlocked">{{ state.warning }}</p>
    <form v-if="state && !state.unlocked" @submit.prevent="submit">
      <label>登录密码<input v-model="pass" type="password" :autocomplete="creating ? 'new-password' : 'current-password'" required :disabled="busy" /></label>
      <template v-if="creating"><small>至少 4 个字符</small><label>再输入一次<input v-model="confirmation" type="password" autocomplete="new-password" required :disabled="busy" /></label>
        <small v-if="confirmation && pass !== confirmation">两次输入不一致</small></template>
      <label class="remember"><input v-model="remember" type="checkbox" :disabled="busy" />设为此设备的默认登录用户</label>
      <button class="btn btn-primary" :disabled="busy || !canSubmit">{{ busy ? '正在打开…' : creating ? '创建并进入 CAF' : '登录' }}</button>
    </form>
    <button v-else-if="!busy" class="btn btn-primary" @click="check">{{ state?.unlocked ? '继续进入 CAF' : '重试' }}</button>
    <button v-if="state?.unlocked && error" type="button" class="btn btn-ghost" :disabled="busy" @click="leave">返回登录</button>
    <p v-if="error" role="alert">{{ error }}</p>
  </section></main>
</template>
<style scoped>
.vault { height: 100%; width: 100%; display: grid; place-items: center; overflow: auto; padding: 24px; box-sizing: border-box; }
.card { width: min(100%, 400px); display: grid; gap: 14px; }
.brand { color: var(--accent); font-weight: 650; }
h1, p { margin: 0; } p, small { line-height: 1.6; color: var(--muted); }
form, label { display: grid; gap: 8px; } form { gap: 12px; }
input:not([type=checkbox]) { padding: 9px 12px; border: 1px solid var(--outline); border-radius: 8px; font: inherit; }
.remember { display: flex; align-items: center; gap: 8px; }
[role=alert] { color: var(--danger); }
</style>
