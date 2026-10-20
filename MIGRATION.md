# 本次迁移

本次合并补丁基于上游提交 `0b3aec23b44fe667862b44a025902f2e3ab7aef8`，已经包含之前“测试集中到 tests/”的修改。**未应用旧测试补丁时，只应用这次的新补丁，不需要先应用旧补丁。**

```powershell
git apply --check CAF-core-platform.patch
git apply CAF-core-platform.patch
.\test.ps1
.\build.ps1
```

如已有其他本地改动，先保存并使用 `--check` 检查；不要强制覆盖。新补丁没有提交或推送到远程。

主要变化：业务迁入 `core/`；平台契约在 `core/platform/`；桌面后端在 `platforms/desktop/`；Runtime 统一持有路径与实现；Go 测试集中在 `tests/`。根目录 App 移为桌面门面，JS 命名空间从 `main.App` 改为 `desktop.App`，前端桥接同步更新。

现有 DLL 只是从 `natives/win-x64/` 原样移动至 `platforms/desktop/natives/win-x64/`，没有重新编译或改变内容。不要使用旧的构建脚本往原路径复制 DLL。

真实 QQ 登录并未新增：只提供平台接口和转发链路，桌面实现明确返回“尚未实现”。安卓仍只有占位说明。
