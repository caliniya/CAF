import test from "node:test";
import assert from "node:assert/strict";
import { createClient, statusRows, canLoadNative, errorMessage } from "../frontend/bridge.mjs";

test("requires an actual Wails backend instead of pretending browser preview works", () => {
  assert.throws(() => createClient(undefined), /Wails/);
  assert.throws(() => createClient({ core: { Core: { GetStatus() {} } } }), /Wails/);
  assert.throws(() => createClient({ main: { App: { GetStatus() {}, LoadNativeLibrary() {}, BeginLogin() {} } } }), /Wails/);
});

test("forwards methods without automatically loading native code", async () => {
  let loads = 0;
  const client = createClient({
    core: {
      Core: {
        GetStatus: async () => ({ name: "CAF" }),
        LoadNativeLibrary: async () => { loads++; return { native: { loaded: true } }; },
        BeginLogin: async () => ({ name: "CAF" }),
      },
    },
  });
  assert.equal(loads, 0);
  assert.deepEqual(await client.status(), { name: "CAF" });
  assert.equal(loads, 0);
  assert.equal((await client.loadNative()).native.loaded, true);
  assert.equal(loads, 1);
});

test("forwards login and preserves an explicit not-implemented error", async () => {
  let calls = 0;
  const backend = {
    GetStatus: async () => ({}),
    LoadNativeLibrary: async () => ({}),
    BeginLogin: async () => { calls++; throw new Error("QQ login is not implemented"); },
  };
  const client = createClient({ core: { Core: backend } });
  assert.throws(() => createClient({ desktop: { App: backend } }), /Wails/);
  assert.equal(calls, 0);
  await assert.rejects(() => client.beginLogin(), /not implemented/);
  assert.equal(calls, 1);
});

test("renders native availability, paths and count as data", () => {
  const rows = statusRows({ name: "CAF", version: "0.1.0", platform: "windows/amd64", dataDir: "D:/CAF/data", native: { loaded: true, exports: 6, path: "native.dll" } });
  assert.deepEqual(rows[0], ["应用", "CAF 0.1.0"]);
  assert.equal(rows.find(([key]) => key === "原生库")[1], "已加载 · 6 个导出");
  assert.deepEqual(rows.at(-1), ["库缓存", "native.dll"]);
  assert.match(statusRows({}).find(([key]) => key === "原生库")[1], /尚未实现/);
});

test("handles both Error objects and string rejections from Go", () => {
  assert.equal(errorMessage(new Error("failed")), "failed");
  assert.equal(errorMessage("native load failed"), "native load failed");
});

test("暂停优先于平台支持状态，Windows 与 Linux 都不显示待加载", () => {
  for (const supported of [true, false]) {
    const status = { ready: true, native: { supported, paused: true, loaded: false } };
    assert.equal(statusRows(status).find(([key]) => key === "原生库")[1], "QQ 集成已暂停 · 不加载");
    assert.equal(canLoadNative(status), false);
  }
});

test("加载按钮同时检查就绪、支持、暂停及加载状态", () => {
  assert.equal(canLoadNative({}), false);
  assert.equal(canLoadNative({ ready: false, native: { supported: true } }), false);
  assert.equal(canLoadNative({ ready: true, native: { supported: false } }), false);
  assert.equal(canLoadNative({ ready: true, native: { supported: true, loaded: true } }), false);
  assert.equal(canLoadNative({ ready: true, native: { supported: true, paused: false, loaded: false } }), true);
});
