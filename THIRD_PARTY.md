# 第三方来源

- Wails v2.17.0：https://github.com/wailsapp/wails （MIT），许可保留在 LICENSES/。
- LagrangeGo：Go 模块版本 v0.1.5-0.20260718062712-f7f8abfaa7f1，来源 https://github.com/LagrangeDev/LagrangeGo 。
  使用 AGPL-3.0，原始许可文本保留在 LICENSES/LagrangeGo-AGPL-3.0.txt。
- 间接 Go 依赖的版本和校验值见 go.mod/go.sum，其许可随对应模块提供。

本轮保留 LagrangeGo 的模块声明和许可，但 QQ 适配代码已删除，当前空窗口入口不导入该 SDK。

Lagrange.Core Git 子模块已移除，不再参与当前构建；旧 goinvoke 也不再是 Go 模块依赖。
保留的 goinvoke MIT 文本仅用于历史来源记录。
原 Windows NativeAPI DLL 继续归档于 backend/desktop/natives，内容与 PROVENANCE.json 未变，应用不引用它。
历史编译补丁仍保留供溯源，不是当前构建步骤。归档不等于替第三方授予新的许可。

请保留来源和许可信息。LagrangeGo 不是 MIT 依赖；发布组合程序前应核对适用授权要求。
本项目尚未替用户决定 CAF 自身许可证；这里是来源记录，不是法律意见。
