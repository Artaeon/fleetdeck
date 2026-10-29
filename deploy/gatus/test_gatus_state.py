import copy
import datetime as dt
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import gatus_state as bridge


class ImportTests(unittest.TestCase):
    def setUp(self):
        self.now = dt.datetime(2026, 1, 2, tzinfo=dt.timezone.utc)
        self.result = {"success": True, "status": 200, "duration": 10,
                       "timestamp": "2026-01-01T23:59:59.123456789Z",
                       "hostname": "private.example.invalid",
                       "conditionResults": [{"condition": "private expression", "success": True}]}
        self.entries = [{"key": "web_frontend", "name": "private name", "results": [self.result]}]

    def test_preserves_observation_time_and_excludes_private_fields(self):
        state = bridge.convert(self.entries, self.now)
        result = state["results"]["web_frontend"]
        self.assertEqual(result["checked_at"], self.result["timestamp"])
        self.assertTrue(result["healthy"])
        self.assertNotIn("private", json.dumps(state))

    def test_stale_and_future_results_are_not_restamped(self):
        for stamp in ["2020-01-01T00:00:00Z", "2099-01-01T00:00:00Z"]:
            self.result["timestamp"] = stamp
            result = bridge.convert(self.entries, self.now)["results"]["web_frontend"]
            self.assertEqual(result["checked_at"], stamp)

    def test_latest_failure_overrides_older_success_and_conflicting_tie(self):
        bad = copy.deepcopy(self.result)
        bad["conditionResults"][0]["success"] = False
        self.entries[0]["results"] = [bad, self.result]
        self.assertFalse(bridge.convert(self.entries, self.now)["results"]["web_frontend"]["healthy"])
        bad["timestamp"] = "2026-01-02T00:00:00Z"
        self.assertFalse(bridge.convert(self.entries, self.now)["results"]["web_frontend"]["healthy"])

    def test_missing_results_are_unknown(self):
        self.entries[0]["results"] = []
        state = bridge.convert(self.entries, self.now)
        self.assertEqual(len(state["targets"]), 1)
        self.assertEqual(state["results"], {})

    def test_external_push_results_keep_reported_success_and_time(self):
        del self.result["conditionResults"]
        self.assertTrue(bridge.convert(self.entries, self.now)["results"]["web_frontend"]["healthy"])
        self.result["errors"] = ["private failure detail"]
        state = bridge.convert(self.entries, self.now)
        self.assertFalse(state["results"]["web_frontend"]["healthy"])
        self.assertNotIn("private", json.dumps(state))

    def test_rejects_duplicate_names_and_untyped_success(self):
        with self.assertRaises(ValueError):
            bridge.convert(self.entries * 2, self.now)
        for value in [1, "true", None]:
            self.result["success"] = value
            with self.assertRaises(ValueError):
                bridge.convert(self.entries, self.now)

    def test_failed_collection_replaces_prior_success(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "state.json"
            bridge.write_state(path, bridge.convert(self.entries, self.now))
            with patch.object(bridge, "fetch", side_effect=OSError("private transport error")):
                self.assertFalse(bridge.collect("unused", path))
            state = json.loads(path.read_text())
            self.assertEqual(state["results"], {})
            self.assertEqual(state["targets"], [])
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)

    def test_rejects_non_loopback_and_embedded_credentials_without_connecting(self):
        for base in ["http://example.invalid", "http://10.0.0.1", "http://user:secret@127.0.0.1", "http://localhost"]:
            with self.assertRaises(ValueError), patch.object(bridge.urllib.request, "build_opener") as opener:
                bridge.fetch(base + bridge.API_PATH)
            opener.assert_not_called()

    def test_redirects_are_refused(self):
        with self.assertRaises(ValueError):
            bridge.NoRedirect().redirect_request(None, None, 302, "Found", {}, "https://example.invalid")

    def test_writable_state_directory_is_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            os.chmod(directory, 0o777)
            with self.assertRaises(ValueError):
                bridge.write_state(Path(directory) / "state.json", {})


if __name__ == "__main__":
    unittest.main()
