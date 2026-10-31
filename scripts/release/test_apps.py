import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import apps


class InstallerSafetyTests(unittest.TestCase):
    def test_unusable_release_endpoints_are_rejected(self):
        for endpoint in ('', 'http://api.example.com', 'https://localhost:8080',
                         'https://127.0.0.1', 'https://[::1]', 'https://0.0.0.0',
                         'https://user:password@api.example.com', 'https://api.example.com?key=secret',
                         'https://api.example.com:abc', 'https://api.example.com:65536'):
            with self.subTest(endpoint=endpoint), self.assertRaises(ValueError):
                apps.validate_endpoint(endpoint)
        apps.validate_endpoint('https://api.example.com/api')

    def test_all_workspace_versions_must_match_release_tag(self):
        import json
        version = json.loads((apps.ROOT / 'client/package.json').read_text())['version']
        apps.validate_version(apps.ROOT, version)
        with self.assertRaises(ValueError):
            apps.validate_version(apps.ROOT, version + '0')

    def test_missing_signing_inputs_fail_without_exposing_secret_values(self):
        with self.assertRaisesRegex(ValueError, 'ANDROID_KEYSTORE_PATH'):
            apps.validate_android({'ANDROID_STORE_PASSWORD': 'private-password'})

    def test_android_version_code_limits(self):
        with tempfile.NamedTemporaryFile() as key:
            env = dict(ANDROID_KEYSTORE_PATH=key.name, ANDROID_STORE_PASSWORD='secret',
                       ANDROID_KEY_ALIAS='release', ANDROID_KEY_PASSWORD='secret')
            for code in ('', '0', '-1', '2100000001', '1.0', '1\n2'):
                with self.subTest(code=code), self.assertRaises(ValueError):
                    apps.validate_android(dict(env, ANDROID_VERSION_CODE=code))
            self.assertEqual(apps.validate_android(dict(env, ANDROID_VERSION_CODE='42')), 42)

    def test_unsigned_apk_is_never_collected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'source'
            source.mkdir()
            (source / 'release').mkdir()
            (source / 'release/app-release-unsigned.apk').write_bytes(b'unsigned')
            with self.assertRaises(ValueError):
                apps.collect(source, root / 'out', ('.apk',))

    def test_debug_apk_is_not_included_in_release(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for variant in ('debug', 'release'):
                folder = root / 'source' / variant
                folder.mkdir(parents=True)
                (folder / f'app-{variant}.apk').write_bytes(variant.encode())
            artifacts = apps.collect(root / 'source', root / 'out', ('.apk',))
            self.assertEqual([p.name for p in artifacts], ['app-release.apk'])

    def test_missing_or_ambiguous_bundles_fail_before_copy(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'source'
            source.mkdir()
            (source / 'admin.AppImage').write_bytes(b'linux')
            with self.assertRaises(ValueError):
                apps.collect(source, root / 'out', ('.AppImage', '.deb'))
            self.assertFalse((root / 'out').exists())
            (source / 'old.AppImage').write_bytes(b'stale')
            with self.assertRaises(ValueError):
                apps.collect(source, root / 'out', ('.AppImage',))

    def test_failed_build_does_not_collect_previous_artifacts(self):
        import subprocess
        with patch.dict(os.environ, {'VITE_API_ENDPOINT': 'https://api.example.com'}), \
             patch('sys.argv', ['apps.py', '--platform', 'linux']), \
             patch('apps.subprocess.run', side_effect=subprocess.CalledProcessError(1, 'tauri')), \
             patch('apps.collect') as collect:
            with self.assertRaises(subprocess.CalledProcessError):
                apps.main()
            collect.assert_not_called()


if __name__ == '__main__':
    unittest.main()
