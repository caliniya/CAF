"""WSL hostname tool checks; all writes stay in a temporary directory."""

import argparse
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch


sys.dont_write_bytecode = True
script = Path(__file__).resolve().parents[1] / "tool" / "set-wsl-hostname.py"
spec = importlib.util.spec_from_file_location("wsl_hostname_tool", script)
tool = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tool)


class HostnameChecks(unittest.TestCase):
    def test_preserves_other_names_and_updates_only_managed_line(self):
        original = "# WSL hosts\n127.0.0.1 localhost\n10.1.1.1 database\n172.18.1.1 oldhost " + tool.MARKER + "\n"
        updated = tool.hosts_content(original, "winhost", "172.22.16.1")
        self.assertIn("127.0.0.1 localhost\n", updated)
        self.assertIn("10.1.1.1 database\n", updated)
        self.assertNotIn("oldhost", updated)
        self.assertIn("172.22.16.1\twinhost " + tool.MARKER + "\n", updated)
        self.assertEqual(tool.hosts_content(updated, "winhost", "172.22.16.1"), updated)
        changed = tool.hosts_content(updated, "newhost", "172.30.48.1")
        self.assertNotIn("winhost", changed)
        self.assertIn("172.30.48.1\tnewhost", changed)

    def test_does_not_replace_unmanaged_alias(self):
        for original in ("10.1.1.1 WINHOST\n", "10.1.1.1 service winhost # hand-written\n"):
            with self.subTest(original=original), self.assertRaises(RuntimeError):
                tool.hosts_content(original, "winhost", "172.22.16.1")

    def test_validates_hostname_and_interval(self):
        for name in ("", "localhost", "a.localhost", "127.0.0.1", "bad/name", "-host", "host-", "x y", "a;reboot", "a..b", "a" * 64):
            with self.subTest(name=name), self.assertRaises(argparse.ArgumentTypeError):
                tool.hostname_argument(name)
        self.assertEqual(tool.hostname_argument("WinHost"), "winhost")
        self.assertEqual(tool.hostname_argument("winhost.internal"), "winhost.internal")
        for interval in ("9", "86401", "x", "1.5"):
            with self.subTest(interval=interval), self.assertRaises(argparse.ArgumentTypeError):
                tool.positive_interval(interval)

    def test_atomic_update_preserves_mode_and_rejects_symlink(self):
        with tempfile.TemporaryDirectory() as directory:
            hosts = Path(directory) / "hosts"
            hosts.write_text("old\n")
            hosts.chmod(0o640)
            self.assertTrue(tool.write_atomic(hosts, "new\n"))
            self.assertEqual(hosts.read_text(), "new\n")
            self.assertEqual(hosts.stat().st_mode & 0o777, 0o640)
            self.assertFalse(tool.write_atomic(hosts, "new\n"))
            alias = Path(directory) / "alias"
            alias.symlink_to(hosts)
            with self.assertRaises(RuntimeError):
                tool.write_atomic(alias, "changed\n")
            self.assertEqual(hosts.read_text(), "new\n")

    def test_gateway_uses_wsl_interface_not_vpn_default(self):
        routes = json.dumps([
            {"dev": "tun0", "gateway": "10.1.1.1", "metric": 1},
            {"dev": "eth0", "gateway": "172.22.16.1", "metric": 100},
        ])
        with patch.object(tool.Path, "read_text", return_value="4.19.128-microsoft-standard"), \
                patch.object(tool.shutil, "which", side_effect=lambda name: "/usr/sbin/ip" if name == "ip" else None), \
                patch.object(tool.Path, "is_file", return_value=False), \
                patch.object(tool, "run", return_value=routes):
            self.assertEqual(tool.windows_gateway("eth0"), "172.22.16.1")
            with self.assertRaises(RuntimeError):
                tool.windows_gateway("missing")
        with patch.object(tool.Path, "read_text", return_value="microsoft-standard-WSL2"), \
                patch.object(tool.shutil, "which", return_value="/usr/bin/wslinfo"), \
                patch.object(tool, "run", return_value="mirrored\n"):
            with self.assertRaises(RuntimeError):
                tool.windows_gateway("eth0")

    def test_installation_is_self_contained_and_repeatable(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            hosts = root / "etc" / "hosts"
            hosts.parent.mkdir()
            hosts.write_text("127.0.0.1 localhost\n")
            overrides = {
                "HOSTS": hosts,
                "UPDATER": root / "usr" / "local" / "sbin" / "caf-update-windows-host",
                "SERVICE": root / "etc" / "systemd" / "system" / "caf-windows-host.service",
                "TIMER": root / "etc" / "systemd" / "system" / "caf-windows-host.timer",
                "BACKUPS": root / "backups",
            }
            # Avoid requiring an actual systemd service manager during this test.
            is_dir = Path.is_dir
            def directory_exists(path):
                return True if str(path) == "/run/systemd/system" else is_dir(path)
            with patch.multiple(tool, **overrides), patch.object(Path, "is_dir", directory_exists), \
                    patch.object(tool.shutil, "which", side_effect=lambda name: "/usr/bin/" + name), \
                    patch.object(tool, "run") as run:
                updated = tool.hosts_content(hosts.read_text(), "winhost", "172.22.16.1")
                tool.install("winhost", "eth0", 60, updated)
                self.assertEqual(overrides["UPDATER"].read_text(), script.read_text())
                service = overrides["SERVICE"].read_text()
                self.assertIn("--once --hostname winhost --interface eth0", service)
                self.assertNotIn(str(script), service)
                backups = list(overrides["BACKUPS"].iterdir())
                self.assertEqual(len(backups), 1)
                old_hosts = backups[0] / hosts.relative_to("/")
                self.assertEqual(old_hosts.read_text(), "127.0.0.1 localhost\n")
                tool.install("winhost", "eth0", 60, updated)
                self.assertEqual(len(list(overrides["BACKUPS"].iterdir())), 1)
                tool.install("newhost", "eth0", 30, tool.hosts_content(updated, "newhost", "172.30.48.1"))
                self.assertIn("--hostname newhost", overrides["SERVICE"].read_text())
                self.assertIn("OnUnitActiveSec=30s", overrides["TIMER"].read_text())
                self.assertIn("172.30.48.1\tnewhost", hosts.read_text())
                self.assertNotIn("winhost", hosts.read_text())
                self.assertTrue(any(command.args[0][1] == "restart" for command in run.call_args_list))


if __name__ == "__main__":
    unittest.main()
