# 集中测试目录

全部自有 Go 测试使用 package tests 和公开接口；没有单独测试 module，也不在业务目录放测试文件。

| 文件 | 内容 |
| --- | --- |
| helpers_test.go | 假平台及独立核心构造，不调用真实 QQ 登录 |
| core_test.go | 构造、路径副本、初始化、错误重试、关闭和并发 |
| core_ui_test.go | 页面直接调用核心、全局同一实例、上下文取消、带类型 nil、缺失 QQ 能力和启动重试 |
| backend_test.go | 桌面基础后端及其 QQ 引用，目录不可变、加载和登录暂停、关闭保护 |
| qq_test.go | 显式注入独立 QQ、禁止隐式推断、带类型 nil、DesktopQQ 占位及稳定引用 |
| desktop_test.go | 真实桌面后端交给核心启动，不开窗口、不加载或登录 |
| paths_test.go | 桌面便携路径、相对覆盖和非法路径 |
| natives_test.go | 历史 DLL 归档的来源、缓存代码和哈希，应用不依赖这个资源包 |
| lagrangego_test.go | 离线 SDK 对象、稳定私有设备、假会话扫码/缓存登录、取消、发送、错误脱敏及不假报上线 |
| qq_diagnostic_test.go | 本机 httptest 假签名服务、实际 SDK 请求格式、HTTP/code/空签名错误、分阶段诊断与脱敏 |
| qq_account_test.go | 配置 qq 的范围、扫码前签名 uin、账号一致性、不匹配缓存离线拒绝及不伪造上线身份 |
| frontend.test.mjs | core.Core 命名空间、转发、暂停状态及禁用加载、登录错误；拒绝旧 desktop.App 命名空间 |
| development.test.ps1 | 7 次模拟开发脚本运行和 VS Code 配置，无窗口、不依赖 Pester |

根目录运行：

```powershell
.\test.ps1
go test -count=1 ./tests
go test -race -count=1 ./tests
go test -tags bindings -count=1 ./...
node --test tests/frontend.test.mjs
```

全部测试已改用 platform 和 backend/desktop 新路径。
核心与后端测试不导入 Wails 窗口包，无需 GTK。整个工程在无窗口环境用 bindings 标签做检查。
全局入口测试只在单个顺序测试里模拟 main 装配，并在结束后恢复全局变量；其他测试使用独立 Core，不修改全局状态。
测试助手可将同一个假对象显式传入基础后端与 QQ 两个参数；生产入口传入独立的 backend.QQ()。
假平台登录返回成功只验证调用路由；真实桌面在缺少签名配置时明确报错，但核心仍正常就绪；旧加载入口返回 ErrNativeRetired。
旧 FFI 装载测试已移除；QQ 流程使用假会话，绝不访问真实 QQ 或外部签名服务。真实 SDK 的对象构造测试不登录；签名 Provider 只请求测试创建的本机模拟 HTTP 服务，使用虚构数据。
资源提取测试仍只写测试临时目录，验证保留的 DLL 和缓存代码，不调用系统加载器。

覆盖率可用：

```powershell
go test -count=1 -coverpkg=./core/...,./backend/desktop,./backend/desktop/natives ./tests
```

Windows 窗口、VS Code/Delve 断点和真实登录仍需在相应环境单独验证。

假会话的 online 只验证 CAF 的编排，不代表真实 QQ 能登录。需要真实服务的验证只能在本机另行执行。
