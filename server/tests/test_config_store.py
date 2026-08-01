import tempfile
import unittest
from pathlib import Path

import yaml

from config_store import ConfigStore


class ConfigStoreTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        root = Path(self.temp.name)
        self.default = root / "default.yaml"
        self.path = root / "data" / "config.yaml"
        self.downloads = root / "downloads"
        self.default.write_text(
            yaml.safe_dump(
                {
                    "storefront": "enter your account storefront",
                    "media-user-token": "your-media-user-token",
                    "alac-save-folder": "AM-DL downloads",
                    "atmos-save-folder": "Atmos",
                    "aac-save-folder": "AAC",
                    "mv-save-folder": "MV",
                }
            ),
            encoding="utf-8",
        )
        self.store = ConfigStore(self.path, self.default, self.downloads)

    def tearDown(self):
        self.temp.cleanup()

    def test_normalizes_container_settings(self):
        raw = yaml.safe_load(self.path.read_text(encoding="utf-8"))
        self.assertEqual(raw["storefront"], "us")
        self.assertEqual(raw["decrypt-m3u8-port"], "wrapper:10020")
        self.assertEqual(raw["get-m3u8-port"], "wrapper:20020")
        self.assertTrue(raw["exit-on-error"])
        self.assertTrue(str(raw["alac-save-folder"]).startswith(str(self.downloads)))
        self.assertEqual(raw["embed-lrc-type"], "word")
        self.assertEqual(raw["save-lrc-file-type"], "line")

    def test_redacts_media_token_and_updates_settings(self):
        result = self.store.update({"storefront": "cn", "media_user_token": "secret-token", "embed_cover": False})
        self.assertEqual(result["storefront"], "cn")
        self.assertEqual(result["media_user_token"], "")
        self.assertTrue(result["media_user_token_set"])
        self.assertFalse(result["embed_cover"])

    def test_updates_independent_lyrics_modes(self):
        result = self.store.update({"embed_lrc_type": "line", "save_lrc_file_type": "word"})
        self.assertEqual(result["embed_lrc_type"], "line")
        self.assertEqual(result["save_lrc_file_type"], "word")


if __name__ == "__main__":
    unittest.main()
