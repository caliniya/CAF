#!/usr/bin/env python3
"""Expose the Windows host under a stable hostname inside WSL2 NAT."""

import argparse
import datetime
import ipaddress
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile


MARKER = "# CAF Windows host (managed)"
UPDATER = Path("/usr/local/sbin/caf-update-windows-host")
SERVICE = Path("/etc/systemd/system/caf-windows-host.service")
TIMER = Path("/etc/systemd/system/caf-windows-host.timer")
HOSTS = Path("/etc/hosts")
BACKUPS = Path("/var/backups/caf-windows-host")


def hostname_argument(value):
    labels = value.split(".")
    if len(value) > 253 or any(
        not re.fullmatch(r"[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?", label)
        for label in labels
    ):
        raise argparse.ArgumentTypeError("主机名只能包含字母、数字、短横线和分隔域名的点。")
    value = value.lower()
    if value in ("localhost", "localhost.localdomain") or value.endswith(".localhost"):
        raise argparse.ArgumentTypeError("不能将 localhost 用作 Windows 主机名称。")
    try:
        ipaddress.ip_address(value)
    except ValueError:
        return value
    raise argparse.ArgumentTypeError("请填写主机名称，不能填写 IP 地址。")


def interface_argument(value):
    if not re.fullmatch(r"[a-zA-Z0-9_.:-]{1,32}", value):
        raise argparse.ArgumentTypeError("无效的网络接口名称。")
    return value


def positive_interval(value):
    try:
        seconds = int(value)
    except ValueError:
        raise argparse.ArgumentTypeError("更新间隔必须是整数秒。") from None
    if seconds < 10 or seconds > 86400:
        raise argparse.ArgumentTypeError("更新间隔必须在 10 到 86400 秒之间。")
    return seconds


def run(command):
    result = subprocess.run(command, text=True, capture_output=True, timeout=20)
    if result.returncode:
        reason = result.stderr.strip() or result.stdout.strip()
        raise RuntimeError("执行失败：" + " ".join(command) + ("\n" + reason if reason else ""))
    return result.stdout


def windows_gateway(interface):
    kernel = Path("/proc/sys/kernel/osrelease").read_text().lower()
    if "microsoft" not in kernel or ("wsl2" not in kernel and "microsoft-standard" not in kernel):
        raise RuntimeError("本脚本需要在 WSL2 的 NAT 网络中运行。")
    # Older Windows 10 WSL releases do not provide wslinfo.
    wslinfo = shutil.which("wslinfo")
    if not wslinfo and Path("/usr/lib/wsl/wslinfo").is_file():
        wslinfo = "/usr/lib/wsl/wslinfo"
    if wslinfo:
        mode = run([wslinfo, "--networking-mode"]).strip().lower()
        if mode != "nat":
            raise RuntimeError("当前 WSL 网络模式是 " + mode + "，本脚本仅适用于 NAT。")
    ip = shutil.which("ip")
    if not ip:
        raise RuntimeError("缺少 ip 命令，请先安装 iproute2。")
    routes = json.loads(run([ip, "-j", "-4", "route", "show", "default"]))
    candidates = [entry for entry in routes if entry.get("dev") == interface and entry.get("gateway")]
    if not candidates:
        raise RuntimeError("未找到 " + interface + " 的 IPv4 默认网关，请检查 WSL NAT 网络。")
    route = min(candidates, key=lambda entry: entry.get("metric", 0))
    address = ipaddress.IPv4Address(route["gateway"])
    if address.is_loopback or address.is_multicast or address.is_unspecified:
        raise RuntimeError("默认网关地址无效。")
    return str(address)


def hosts_content(original, hostname, address):
    """Preserve unrelated entries; replace only this tool's managed line."""
    lines = []
    for line in original.splitlines():
        if "#" in line and "#" + line.split("#", 1)[1] == MARKER:
            continue
        fields = line.split("#", 1)[0].split()
        if hostname in [alias.lower() for alias in fields[1:]]:
            raise RuntimeError("/etc/hosts 已有非本脚本管理的 " + hostname + " 条目，请先检查该条目。")
        lines.append(line)
    return "\n".join(lines).rstrip("\n") + "\n" + address + "\t" + hostname + " " + MARKER + "\n"


def write_atomic(path, content, mode=0o644):
    """Preserve existing ownership and permissions while replacing a complete file."""
    if path.is_symlink():
        raise RuntimeError("拒绝覆盖符号链接：" + str(path))
    encoded = content.encode("utf-8")
    if path.exists() and path.read_bytes() == encoded:
        return False
    path.parent.mkdir(parents=True, exist_ok=True)
    previous = path.stat() if path.exists() else None
    with tempfile.NamedTemporaryFile(dir=path.parent, prefix=".caf-host-", delete=False) as file:
        temporary = Path(file.name)
        file.write(encoded)
        file.flush()
        os.fsync(file.fileno())
    try:
        temporary.chmod(previous.st_mode & 0o777 if previous else mode)
        if previous:
            os.chown(temporary, previous.st_uid, previous.st_gid)
        temporary.replace(path)
    finally:
        temporary.unlink(missing_ok=True)
    return True


def unit_contents(hostname, interface, interval, python):
    service = """[Unit]
Description=Resolve Windows hostname from the WSL NAT gateway
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
ExecStart={python} {updater} --once --hostname {hostname} --interface {interface}
Restart=on-failure
RestartSec=5
TimeoutStartSec=30

[Install]
WantedBy=multi-user.target
""".format(python=python, updater=UPDATER, hostname=hostname, interface=interface)
    timer = """[Unit]
Description=Refresh the Windows hostname when the WSL gateway changes

[Timer]
OnBootSec=5s
OnUnitActiveSec={interval}s
Unit=caf-windows-host.service

[Install]
WantedBy=timers.target
""".format(interval=interval)
    return service, timer


def install(hostname, interface, interval, updated_hosts):
    systemctl = shutil.which("systemctl")
    if not systemctl or not Path("/run/systemd/system").is_dir():
        raise RuntimeError(
            "自动更新需要 systemd。请在 /etc/wsl.conf 的 [boot] 中启用 systemd=true，"
            "然后在 Windows 执行 wsl --shutdown 并重新打开 WSL；或者加 --once 只更新一次。"
        )
    python = shutil.which("python3")
    if not python or not re.fullmatch(r"/[a-zA-Z0-9_./-]+", python):
        raise RuntimeError("无法确定 systemd 可以调用的 python3 路径。")
    source = Path(__file__).read_text(encoding="utf-8")
    service, timer = unit_contents(hostname, interface, interval, python)
    files = [(HOSTS, updated_hosts, 0o644), (UPDATER, source, 0o755), (SERVICE, service, 0o644), (TIMER, timer, 0o644)]
    for path, _, _ in files:
        if path.is_symlink():
            raise RuntimeError("拒绝覆盖符号链接：" + str(path))
    changes = [(path, content, mode) for path, content, mode in files if not path.exists() or path.read_text(encoding="utf-8") != content]
    backup = None
    if changes:
        BACKUPS.mkdir(parents=True, exist_ok=True, mode=0o700)
        stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S-")
        backup = Path(tempfile.mkdtemp(prefix=stamp, dir=BACKUPS))
        for path, _, _ in changes:
            if path.exists():
                destination = backup / path.relative_to("/")
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(path, destination)
                destination.chmod(0o600)
    try:
        # Avoid an old timer or service racing with installation.
        existing = [path.name for path in (TIMER, SERVICE) if path.exists()]
        if changes and existing:
            run([systemctl, "stop", *existing])
        for path, content, mode in changes:
            write_atomic(path, content, mode)
        run([systemctl, "daemon-reload"])
        run([systemctl, "enable", SERVICE.name, TIMER.name])
        run([systemctl, "restart", SERVICE.name, TIMER.name])
    except (OSError, RuntimeError, subprocess.SubprocessError) as error:
        location = "\n原文件备份：" + str(backup) if backup else ""
        raise RuntimeError("自动更新安装失败：" + str(error) + location) from error
    print("已安装自动更新：WSL 启动后更新，每 " + str(interval) + " 秒复查。")
    print("手动更新：sudo " + str(UPDATER) + " --once --hostname " + hostname + " --interface " + interface)
    if backup:
        print("原文件备份：" + str(backup))


def main():
    parser = argparse.ArgumentParser(description="在 WSL2 NAT 中设置固定的 Windows 主机名，默认 winhost。")
    parser.add_argument("--hostname", type=hostname_argument, default="winhost", help="主机名（默认 winhost）")
    parser.add_argument("--interface", type=interface_argument, default="eth0", help="WSL NAT 网络接口（默认 eth0）")
    parser.add_argument("--interval", type=positive_interval, default=60, help="自动更新间隔，单位秒（默认 60）")
    parser.add_argument("--once", action="store_true", help="只更新本次 /etc/hosts，不安装或修改自动更新服务")
    parser.add_argument("--dry-run", action="store_true", help="预览网关和名称，不修改任何文件")
    args = parser.parse_args()
    if not args.dry_run and os.geteuid() != 0:
        raise RuntimeError("修改 /etc/hosts 需要 root，请用 sudo python3 运行本脚本。")
    address = windows_gateway(args.interface)
    updated = hosts_content(HOSTS.read_text(encoding="utf-8"), args.hostname, address)
    print(args.hostname + " -> " + address)
    if args.dry_run:
        print("预览完成，没有修改文件。")
    elif args.once:
        changed = write_atomic(HOSTS, updated)
        print("已更新 /etc/hosts。" if changed else "/etc/hosts 已是最新。")
    else:
        install(args.hostname, args.interface, args.interval, updated)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        print("错误：" + str(error), file=sys.stderr)
        sys.exit(1)
