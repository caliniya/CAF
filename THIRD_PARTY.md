# 第三方来源

- Wails v2.17.0：https://github.com/wailsapp/wails （MIT）
- goinvoke v1.3.3：https://github.com/jamesits/goinvoke （MIT）
- Lagrange.Core：Git 子模块 `lagrange/`，固定提交 `20c2ba079b5ee67156bd71419ed54ea61a24e91f`。
- 内置 NativeAPI DLL 来自用户现有 CAF 仓库，哈希与来源见 `natives/PROVENANCE.json`；本次未重新编译 Windows DLL。

Wails 和 goinvoke 的原始 MIT 许可文本保留在 `LICENSES/`。间接 Go 依赖的具体版本见 `go.mod/go.sum`，其许可随各自上游模块提供。

请保留第三方来源和许可信息。固定版本 Lagrange.Proto / Lagrange.Proto.Generator 的项目文件声明 `GPL-3.0-or-later`，Lagrange 根目录未发现单独 LICENSE；不能据此把所有内容统一视为 MIT。发布组合程序前需要核对第三方授权及相关要求。

本脚手架没有替用户决定项目自身许可证。以上是来源记录，不是法律意见。
