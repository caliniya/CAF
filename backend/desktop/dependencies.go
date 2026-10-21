//go:build caf_deps

package desktopBackend

// 这里只保留后续要使用的 QQ SDK 依赖，不提供平台实现。
// Wails 会自动执行 go mod tidy；tidy 会计入本文件的导入，避免误删依赖。
// 普通构建不启用 caf_deps，因此不会编译或初始化这个 SDK。
// 正式的桌面 QQ 后端开始导入 client 包后，就可以删除本文件。
import _ "github.com/LagrangeDev/LagrangeGo/client"
