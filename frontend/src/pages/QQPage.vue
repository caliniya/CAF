<script setup lang="ts">
import { MessageCircle } from '@lucide/vue'
</script>

<template>
  <section class="qq-page" aria-labelledby="qq-page-title">
    <header class="qq-head">
      <h1 id="qq-page-title">QQ</h1>
      <p>消息与会话</p>
    </header>
    <div class="qq-empty">
      <span class="qq-empty-icon" aria-hidden="true">
        <MessageCircle :size="28" :stroke-width="1.4" />
      </span>
      <h2>暂无 QQ 会话</h2>
      <p>QQ 消息将在这里显示。</p>
    </div>
  </section>
</template>

<style scoped>
.qq-page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.qq-head {
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

.qq-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  flex: 1;
  padding: 32px 24px;
  text-align: center;
}

.qq-empty-icon {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  margin-bottom: 4px;
  border-radius: 16px;
  background: var(--tonal);
  color: var(--accent);
}

.qq-empty h2 {
  font-size: 15px;
  font-weight: 500;
}

.qq-empty p {
  color: var(--muted);
  font-size: 12px;
}
</style>
