import unittest

from task_manager import TaskManager


class TaskManagerTests(unittest.TestCase):
    def test_accepts_only_apple_music_https_urls(self):
        valid = "https://music.apple.com/us/album/example/123"
        self.assertEqual(TaskManager._validate_url(valid), valid)
        for invalid in (
            "http://music.apple.com/us/album/example/123",
            "https://example.com/us/album/example/123",
            "not-a-url",
        ):
            with self.assertRaises(ValueError):
                TaskManager._validate_url(invalid)

    def test_builds_argument_list_without_shell_splitting(self):
        manager = TaskManager.__new__(TaskManager)
        manager.binary = "/usr/local/bin/apple-music-dl"
        task = {
            "kind": "aac",
            "url": "https://music.apple.com/us/album/an-example/123?i=456",
            "options": {
                "aac_type": "aac-lc",
                "alac_max": 96000,
                "atmos_max": 2448,
                "mv_audio_type": "atmos",
                "mv_max": 1080,
            },
        }
        arguments = manager._argv(task)
        self.assertIn("--aac", arguments)
        self.assertEqual(arguments[-1], task["url"])
        self.assertNotIn("sh", arguments)


if __name__ == "__main__":
    unittest.main()
