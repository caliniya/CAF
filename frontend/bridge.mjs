export function createClient(bindings) {
  // 页面只访问核心对象，不接触 SDK 客户端、签名配置或凭据。
  const backend = bindings?.core?.Core;
  if (!backend || !["GetStatus", "BeginLogin", "CancelLogin", "SendGroupText"].every((name) => typeof backend[name] === "function")) {
    throw new Error("请通过 Wails 启动 CAF；普通浏览器预览没有 Go 后端。");
  }
  return {
    status: () => backend.GetStatus(),
    beginLogin: () => backend.BeginLogin(),
    cancelLogin: () => backend.CancelLogin(),
    sendGroupText: (group, text) => backend.SendGroupText(group, text),
  };
}

export const phaseNames = {
  idle: "未登录", starting: "正在创建会话", waiting: "等待扫码",
  scanned: "已扫码，等待手机确认", logging_in: "正在登录", online: "已上线",
  error: "登录或连接失败", closed: "已关闭", unavailable: "当前后端未提供 QQ",
};

export function statusRows(status) {
  const qq = status.qq ?? {};
  return [
    ["应用", `${status.name ?? "CAF"} ${status.version ?? ""}`.trim()],
    ["平台", status.platform || "尚未启动"],
    ["数据目录", status.dataDir || "尚未初始化"],
    ["QQ 后端", qq.provider || "尚未提供"],
    ["QQ 状态", phaseNames[qq.phase] || "未知状态"],
    ...(qq.account ? [["登录账号", String(qq.account)]] : []),
  ];
}

export function canBeginLogin(status) {
  return Boolean(status.ready && ["idle", "error"].includes(status.qq?.phase));
}
export function canSendGroupText(status) {
  return Boolean(status.ready && status.qq?.phase === "online");
}
export function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}
