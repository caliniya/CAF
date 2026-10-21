# Android 占位

目前仅保留此目录，没有 Android 入口、构建脚本、JNI 或可用原生库。桌面优先，不提前实现安卓后端。

将来可复用 `core` 的业务与 `platform` 契约，由新平台提供自己的后端和路径。不能 import 桌面实现来假装支持安卓，不能直接使用 Windows DLL；运行壳、应用私有目录、生命周期及 Android/Bionic 原生库兼容性需单独验证。
