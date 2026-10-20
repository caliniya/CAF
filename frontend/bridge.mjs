export function createClient(backend) {
  if (!backend || typeof backend.GetStatus !== "function" || typeof backend.LoadNativeLibrary !== "function") {
    throw new Error("请通过 Wails 启动 CAF；普通浏览器预览没有 Go 后端。");
  }
  return {
    status: () => backend.GetStatus(),
    loadNative: () => backend.LoadNativeLibrary(),
  };
}

export function statusRows(status) {
  const native = status.native ?? {};
  return [
    ["应用", `${status.name ?? "CAF"} ${status.version ?? ""}`.trim()],
    ["平台", status.platform || "尚未启动"],
    ["数据目录", status.dataDir || "尚未初始化"],
    ["原生库", native.loaded ? `已加载 · ${native.exports} 个导出` : native.supported ? "待加载" : "当前平台尚未实现"],
    ...(native.path ? [["库缓存", native.path]] : []),
  ];
}

export function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}
