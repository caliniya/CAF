<template>
  <section class="card stack page-scroll">
    <p class="description">通用设置（占位内容，用于界面演示，暂不持久化）。</p>

    <div class="field">
      <label for="set-theme">主题</label>
      <select id="set-theme">
        <option>浅色</option>
        <option>深色</option>
      </select>
    </div>

    <div class="field">
      <label for="set-lang">界面语言</label>
      <select id="set-lang">
        <option>中文</option>
        <option>English</option>
      </select>
    </div>

    <div class="stack">
      <label class="check-line">
        <input type="checkbox" checked />
        启动时恢复上次的对话
      </label>
      <label class="check-line">
        <input type="checkbox" />
        对话完成时发送系统通知
      </label>
      <label class="check-line">
        <input type="checkbox" checked />
        工具调用前需要确认
      </label>
    </div>

    <hr class="divider" />

    <div class="field">
      <label for="set-datadir">数据目录</label>
      <input id="set-datadir" type="text" value="D:\Program\CAF\data" disabled />
      <p class="field-hint">便携模式：数据目录位于程序目录下，可用 CAF_DATA_DIR 环境变量覆盖。</p>
    </div>
  </section>
</template>
