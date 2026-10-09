import pathlib
import subprocess
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]

class InstallerTests(unittest.TestCase):
    def test_help_exits_without_building(self):
        result = subprocess.run(['bash', str(ROOT / 'scripts/install.sh'), '--help'], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('--binary', result.stdout)
        self.assertIn('--hermes', result.stdout)

if __name__ == '__main__':
    unittest.main()
