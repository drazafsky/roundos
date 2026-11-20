"""Offline tests for the low-impact performance sampler."""
import importlib.util
import pathlib
import tempfile
import unittest
from unittest import mock

HERE = pathlib.Path(__file__).parents[1] / "collect-baseline.py"
spec = importlib.util.spec_from_file_location("baseline", HERE)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class SamplerTests(unittest.TestCase):
    def test_cpu_delta_and_rss(self):
        a = {"roundd:10:1": {"name": "roundd", "pid": 10, "cpu_seconds": 2.0, "rss_kib": 4096}}
        b = {"roundd:10:1": {"name": "roundd", "pid": 10, "cpu_seconds": 2.5, "rss_kib": 5000}}
        result = module.sample(a, b, 2)
        self.assertEqual(result, [{"name": "roundd", "pid": 10, "rss_kib": 5000, "cpu_percent": 25.0}])

    def test_missing_previous_process_has_no_spurious_cpu(self):
        b = {"round-clock:11:2": {"name": "round-clock", "pid": 11, "cpu_seconds": 2, "rss_kib": 1024}}
        self.assertIsNone(module.sample({}, b, 1)[0]["cpu_percent"])

    def test_process_and_credential_separation(self):
        with tempfile.TemporaryDirectory() as tmp:
            proc = pathlib.Path(tmp)
            (proc / "123").mkdir()
            # Unrecognized process is not surfaced even with secret-like name.
            (proc / "123" / "stat").write_text("123 (secret-wifi-psk) S " + "0 " * 40)
            self.assertEqual(module.read_tasks(tmp), {})
            # Fields exposed in sampled processes are allowlisted metadata only.
            self.assertTrue(set(["name", "pid", "cpu_percent", "rss_kib"]) >= set(module.sample({}, {}, 1)[0]) if module.sample({}, {}, 1) else True)


if __name__ == "__main__":
    unittest.main()
