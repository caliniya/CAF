# 本次结构变更

基于 CAF 最新提交 `bc845bf`，补丁名 `CAF-structure.patch`。
本补丁已经包含之前尚未合入仓库的“暂停 QQ 加载”改动，不需要先应用旧补丁。

在干净的项目根目录执行（补丁先放到根目录）：

```powershell
git apply --check .\CAF-structure.patch
git apply .\CAF-structure.patch
.\test.ps1
.\build.ps1
```

本地有其他改动时先保存；如果 --check 报错，不要强行应用。
如果已经应用旧的暂停补丁，也先检查，不要重复叠加；需要先保存现有工作再整理到上述基线。

## 路径变更

- core/platform → platform。
- platforms/desktop → backend/desktop，原 window、native 和 natives 子目录一起迁移。
- platforms/android → backend/android，仍为占位。
- 新建 client、pre 的包说明，暂不实现网络、解析或工具执行。
- 原生 DLL 只移动路径，Git 补丁按重命名记录，不重编、不替换内容。
- main、命令行、测试、原生重编脚本和来源文档同步新路径。

## QQ 引用单独注入

桌面基础能力由 DesktopBackend 提供，QQ 占位由 DesktopQQ.go 中的 DesktopQQ 提供。
入口从 backend.QQ() 取得同一个占位引用，显式交给核心：

```go
core.Platform, err = core.NewCore(paths, backend, backend.QQ())
```

独立调用 NewCore 的代码需要补上第三个参数；没有 QQ 能力时传 nil。
不再由核心通过类型断言猜测基础后端是否兼任 QQ；调用方仍走 core.Platform.BeginLogin()。
QQ 占位与后端共用生命周期，暂停、缺失或关闭都返回明确错误，不阻止普通核心启动。

Wails 仍绑定 core.Core；前端命名空间、数据位置和 Windows F5 配置不变。
本次不创建 desktop.App、不改 Lagrange 子模块或 DLL 内容、不删除或迁移用户数据，也不提交或推送代码。
