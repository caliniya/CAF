# 开发工具

## WSL 内设置 Windows 主机名

`set-wsl-hostname.py` 在 WSL2 的 NAT 网络中，将固定名称映射到当前 Windows 网关。默认名称为 `winhost`，IP 从默认路由读取，适合 Windows 10 的 WSL2。需要 `python3`、`iproute2`；自动更新还需要正在运行的 systemd。

在 WSL 中进入项目目录后运行：

```bash
sudo python3 tool/set-wsl-hostname.py
```

也可以直接从 PowerShell 调用。下面示例的项目在 `D:\project\CAF`，新设备按实际路径调整：

```powershell
wsl.exe -d Debian -u root -- python3 /mnt/d/project/CAF/tool/set-wsl-hostname.py
```

完成后，NapCat 的反向 WebSocket 地址填写：

```text
ws://winhost:6701/onebot/v11/ws
```

端口使用 CAF 扩展实际配置的端口。此名称在 WSL 中解析，Windows 浏览器访问 NapCat WebUI 仍可使用 `http://127.0.0.1:6099/webui/`。

脚本保留 `/etc/hosts` 中其他条目，仅更新自身带标记的一行。遇到同名的手动配置会停止并提示；可重复执行。安装自动更新时，修改前的文件备份在 `/var/backups/caf-windows-host/`。安装使用的名称与之前开发环境的服务一致，会更新已有设置。

已有自动更新服务时，`--once` 不会更改服务中保存的名称。要永久换名，使用 `--hostname 新名称` 重新安装自动更新设置。

自动更新程序复制到 `/usr/local/sbin/caf-update-windows-host`，由 `caf-windows-host.service` 在启动后运行，`caf-windows-host.timer` 默认每 60 秒复查。因此安装后移动项目或关闭终端，不影响更新。脚本只配置主机名，不修改 NapCat 配置或 Windows 防火墙。

可选参数：

```bash
# 先查看检测到的网关，不修改系统。
python3 tool/set-wsl-hostname.py --dry-run

# 自定义名称、网卡与更新间隔。
sudo python3 tool/set-wsl-hostname.py --hostname winhost --interface eth0 --interval 60

# 不安装服务，仅更新一次（没有 systemd 时也可使用）。
sudo python3 tool/set-wsl-hostname.py --once

# 检查解析与自动更新服务。
getent ahostsv4 winhost
systemctl status caf-windows-host.timer --no-pager
```

若提示 systemd 未启用，可在 `/etc/wsl.conf` 的已有 `[boot]` 段中设置 `systemd=true`，保存后从 Windows 执行 `wsl --shutdown`，再打开 WSL 运行脚本。重启会停止当前 WSL 程序，安排好时机再操作。

此工具用于 WSL2 NAT；镜像网络或 WSL1 不使用 NAT 网关方案。名称能解析不代表 TCP 已放行，WSL 连接 CAF 仍需要扩展监听对应端口及 Windows 防火墙允许入站连接。

测试脚本在 `tests/wsl_hostname_test.py`，在 WSL 中执行 `python3 tests/wsl_hostname_test.py`；测试仅修改临时目录，不修改系统配置。
