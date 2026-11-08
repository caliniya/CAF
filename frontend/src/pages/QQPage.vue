<template>
  <section class="qq-page" aria-labelledby="qq-page-title">
    <header class="qq-head">
      <h1 id="qq-page-title">QQ</h1>
    </header>
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
</style>
