# CAF
# caliniya's agent frame

CAF 是 Go/Wails 制作的 Agent 框架软件。

目前支持 OneBot 11 反向 WebSocket集成
未来计划实现内置编辑器
计划实现插件等扩展功能

开发辅助脚本放在 [tool](tool/README.md)，包括 WSL 中固定 Windows 主机名的配置工具。

## GitHub Actions

每次 push（包括 tag）和 Pull Request 都会运行 CI：构建前端、运行 `go vet` 和无界面 Go 测试，并分别编译 Windows、Linux 和 macOS 桌面程序。Go 和 Wails 版本读取 `go.mod`，前端使用 Node.js 24 和 `npm ci`。

发行 tag 的数字位数不限，但数字段数、平台名称和后缀必须符合下表：

| Tag 规则 | 示例 | 编译目标 |
| --- | --- | --- |
| `v数字.数字` | `v0.4`、`v01.02` | Windows / Linux / macOS，各自 amd64 和 arm64，共 6 个 |
| `v数字.数字-all`（也支持直接追加 `all`） | `v0.4-all`、`v0.4all` | 上述 6 个，加 Windows 386 和 Linux ARMv7，共 8 个 |
| `v数字.数字.数字-windows` | `v0.4.1-windows` | Windows amd64 和 arm64 |
| `v数字.数字.数字-linux` | `v0.4.1-linux` | Linux amd64 和 arm64 |
| `v数字.数字.数字-mac` | `v0.4.1-mac` | macOS amd64 和 arm64 |

`amd64` 表示 x86-64。当前 Wails 不支持 Linux 386 或 macOS 32 位；Linux ARMv7 使用硬浮点 ABI（armhf / GOARM=7）。`v0.02linux`、`v0.4-linux`、`v0.4.1` 等不符合规则的 tag 不会发行。

所有选中的目标编译和打包成功后，工作流会创建或更新**触发 tag 对应的 GitHub Release**，上传 Windows/macOS 的 ZIP、Linux 的 tar.gz，以及 `SHA256SUMS.txt`。压缩包包含程序、`LICENSE` 和 `LICENSES`。重新运行时会替换同名附件，不需要额外配置发布 token。

先将工作流提交到仓库，再创建并推送 tag。例如：

```sh
git tag v0.4
git push origin v0.4

git tag v0.4.1-linux
git push origin v0.4.1-linux

git tag v0.5-all
git push origin v0.5-all
```

Linux 产物基于 Ubuntu 24.04，运行环境需要 GTK 3 和 WebKitGTK 4.1（Ubuntu 对应 `libgtk-3-0t64`、`libwebkit2gtk-4.1-0`）。macOS 产物使用 Wails 的临时自签名，未进行 Apple 公证。
