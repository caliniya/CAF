# 当前补丁：适配最新提交 537d964

补丁名 `CAF-537d964-qq-fix.patch`，基于远端提交 `537d964`，本次只用这一份。

检查发现：诊断改动已应用，但账号补丁只作为 .patch 文件提交，实际 QQConfig 里尚无 qq 字段。
QQSigner.go 少了旧补丁上下文里的两行注释，导致原账号补丁检查失败；这不是 Git 对象损坏。
远端其他改动和已提交的两个补丁文件保持原样，不重写提交，不恢复用户删除的占位文档文件。

先保存本地工作，在项目根目录确认当前提交，再检查并应用：

```powershell
git status
git log -1 --oneline
git apply --check .\CAF-537d964-qq-fix.patch
git apply .\CAF-537d964-qq-fix.patch
.\test.ps1
```

不要再叠加应用旧的诊断、账号或完整版补丁。如果 --check 报错，先保留工作并查看状态，不要强行覆盖、reset --hard 或强推。

在界面显示的数据目录里编辑 `qq/config.json`，给现有 JSON 对象增加一项：

```json
"qq": 123456789
```

数字仅为示例，替换为签名服务绑定的号码，不加引号；注意与其他字段间的逗号。
不要删除原有签名地址、令牌和版本，不要把完整配置发到群里。程序不覆盖已有配置，改完再发起登录。
0 或缺省保留自动识别；配置非零时，必须使用同一 QQ 扫码，且缓存不能属于别的账号。
如遇 `QQ_CACHE_ACCOUNT_MISMATCH`，先在本机备份并移走旧 sig.bin，再扫码；程序不会替你删除。
此补丁只把配置号码用于 SDK 签名请求的 uin，不绕过服务鉴权，也没有验证真实服务的 403 已解除。

## 上一份补丁：QQ 失败诊断

基于 `01587d4`，补丁名 `CAF-qq-diagnostics.patch`。先保存自己的本地改动，在项目根目录执行：

```powershell
git apply --check .\CAF-qq-diagnostics.patch
git apply .\CAF-qq-diagnostics.patch
.\test.ps1
.\dev.ps1
```

状态面板会显示“失败阶段、错误码、具体原因”，不再只有笼统的“登录或连接失败”。
此补丁不修改签名配置、设备或登录凭据，不启用原始 SDK 认证日志，不替代真实登录测试。
如果检查报冲突，不要强制覆盖；保留现有工作后再对照合并。

## 历史迁移：LagrangeGo 桌面 QQ 试验后端（已由 01587d4 包含）

基于 `dd4f542`，补丁名 `CAF-lagrangego.patch`。包含依赖、源码、测试和许可，不包含任何账号配置。
需要 Go 1.26+。

本次会删除 Git 子模块登记和 gitlink。请在没有未保存改动的项目根目录执行，
先确保 lagrange 子模块自身没有改动，再解除工作目录初始化：

```powershell
git -C lagrange status --short
git submodule deinit lagrange
git apply --check --index .\CAF-lagrangego.patch
git apply --index .\CAF-lagrangego.patch
.\test.ps1
```

这里使用 --index，使子模块 gitlink 的删除也正确写入 Git 索引；源码改动也会一并暂存，但不会提交或推送。
如果从未初始化过 lagrange，可以跳过 deinit；未初始化目录的清理警告不影响后续补丁检查。
如果 --check 报错，不要强行覆盖，也不要运行 git clean 或 reset --hard；先保存现有工作。
旧子模块的 Git 对象缓存和历史提交没有强制删除，便于回溯。

主要变化：

- LagrangeGo 通过 Go modules 固定到 f7f8abfaa7f1；升级 Go 声明到 1.26，间接依赖按模块选择规则更新。
- 删除 Lagrange.Core 子模块、旧 native FFI 源码、goinvoke 与 NativeAOT 构建入口。
- 原 DLL、来源记录和历史补丁原样保留归档，但应用不再引用 natives 包，不会把 DLL 打进 EXE。
- DesktopQQ 换成实际 Go SDK 适配；登录在后台执行，不阻塞状态查询。
- QQPlatform 的 BeginLogin 保留；状态、取消和群文本是独立的可选接口，不破坏最小假平台测试。
- 页面改为本机扫码登录和手动发送群文本，增加 cmd/qqcheck 供 WSL 无窗口试验。
- 没有恢复 OneBot、消息事件转发或 Agent 业务；没有替用户填写签名服务或自动发测试消息。

首次运行后在数据目录 qq/config.json 本地配置签名服务。具体步骤见 README，不要将令牌或二维码发到群里。
旧数据不删除、不迁移；新的敏感文件加入 Git 忽略规则。
