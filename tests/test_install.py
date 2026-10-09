import pathlib
import subprocess
import unittest

import tempfile
import os

ROOT = pathlib.Path(__file__).resolve().parents[1]

class InstallerTests(unittest.TestCase):
    def test_help_exits_without_building(self):
        result = subprocess.run(['bash', str(ROOT / 'scripts/install.sh'), '--help'], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('--binary', result.stdout)
        self.assertIn('--hermes', result.stdout)

    def test_binary_deployment_copies_executable(self):
        # Only OS dependency discovery is stubbed: this runner has no user bus.
        with tempfile.TemporaryDirectory(prefix='.install-test-', dir=ROOT) as directory:
            root = pathlib.Path(directory)
            tools = root / 'tools'
            tools.mkdir()
            for name in ['labwc', 'grim', 'wtype', 'ffmpeg', 'systemctl', 'systemd-run']:
                executable = tools / name
                executable.write_text('#!/bin/sh\nexit 0\n')
                executable.chmod(0o755)
            source = root / 'cu-source'
            source.write_text('#!/bin/sh\nprintf "verified-cu\\n"\n')
            source.chmod(0o755)
            destination = root / 'installed'
            env = dict(os.environ, PATH=str(tools) + ':' + os.environ['PATH'],
                       XDG_RUNTIME_DIR=directory, CU_INSTALL_DIR=str(destination))
            result = subprocess.run(['bash', str(ROOT / 'scripts/install.sh'), '--binary', str(source)],
                                    env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((destination / 'cu').read_bytes(), source.read_bytes())
            self.assertTrue(os.access(destination / 'cu', os.X_OK))

    def test_unknown_option_is_rejected(self):
        result = subprocess.run(['bash', str(ROOT / 'scripts/install.sh'), '--unknown'], capture_output=True)
        self.assertEqual(result.returncode, 2)

if __name__ == '__main__':
    unittest.main()
