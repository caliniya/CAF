# 剩余测试

测试仍集中放在 tests/，不在业务包中散放测试。

- paths_test.go / helpers_test.go：纯路径解析与校验，不初始化业务、不创建数据目录。
- development.test.ps1：7 次开发脚本模拟及 VS Code 配置检查，不启动真实窗口或网络服务。

```powershell
.\test.ps1
go test -race -count=1 ./tests
go test -tags bindings -count=1 ./...
```

旧 Core、QQ 封装、原生资源提取和演示页面已删除，其专用测试也一并移除。
当前没有前端脚本，无需 Node 测试；没有替尚未定义的新接口编写实现或测试。
