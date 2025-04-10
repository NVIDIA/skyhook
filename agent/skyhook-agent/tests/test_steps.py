# 
# LICENSE START
#
#    Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
#    Licensed under the Apache License, Version 2.0 (the "License");
#    you may not use this file except in compliance with the License.
#    You may obtain a copy of the License at
#
#        http://www.apache.org/licenses/LICENSE-2.0
#
#    Unless required by applicable law or agreed to in writing, software
#    distributed under the License is distributed on an "AS IS" BASIS,
#    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#    See the License for the specific language governing permissions and
#    limitations under the License.
#
# LICENSE END
# 


import unittest, os

from tempfile import TemporaryDirectory
from contextlib import contextmanager

from unittest import mock
from skyhook_agent.step import Steps, Step, UpgradeStep, Mode, StepError, APPLY_TO_CHECK

def _dump_steps(requires_interrupt=False, pass_validation=True):

    steps = {
        Mode.UNINSTALL: [Step("uninstall", "uninstall")],
        Mode.UNINSTALL_CHECK: [Step("uninstall_check", "uninstall_check")],
        Mode.UPGRADE: [Step("upgrade", "upgrade"), UpgradeStep("upgradeStep", "upgradeStep")],
        Mode.UPGRADE_CHECK: [Step("upgrade_check", "upgrade_check"), UpgradeStep("upgradeStep_check", "upgradeStep_check")],
        Mode.APPLY: [Step("foo", "foo", requires_interrupt=requires_interrupt)],
        Mode.APPLY_CHECK: [Step("foobar", "foobar")],
        Mode.CONFIG: [Step("foo_config", "foo_config")],
        Mode.CONFIG_CHECK: [Step("foo_config_check", "foo_config_check")],
        Mode.POST_INTERRUPT: [Step("bar", "bar")],
        Mode.POST_INTERRUPT_CHECK: [Step("barfoo", "barfoo")],
    }
    if not pass_validation:
        return Steps.dump(steps, root_dir="/tmp")
    # Setup validation passing
    with _make_files_for_validation(steps) as tmpdir:
        steps = Steps.dump(steps, root_dir=tmpdir)

    return steps

@contextmanager
def _make_files_for_validation(steps):
    with TemporaryDirectory() as tmpdir:
        for _, mode_steps in steps.items():
            for step in mode_steps:
                with open(f"{tmpdir}/{step.path}", "w") as f:
                    f.write("#!/bin/bash\n")
        yield tmpdir

class TestStepsSerialization(unittest.TestCase):
    def test_serialization(self):
        steps = _dump_steps()
        expected = {
            "uninstall": [{"name": "uninstall", "path": "uninstall", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": False}],
            "uninstall-check": [{"name": "uninstall_check", "path": "uninstall_check", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": False}],
            "upgrade": [
                {"name": "upgrade", "path": "upgrade", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": False},
                {"name": "upgradeStep", "path": "upgradeStep", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": True}
            ],
            "upgrade-check": [
                {"name": "upgrade_check", "path": "upgrade_check", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": False},
                {"name": "upgradeStep_check", "path": "upgradeStep_check", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": True}
            ],
            "apply": [ {"name": "foo", "path": "foo", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": False},],
            "apply-check": [{"name": "foobar", "path": "foobar", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": False},],
            "config": [{"name": "foo_config", "path": "foo_config", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": False},],
            "config-check": [{"name": "foo_config_check", "path": "foo_config_check", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": False},],
            "post-interrupt": [{"name": "bar", "path": "bar", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": False},],
            "post-interrupt-check": [{"name": "barfoo", "path": "barfoo", "arguments": [], "returncodes": [0], "on_host": True, "idempotence": False, "upgrade_step": False},],
        }
        for mode, mode_steps in steps.items():
            self.assertEqual(expected[mode], mode_steps)

        self.assertRaises(ValueError, _dump_steps, pass_validation=False)

    def test_circular_serialization(self):
        steps = _dump_steps()

        copied_steps = {m: [dict(**s) for s in msteps] for m, msteps in steps.items()}
        with TemporaryDirectory() as tmpdir:
            for _, mode_steps in steps.items():
                for step in mode_steps:
                    with open(f"{tmpdir}/{step['path']}", "w") as f:
                        f.write("#!/bin/bash\n")
            loaded_steps = Steps.load(copied_steps, root_dir=tmpdir)
        
        for mode, mode_steps in steps.items():
            for i, step in enumerate(mode_steps):
                self.assertEqual(step, loaded_steps[Mode(mode)][i].dump())
        
class TestStep(unittest.TestCase):
    def test_idempotence_validation(self):
        self.assertRaises(ValueError, Step, "foo", idempotence="a")

    def test_step_name_default(self):
        step = Step("foo.sh")
        self.assertEqual(step.name, "foo.sh")

        step = Step("foo.sh", name="my_name")
        self.assertEqual(step.name, "my_name")

    @unittest.mock.patch("skyhook_agent.step.logger.warning")
    def test_step_validation_with_no_warnings(self, mockstep_warning):
        steps = {
            Mode.APPLY: [Step("foo.sh")],
            Mode.APPLY_CHECK: [Step("foo_check.sh")],
            Mode.POST_INTERRUPT: [Step("bar.sh")],
            Mode.POST_INTERRUPT_CHECK: [Step("bar_check.sh")],
        }
        with _make_files_for_validation(steps) as tmpdir:
            Steps.validate(steps, root_dir=tmpdir)
        mockstep_warning.assert_not_called()

    @unittest.mock.patch("skyhook_agent.step.logger.warning")
    def test_step_validation_with_one_warning(self, mockstep_warning):
        steps = {
            Mode.APPLY: [Step("foo.sh")],
            Mode.APPLY_CHECK: [Step("barfoo_check.sh")],
            Mode.POST_INTERRUPT: [Step("bar.sh")],
            Mode.POST_INTERRUPT_CHECK: [Step("bar_check.sh")],
        }
        with _make_files_for_validation(steps) as tmpdir:
            Steps.validate(steps, root_dir=tmpdir)
        mockstep_warning.assert_called_with(f" foo_check.sh doesn't exist. Checks ensure that all tasks in the step will complete.")

    @unittest.mock.patch("skyhook_agent.step.logger.warning")
    def test_step_validation_with_two_warnings(self, mockstep_warning):
        steps = {
            Mode.APPLY: [Step("foo")],
            Mode.APPLY_CHECK: [Step("barfoo_check.sh")],
            Mode.POST_INTERRUPT: [Step("bar")],
            Mode.POST_INTERRUPT_CHECK: [Step("barfoo")],
        }
        with _make_files_for_validation(steps) as tmpdir:
            Steps.validate(steps, root_dir=tmpdir)
        mockstep_warning.assert_has_calls([
            mock.call(f" bar_check doesn't exist. Checks ensure that all tasks in the step will complete."),
            mock.call(f" foo_check doesn't exist. Checks ensure that all tasks in the step will complete."),
        ], any_order=True)

    def test_step_validation_errors_with_no_steps(self):
        steps = {
        }
        self.assertRaisesRegex(StepError, "There are no defined steps.", Steps.validate, steps, "/tmp")

    def test_step_validation_errors_with_all_check_steps(self):
        steps = {
            Mode.APPLY_CHECK: [Step("foo_check.sh")],
        }
        non_check_modes = APPLY_TO_CHECK.keys()
        self.assertRaisesRegex(StepError, f"There are only check modes defined. You must define at least one of {', '.join(m.name for m in non_check_modes)}", Steps.validate, steps, "/tmp")

    def test_step_validation_errors_with_no_apply_checks(self):
        steps = {
            Mode.APPLY: [Step("foo.sh")]
        }
        self.assertRaisesRegex(StepError, "Couldn't validate steps. There are no checks for any of the apply steps.", Steps.validate, steps, "/tmp")

    def test_step_validation_errors_with_no_post_interrupt_checks(self):
        steps = {
            Mode.APPLY: [Step("foo.sh")],
            Mode.APPLY_CHECK: [Step("foo_check.sh")],
            Mode.POST_INTERRUPT: [Step("bar.sh")],
        }
        self.assertRaisesRegex(StepError, "Couldn't validate steps. There are no checks for any of the post-interrupt steps.", Steps.validate, steps, "/tmp")

    def test_step_validation_errors_with_no_config_checks(self):
        steps = {
            Mode.APPLY: [Step("foo.sh")],
            Mode.APPLY_CHECK: [Step("foo_check.sh")],
            Mode.CONFIG: [Step("foo_config.sh")],
        }
        self.assertRaisesRegex(StepError, "Couldn't validate steps. There are no checks for any of the config steps.", Steps.validate, steps, "/tmp")

    def test_step_validation_errors_with_no_uninstall_checks(self):
        steps = {
            Mode.APPLY: [Step("foo.sh")],
            Mode.APPLY_CHECK: [Step("foo_check.sh")],
            Mode.UNINSTALL: [Step("foo_uninstall.sh")],
        }
        self.assertRaisesRegex(StepError, "Couldn't validate steps. There are no checks for any of the uninstall steps.", Steps.validate, steps, "/tmp")

    def test_step_validation_errors_with_no_upgrade_checks(self):
        steps = {
            Mode.APPLY: [Step("foo.sh")],
            Mode.APPLY_CHECK: [Step("foo_check.sh")],
            Mode.UPGRADE: [Step("foo_upgrade.sh")],
        }
        self.assertRaisesRegex(StepError, "Couldn't validate steps. There are no checks for any of the upgrade steps.", Steps.validate, steps, "/tmp")

    def test_step_validation_errors_with_upgradestep_not_in_upgrade_or_upgrade_check_modes(self):
        steps = {
            Mode.APPLY: [Step("foo.sh")],
            Mode.APPLY_CHECK: [Step("foo_check.sh")],
            Mode.CONFIG: [UpgradeStep("foo_upgrade.sh")],
            Mode.CONFIG_CHECK: [UpgradeStep("foo_upgrade_check.sh")],
        }
        self.assertRaisesRegex(StepError, "UpgradeStep foo_upgrade.sh defined in the config mode but can only be defined in the UPGRADE or UPGRADE_CHECK modes.", Steps.validate, steps, "/tmp")
    
    def test_step_validation_errors_with_upgradestep_in_upgrade_with_arg(self):
       with self.assertRaisesRegex(StepError, "UpgradeStep foo_upgrade\.sh can not have any arguments, but found: \['bogus'\]"):
           UpgradeStep("foo_upgrade.sh", arguments=["bogus"])

    def test_step_validation_checks_for_all_empty_lists(self):
        steps = {
            Mode.APPLY: [],
            Mode.APPLY_CHECK: [],
            Mode.CONFIG: [],
        }
        self.assertRaisesRegex(StepError, "There are no defined steps.", Steps.validate, steps, "/tmp")