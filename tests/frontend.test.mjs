import test from "node:test";
import assert from "node:assert/strict";
import { createClient, statusRows, canBeginLogin, canSendGroupText, errorMessage } from "../frontend/bridge.mjs";

function fakeBindings(overrides = {}) {
  return { core: { Core: {
    GetStatus: async () => ({ name: "CAF" }), BeginLogin: async () => ({}),
    CancelLogin: async () => ({}), SendGroupText: async () => ({}), ...overrides,
  } } };
}
test("仅接受核心的实际 Wails 绑定", () => {
  assert.throws(() => createClient(undefined), /Wails/);
  assert.throws(() => createClient({ core: { Core: { GetStatus() {} } } }), /Wails/);
  assert.throws(() => createClient({ desktop: { App: fakeBindings().core.Core } }), /Wails/);
});
test("构造和状态查询不会自动登录或发消息", async () => {
  let login = 0, sends = 0;
  const client = createClient(fakeBindings({ BeginLogin: async () => { login++; }, SendGroupText: async () => { sends++; } }));
  await client.status();
  assert.equal(login, 0); assert.equal(sends, 0);
  await client.beginLogin(); assert.equal(login, 1); assert.equal(sends, 0);
});
test("发送和取消只转发显式调用", async () => {
  const calls = [];
  const client = createClient(fakeBindings({ CancelLogin: async () => { calls.push("cancel"); }, SendGroupText: async (group, text) => { calls.push([group, text]); } }));
  await client.sendGroupText(123, "测试消息"); await client.cancelLogin();
  assert.deepEqual(calls, [[123, "测试消息"], "cancel"]);
});
test("后端错误原样交给调用方，不伪造登录成功", async () => {
  const client = createClient(fakeBindings({ BeginLogin: async () => { throw new Error("请配置签名服务"); } }));
  await assert.rejects(() => client.beginLogin(), /配置/);
});
test("显示 Go SDK 与独立的 QQ 状态，不显示旧 DLL 已加载", () => {
  const rows = statusRows({ name: "CAF", qq: { provider: "LagrangeGo", phase: "online", account: 123 } });
  assert.deepEqual(rows.find(([key]) => key === "QQ 后端"), ["QQ 后端", "LagrangeGo"]);
  assert.deepEqual(rows.find(([key]) => key === "QQ 状态"), ["QQ 状态", "已上线"]);
  assert.deepEqual(rows.at(-1), ["登录账号", "123"]);
  assert.equal(rows.some(([key]) => key === "原生库"), false);
});
test("失败时直接显示阶段、错误码和脱敏原因，不只显示笼统状态", () => {
  const rows = statusRows({ qq: { phase: "error", stage: "fetch_qr", errorCode: "SIGN_HTTP_401", error: "获取二维码失败：签名认证失败" } });
  assert.deepEqual(rows.find(([key]) => key === "失败阶段"), ["失败阶段", "获取二维码"]);
  assert.deepEqual(rows.find(([key]) => key === "错误码"), ["错误码", "SIGN_HTTP_401"]);
  assert.deepEqual(rows.find(([key]) => key === "具体原因"), ["具体原因", "获取二维码失败：签名认证失败"]);
  assert.equal(statusRows({ qq: { phase: "online" } }).some(([key]) => key === "失败阶段" || key === "错误码"), false);
});
test("登录按钮仅在就绪且未登录或失败时允许使用", () => {
  for (const phase of ["starting", "waiting", "scanned", "logging_in", "online", "closed", "unavailable"]) {
    assert.equal(canBeginLogin({ ready: true, qq: { phase } }), false);
  }
  assert.equal(canBeginLogin({ ready: true, qq: { phase: "idle" } }), true);
  assert.equal(canBeginLogin({ ready: true, qq: { phase: "error" } }), true);
  assert.equal(canBeginLogin({ ready: false, qq: { phase: "idle" } }), false);
});
test("只有已上线才允许群文本发送", () => {
  assert.equal(canSendGroupText({ ready: true, qq: { phase: "online" } }), true);
  assert.equal(canSendGroupText({ ready: true, qq: { phase: "waiting" } }), false);
  assert.equal(canSendGroupText({}), false);
});
test("支持 Error 和 Go 字符串错误", () => {
  assert.equal(errorMessage(new Error("失败")), "失败");
  assert.equal(errorMessage("失败"), "失败");
});
