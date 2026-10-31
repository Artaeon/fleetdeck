import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("host", Path(__file__).with_name("host.py"))
host = importlib.util.module_from_spec(spec)
spec.loader.exec_module(host)


class MinimizationTest(unittest.TestCase):
    def setUp(self):
        self.plan = {"version": 1, "host": "fixture", "units": {
            "ModemManager.service": {"Id": "ModemManager.service", "LoadState": "loaded",
                "ActiveState": "active", "UnitFileState": "enabled",
                "FragmentPath": "/usr/lib/systemd/system/ModemManager.service"}}}

    def test_rejects_other_host_critical_services_and_custom_replacements(self):
        with patch.object(host, "identity", return_value="other"):
            with self.assertRaises(ValueError): host.validate_plan(self.plan)
        with patch.object(host, "identity", return_value="fixture"):
            unit = self.plan["units"].pop("ModemManager.service")
            self.plan["units"]["ssh.service"] = unit
            with self.assertRaises(ValueError): host.validate_plan(self.plan)
            self.plan["units"] = {"ModemManager.service": unit}
            unit["FragmentPath"] = "/etc/systemd/system/ModemManager.service"
            with self.assertRaises(ValueError): host.validate_plan(self.plan)

    def test_recovery_is_armed_before_mutation_and_failure_restores(self):
        calls = []
        def run(args, **kwargs):
            calls.append(args)
            if args[:3] == ["systemctl", "mask", "--runtime"]:
                raise RuntimeError("simulated failure")
        with tempfile.TemporaryDirectory() as folder, \
                patch.object(host, "identity", return_value="fixture"), \
                patch.object(host, "inspect_unit", return_value=self.plan["units"]["ModemManager.service"]), \
                patch.object(host, "run", side_effect=run), patch.object(host, "rollback") as rollback:
            with self.assertRaises(RuntimeError): host.apply(Path(folder), self.plan, Path("/root/helper.py"))
            self.assertEqual(calls[0][0], "systemd-run")
            self.assertEqual(calls[1][:3], ["systemctl", "mask", "--runtime"])
            rollback.assert_called_once()

    def test_stale_plan_never_arms_or_changes_services(self):
        with tempfile.TemporaryDirectory() as folder, \
                patch.object(host, "identity", return_value="fixture"), \
                patch.object(host, "inspect_unit", return_value={}), patch.object(host, "run") as run:
            with self.assertRaises(ValueError): host.apply(Path(folder), self.plan, Path("/root/helper.py"))
            run.assert_not_called()

    def test_confirmation_expiry_and_completed_timer_are_safe(self):
        state = {"plan": self.plan, "phase": "pending", "deadline": 0, "timer": "fixture"}
        with patch.object(host, "run") as run:
            with self.assertRaises(ValueError): host.confirm(Path("/unused"), state)
            run.assert_not_called()
            state["phase"] = "confirmed"
            host.rollback(Path("/unused"), state, if_pending=True)
            run.assert_not_called()


if __name__ == "__main__": unittest.main()
