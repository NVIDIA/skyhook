#  SPDX-FileCopyrightText: Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
#  SPDX-License-Identifier: Apache-2.0
import unittest
import tempfile
import os
import json
import asyncio
import textwrap
import shutil

from datetime import datetime, timezone

from contextlib import contextmanager
from unittest import mock


from skyhook_agent import controller, config
from skyhook_agent.step import Step, UpgradeStep, Idempotence, Mode
from skyhook_agent import interrupts

this_directory = os.path.dirname(os.path.abspath(__file__))

os.environ['OVERLAY_FRAMEWORK_VERSION'] = 'testing-1.0.0'

# A helper to set environment variables within a context
@contextmanager
def set_env(**vars):
    """
    Temporarily set environment variables within a context. Once
    complete if the environment variable was not set before it will
    be deleted. If it was set before it will be restored to its
    original value.
    """
    prior_values = {k: os.environ[k] for k in vars if k in os.environ}
    os.environ.update(vars)
    try:
        yield
    finally:
        for k in vars:
            if k in prior_values:
                os.environ[k] = prior_values[k]
            else:
                del os.environ[k]


class FakeSubprocessResult:
    def __init__(self, returncode):
        self.returncode = returncode


def fake_a_tee(return_code):
    async def fake_tee(*args, **kwargs):
        return FakeSubprocessResult(return_code)

    return fake_tee

class FakeIO():
    def __init__(self) -> None:
        self.buffer = ""

    def write(self, x):
        self.buffer += x

    def read_lines(self):
        return self.buffer.split("\n")

    def flush(self): pass

    def read(self):
        return self.buffer


PYTHON_EXE=os.getenv("PYTHON_EXE", "python")

class TestHelpers(unittest.TestCase):
    def setUp(self):
        self.config_data = {"package_name": "foo", "package_version": "1.0.0"}

    def test_make_flag_path_uses_args(self):
        path_a = controller.make_flag_path(Step("foo.sh", arguments=["1", "2"], returncodes=(0, 1, 2)), "/", self.config_data)
        path_b = controller.make_flag_path(Step("foo.sh", arguments=["1"], returncodes=(0, 1, 2)), "/", self.config_data)

        self.assertNotEqual(path_a, path_b)

    def test_make_flag_path_uses_returncodes(self):
        path_a = controller.make_flag_path(Step("foo.sh", arguments=["1", "2"], returncodes=(0, 1, 2)), "/", self.config_data)
        path_b = controller.make_flag_path(Step("foo.sh", arguments=["1", "2"], returncodes=(0)), "/", self.config_data)

        self.assertNotEqual(path_a, path_b)

    def test_set_flag(self):
        with tempfile.TemporaryDirectory() as dir:
            path = f"{dir}/foo/bar.sh_123asdas"
            controller.set_flag(path)
            self.assertTrue(os.path.exists(path))

    def test_get_history_dir(self):
        root_mount = "/root"
        self.assertEqual(controller.get_history_dir(root_mount=root_mount), "/root/etc/skyhook/history")

    @mock.patch("skyhook_agent.controller.sys")
    def test_tee_adds_cmds(self, sys_mock):
        sys_mock.stdout = FakeIO()
        sys_mock.stderr = FakeIO()

        with tempfile.TemporaryDirectory() as dir:
            with open(f"{dir}/foo", 'w') as f:
                f.write("")

            with tempfile.NamedTemporaryFile('w') as f:
                result = asyncio.run(
                    controller.tee(["ls", dir], f.name, f"{dir}/foo.err", write_cmds=True)
                )
                self.assertEqual(
                    f"ls {dir}",
                    sys_mock.stdout.read_lines()[0].strip()
                )
                with open(f.name, 'r') as read_f:
                    self.assertEqual(
                        f"ls {dir}",
                        read_f.read().split("\n")[0]
                    )

    def test_stream_process_deals_with_large_lines(self):
        async def make_process(file, buffer):
            p = await asyncio.create_subprocess_shell(f"cat {file}", limit=5, stdout=asyncio.subprocess.PIPE)
            await(controller._stream_process(p.stdout, [buffer], limit=5))

        with tempfile.TemporaryDirectory() as dir:
            with open(f"{dir}/foo", 'w') as f:
                f.write("a" * 1000 + "\n")
                f.write("b" * 1000 + "\n")
                f.write("c" * 1000 + "\n")
                f.flush()

            buffer = FakeIO()
            asyncio.run(make_process(f"{dir}/foo", buffer))

            self.assertEqual(len(buffer.read_lines()), 4)

    def test_get_package_information(self):
        name, version = controller._get_package_information(self.config_data) 
        self.assertEqual(name, "foo")
        self.assertEqual(version, "1.0.0")

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller.os.makedirs")
    def test_get_log_file(self, os_mock, datetime_mock):
        now_mock = mock.MagicMock()
        datetime_mock.now.return_value = now_mock
        now_mock.strftime.return_value = "12345"
        log_file = controller.get_log_file("", "foo", "copy_dir", self.config_data)
        self.assertEqual(log_file, f"/var/log/skyhook/{self.config_data['package_name']}/{self.config_data['package_version']}/foo-12345.log")

    def test_make_flag_path_has_package_name(self):
        flag_path = controller.make_flag_path(Step("foo", returncodes=[0]), "/", self.config_data)
        self.assertTrue(flag_path.startswith(f"{controller.get_skyhook_directory('/')}/flags/foo/1.0.0"))

    @mock.patch("skyhook_agent.controller.cleanup_old_logs")
    @mock.patch("skyhook_agent.controller.get_log_file")
    @mock.patch("skyhook_agent.controller.subprocess")
    @mock.patch("skyhook_agent.controller.tee")
    @mock.patch("skyhook_agent.controller.os.makedirs")
    # @mock.patch.object(controller, "tee", fake_a_tee(0))
    def test_run_step_is_successful(self, os_mock, tee_mock, subprocess_mock, log_mock, cleanup_mock):
        subprocess_mock.run.return_value = FakeSubprocessResult(0)
        tee_mock.return_value = FakeSubprocessResult(0)

        run_step_result = controller.run_step(
            Step("foo", arguments=["a", "b"], returncodes=[0]), "/", "copy_dir", self.config_data
        )
        self.assertFalse(run_step_result)

        subprocess_mock.run.assert_has_calls(
            [
                mock.call("chroot / chmod +x copy_dir/skyhook_dir/foo", shell=True),
            ]
        )
        log_file = controller.get_log_file(
            "/", f"{controller.get_host_path_for_steps('copy_dir')}/foo", "copy_dir", self.config_data
        )
        tee_mock.assert_has_calls(
            [
                mock.call(
                    ["chroot", "/", "copy_dir/skyhook_dir/foo", "a", "b"],
                    log_file,
                    f"{log_file}.err",
                    env=dict(**os.environ, **{"STEP_ROOT": "copy_dir/skyhook_dir", "SKYHOOK_DIR": "copy_dir"}),
                    write_cmds=False,
                )
            ]
        )

    @mock.patch("skyhook_agent.controller.cleanup_old_logs")
    @mock.patch("skyhook_agent.controller.get_log_file")
    @mock.patch("skyhook_agent.controller.subprocess")
    @mock.patch("skyhook_agent.controller.tee")
    @mock.patch("skyhook_agent.controller.os.makedirs")
    def test_run_step_is_failed(self, os_mock, tee_mock, subprocess_mock, get_log_file_mock, cleanup_mock):
        # chmod +x will work
        subprocess_mock.run.return_value = FakeSubprocessResult(0)
        # step will fail
        tee_mock.return_value = FakeSubprocessResult(1)
        run_step_result = controller.run_step(Step("foo", arguments=["a", "b"], returncodes=[0]), "/", "copy_dir", self.config_data)
        self.assertTrue(run_step_result)

    @mock.patch("skyhook_agent.controller.cleanup_old_logs")
    @mock.patch("skyhook_agent.controller.get_log_file")
    @mock.patch("skyhook_agent.controller.subprocess")
    @mock.patch("skyhook_agent.controller.tee")
    @mock.patch("skyhook_agent.controller.os.makedirs")
    def test_run_step_replaces_environment_variables(
        self, os_mock, tee_mock, subprocess_mock, log_mock, cleanup_mock
    ):
        subprocess_mock.run.return_value = FakeSubprocessResult(0)
        tee_mock.return_value = FakeSubprocessResult(0)

        with set_env(FOO="foo"):
            run_step_result = controller.run_step(
                Step("foo", arguments=["a", "env:FOO"], returncodes=[0]), "/", "copy_dir", self.config_data
            )
        self.assertFalse(run_step_result)

        subprocess_mock.run.assert_has_calls(
            [mock.call("chroot / chmod +x copy_dir/skyhook_dir/foo", shell=True)]
        )
        log_file = controller.get_log_file(
            "/", f"{controller.get_host_path_for_steps('copy_dir')}/foo", "copy_dir", self.config_data
        )
        tee_mock.assert_has_calls(
            [
                mock.call(
                    ["chroot", "/", "copy_dir/skyhook_dir/foo", "a", "foo"],
                    log_file,
                    f"{log_file}.err",
                    env=dict(**os.environ, **{"STEP_ROOT": "copy_dir/skyhook_dir", "FOO": "foo", "SKYHOOK_DIR": "copy_dir"}),
                    write_cmds=False
                )
            ]
        )

    @mock.patch("builtins.print")
    def test_run_step_prints_all_missing_environment_variables(self, print_mock):
        run_step_result = controller.run_step(
            Step("foo", arguments=["/some/path", "env:BAR", "env:FOO"], returncodes=[0]), "/", "copy_dir", self.config_data
        )
        self.assertTrue(run_step_result)

        print_mock.assert_has_calls(
            [
                mock.call("foo: Expected environment variable did not exist: BAR"),
                mock.call("foo: Expected environment variable did not exist: FOO"),
            ]
        )

    @mock.patch("skyhook_agent.controller.os")
    def test_check_flag_file(self, os_mock):
        os_mock.path = mock.MagicMock()

        os_mock.path.exists.return_value = True
        self.assertTrue(
            controller.check_flag_file(Step("foo", idempotence=Idempotence.Auto), "foo_bar", False, Mode.APPLY)
        )
        self.assertFalse(
            controller.check_flag_file(Step("foo", idempotence=Idempotence.Disabled), "foo_bar", False, Mode.APPLY)
        )
        self.assertFalse(
            controller.check_flag_file(Step("foo", idempotence=Idempotence.Disabled), "foo_bar", False, Mode.CONFIG)
        )
        self.assertFalse(
            controller.check_flag_file(Step("foo", idempotence=Idempotence.Auto), "foo_bar", False, Mode.CONFIG)
        )
        self.assertFalse(
            controller.check_flag_file(Step("foo", idempotence=Idempotence.Disabled), "foo_bar", False, Mode.UNINSTALL)
        )
        self.assertFalse(
            controller.check_flag_file(Step("foo", idempotence=Idempotence.Auto), "foo_bar", False, Mode.UNINSTALL)
        )

        os_mock.path.exists.return_value = False
        self.assertFalse(
            controller.check_flag_file(Step("foo", idempotence=Idempotence.Auto), "foo_bar", False, Mode.APPLY)
        )

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_check_step_in_container(self, _run_step_mock, datetime_mock):
        now_mock = mock.MagicMock()
        datetime_mock.now.return_value = now_mock
        now_mock.strftime.return_value = "12345"
        with tempfile.TemporaryDirectory() as dir:
            os.makedirs(f"{dir}/skyhook_dir")
            with open(f"{dir}/skyhook_dir/foo_check.py", "w") as f:
                f.write("")
            with mock.patch.object(controller, "get_host_path_for_steps") as host_path_mock:
                host_path_mock.return_value = dir
            
            controller.run_step(Step("foo_check.py", arguments=["a"], on_host=False), "root", dir, self.config_data)

        
        _run_step_mock.assert_has_calls([
            mock.call(
                [f"{dir}/skyhook_dir/foo_check.py", "a"], 
                controller.get_log_file("root", f"/foo_check.py", "", self.config_data), 
                on_host=False, 
                root_mount="root", 
                env=dict(**os.environ, **{"STEP_ROOT": f"{dir}/skyhook_dir", "SKYHOOK_DIR": dir})
            ),
        ])

    @mock.patch("skyhook_agent.controller.get_flag_dir")
    def test_summarize_check_results(self, flag_dir_mock):
        with tempfile.TemporaryDirectory() as dir:
            flag_dir_mock.return_value = dir

            # Has a failure
            steps = {
                Mode.APPLY: [
                    Step("foo", arguments=[]),
                    Step("bar", arguments=[]),
                    Step("baz", arguments=[]),
                ]
            }
            result = controller.summarize_check_results(
                [False, False, True], steps, Mode.APPLY, "root"
            )
            self.assertTrue(result)
            with open(f"{dir}/check_results", "r") as f:
                self.assertEqual("foo False\nbar False\nbaz True", f.read().strip())

            # Did not fail]
            result = controller.summarize_check_results(
                [False, False, False], steps, Mode.APPLY, "root"
            )
            self.assertFalse(result)
            with open(f"{dir}/check_results", "r") as f:
                self.assertEqual("foo False\nbar False\nbaz False", f.read().strip())
            self.assertTrue(os.path.exists(f"{dir}/{str(Mode.APPLY)}_ALL_CHECKED"))


class TestUseCases(unittest.TestCase):
    def setUp(self):
        self.config_data = {"package_name": "foo", "package_version": "1.0.0"}

    @contextmanager
    def _setup_for_main(self, steps, agent_mode="legacy"):
        with tempfile.TemporaryDirectory() as container_root_dir:
            os.makedirs(f"{container_root_dir}/skyhook_dir")
            os.makedirs(f"{container_root_dir}/root_dir")
            os.makedirs(f"{container_root_dir}/configmaps")
            # Create the step file so validation doesn't fail
            for step_list in steps.values():
                for step in step_list:
                    with open(f"{container_root_dir}/skyhook_dir/{step.path}", "w") as temp_f:
                        temp_f.write("")

            config_data = config.dump("foo", "1.0.0", container_root_dir, steps)
            with open(f"{container_root_dir}/config.json", "w") as temp_f:
                json.dump(config_data, temp_f)

            pass_config_data = config.load(config_data, f"{container_root_dir}/skyhook_dir")
            with tempfile.TemporaryDirectory() as root_dir:
                with set_env(
                    SKYHOOK_CONFIGMAP_DIR=f"{container_root_dir}/configmaps",
                    SKYHOOK_AGENT_MODE=agent_mode,
                    SKYHOOK_DATA_DIR=container_root_dir):
                    try:
                        yield container_root_dir, pass_config_data, root_dir
                    finally:
                        shutil.rmtree(container_root_dir)
                        shutil.rmtree(root_dir)

    @mock.patch("skyhook_agent.controller._run")
    def test_flags_are_removed_after_uninstall(self, run_mock):
        run_mock.return_value = 0

        
        steps = {
            Mode.UNINSTALL: [Step("foo", arguments=[])],
            Mode.UNINSTALL_CHECK: [Step("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):

                ## make flags
                flags = []
                os.makedirs(f"{root_dir}/etc/skyhook/flags", exist_ok=True)
                for step in [step for steps in steps.values() for step in steps]:
                    flag_file = controller.make_flag_path(step, root_dir, config_data)
                    controller.set_flag(flag_file, "")
                    flags.append(flag_file)

                ## making flag file that isn't in steps definition to assert that
                ## it doesn't get deleted
                with open(controller.make_flag_path(Step("bogus"), root_dir, config_data), 'w'): pass

                controller.main(str(Mode.UNINSTALL_CHECK), root_dir, f"/foo", None)

                ## assert the flags were erased
                for flag in flags:
                    self.assertFalse(os.path.exists(flag))

                self.assertTrue(os.path.exists(controller.make_flag_path(Step("bogus"), root_dir, config_data)))

    @mock.patch("skyhook_agent.controller._run")
    def test_flags_arent_removed_after_failed_uninstall(self, run_mock):
        run_mock.return_value = 1 ## make uninstall_check fail

        steps = {
            Mode.UNINSTALL: [Step("foo", arguments=[])],
            Mode.UNINSTALL_CHECK: [Step("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):

                ## make flags
                flags = []
                os.makedirs(f"{dir}/etc/skyhook/flags", exist_ok=True)
                for step in [step for steps in steps.values() for step in steps]:
                    flag_file = controller.make_flag_path(step, root_dir, config_data)
                    controller.set_flag(flag_file, "")
                    flags.append(flag_file)

                ## making flag file that isn't in steps definition to assert that
                ## it doesn't get deleted
                with open(controller.make_flag_path(Step("bogus"), root_dir, config_data), 'w'): pass

                controller.main(str(Mode.UNINSTALL_CHECK), root_dir, f"/foo", None)

                ## assert the flags weren't erased
                for flag in flags:
                    self.assertTrue(os.path.exists(flag))

                self.assertTrue(os.path.exists(controller.make_flag_path(Step("bogus"), root_dir, config_data)))

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_version_history_is_created_after_apply(self, run_mock, datetime_mock):
        run_mock.return_value = 0

        mock_time = datetime(2024, 8, 28, 12, 0, 0, tzinfo=timezone.utc)
        datetime_mock.now.return_value = mock_time

        steps = {
            Mode.UPGRADE: [Step("foo", arguments=[])],
            Mode.UPGRADE_CHECK: [Step("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
            controller.main(str(Mode.APPLY_CHECK), root_dir, f"/foo", None)
            with open(f"{root_dir}/etc/skyhook/history/foo.json", "r") as history_file:
                history = json.load(history_file)

                self.assertEqual(history["current-version"], "1.0.0")

                self.assertEqual(len(history["history"]), 1)
                self.assertEqual(history["history"][0]["version"], "1.0.0")
                self.assertEqual(history["history"][0]["time"], mock_time.isoformat())

    @mock.patch("skyhook_agent.controller._run")
    def test_version_history_isnt_changed_after_check_fails(self, run_mock):
        run_mock.return_value = 1
        steps = {
            Mode.UPGRADE: [Step("foo", arguments=[])],
            Mode.UPGRADE_CHECK: [Step("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
            controller.main(str(Mode.APPLY_CHECK), root_dir, f"/foo", None)
            self.assertFalse(os.path.exists(f"{root_dir}/etc/skyhook/history/foo.json"))

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_corrupt_history_file_is_moved_to_backup(self, run_mock, datetime_mock):
        run_mock.return_value = 0

        mock_time = datetime(2024, 8, 28, 12, 0, 0, tzinfo=timezone.utc)
        datetime_mock.now.return_value = mock_time

        steps = {
            Mode.UPGRADE: [Step("foo", arguments=[])],
            Mode.UPGRADE_CHECK: [Step("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):

                os.makedirs(f"{root_dir}/etc/skyhook/history", exist_ok=True)
                with open(f"{root_dir}/etc/skyhook/history/foo.json", "w") as history_file:
                    history_file.write("{") ## Corrupt history file

                controller.main(str(Mode.APPLY_CHECK), root_dir, f"/foo", None)

                with open(f"{root_dir}/etc/skyhook/history/foo.json.backup") as backup_file:
                    backup_data = backup_file.read()
                    self.assertEqual(backup_data, "{")

                with open(f"{root_dir}/etc/skyhook/history/foo.json", "r") as history_file:
                    history = json.load(history_file)

                    self.assertEqual(history["current-version"], "1.0.0")

                    self.assertEqual(len(history["history"]), 1)
                    self.assertEqual(history["history"][0]["version"], "1.0.0")
                    self.assertEqual(history["history"][0]["time"], mock_time.isoformat())

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_version_history_is_updated_after_apply(self, run_mock, datetime_mock):
        run_mock.return_value = 0

        mock_time = datetime(2024, 8, 28, 12, 0, 0, tzinfo=timezone.utc)
        datetime_mock.now.return_value = mock_time

        steps = {
            Mode.UPGRADE: [Step("foo", arguments=[])],
            Mode.UPGRADE_CHECK: [Step("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):

                os.makedirs(f"{root_dir}/etc/skyhook/history", exist_ok=True)
                with open(f"{root_dir}/etc/skyhook/history/foo.json", "w") as history_file:
                    json.dump({
                        "current-version": "0.0.9",
                        "history": [
                            {"version": "0.0.9", "time": "2024-08-28T14:33:20.123456+00:00"}
                        ]
                    }, history_file)

                controller.main(str(Mode.APPLY_CHECK), root_dir, f"/foo", None)
                
                with open(f"{root_dir}/etc/skyhook/history/foo.json", "r") as history_file:
                    history = json.load(history_file)

                    self.assertEqual(history["current-version"], "1.0.0")

                    self.assertEqual(len(history["history"]), 2)
                    self.assertEqual(history["history"][0]["version"], "1.0.0")
                    self.assertEqual(history["history"][0]["time"], mock_time.isoformat())

                    self.assertEqual(history["history"][1]["version"], "0.0.9")
                    self.assertEqual(history["history"][1]["time"], "2024-08-28T14:33:20.123456+00:00")
    
    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_version_history_is_created_after_upgrade(self, run_mock, datetime_mock):
        run_mock.return_value = 0

        mock_time = datetime(2024, 8, 28, 12, 0, 0, tzinfo=timezone.utc)
        datetime_mock.now.return_value = mock_time
        
        steps = {
            Mode.UPGRADE: [Step("foo", arguments=[])],
            Mode.UPGRADE_CHECK: [Step("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
                controller.main(str(Mode.UPGRADE_CHECK), root_dir, f"/foo", None)
                
                with open(f"{root_dir}/etc/skyhook/history/foo.json", "r") as history_file:
                    history = json.load(history_file)

                    self.assertEqual(history["current-version"], "1.0.0")

                    self.assertEqual(len(history["history"]), 1)
                    self.assertEqual(history["history"][0]["version"], "1.0.0")
                    self.assertEqual(history["history"][0]["time"], mock_time.isoformat())

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_version_history_is_updated_after_upgrade(self, run_mock, datetime_mock):
        run_mock.return_value = 0

        mock_time = datetime(2024, 8, 28, 12, 0, 0, tzinfo=timezone.utc)
        datetime_mock.now.return_value = mock_time

        steps = {
            Mode.UPGRADE: [Step("foo", arguments=[])],
            Mode.UPGRADE_CHECK: [Step("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):

            os.makedirs(f"{root_dir}/etc/skyhook/history", exist_ok=True)
            with open(f"{root_dir}/etc/skyhook/history/foo.json", "w") as history_file:
                json.dump({
                    "current-version": "0.0.9",
                    "history": [
                        {"version": "0.0.9", "time": "2024-08-28T14:33:20.123456+00:00"}
                    ]
                }, history_file)

            controller.main(str(Mode.UPGRADE_CHECK), root_dir, f"/foo", None)
            
            with open(f"{root_dir}/etc/skyhook/history/foo.json", "r") as history_file:
                history = json.load(history_file)

                self.assertEqual(history["current-version"], "1.0.0")

                self.assertEqual(len(history["history"]), 2)
                self.assertEqual(history["history"][0]["version"], "1.0.0")
                self.assertEqual(history["history"][0]["time"], mock_time.isoformat())

                self.assertEqual(history["history"][1]["version"], "0.0.9")
                self.assertEqual(history["history"][1]["time"], "2024-08-28T14:33:20.123456+00:00")

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_version_history_is_updated_after_uninstall(self, run_mock, datetime_mock):
        run_mock.return_value = 0

        mock_time = datetime(2024, 8, 28, 12, 0, 0, tzinfo=timezone.utc)
        datetime_mock.now.return_value = mock_time
        steps = {
            Mode.UPGRADE: [Step("foo", arguments=[])],
            Mode.UPGRADE_CHECK: [Step("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):

            os.makedirs(f"{root_dir}/etc/skyhook/history", exist_ok=True)
            with open(f"{root_dir}/etc/skyhook/history/foo.json", "w") as history_file:
                json.dump({
                    "current-version": "0.0.9",
                    "history": [
                        {"version": "0.0.9", "time": "2024-08-28T14:33:20.123456+00:00"}
                    ]
                }, history_file)

            controller.main(str(Mode.UNINSTALL_CHECK), root_dir, f"/foo", None)
            
            with open(f"{root_dir}/etc/skyhook/history/foo.json", "r") as history_file:
                history = json.load(history_file)

                self.assertEqual(history["current-version"], "uninstalled")

                self.assertEqual(len(history["history"]), 2)
                self.assertEqual(history["history"][0]["version"], "uninstalled")
                self.assertEqual(history["history"][0]["time"], mock_time.isoformat())

                self.assertEqual(history["history"][1]["version"], "0.0.9")
                self.assertEqual(history["history"][1]["time"], "2024-08-28T14:33:20.123456+00:00")

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_from_and_to_version_is_given_to_upgrade_step_as_env_var(self, run_mock, datetime_mock):
        now_mock = mock.MagicMock()
        datetime_mock.now.return_value = now_mock
        now_mock.strftime.return_value = "12345"
        steps = {
            Mode.UPGRADE: [Step("foo", arguments=[])],
            Mode.UPGRADE_CHECK: [Step("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):

                os.makedirs(f"{root_dir}/etc/skyhook/history", exist_ok=True)
                with open(f"{root_dir}/etc/skyhook/history/foo.json", "w") as history_file:
                    json.dump({
                        "current-version": "0.0.9",
                        "history": [
                            {"version": "0.0.9", "time": "2024-08-28T14:33:20.123456+00:00"}
                        ]
                    }, history_file)

                controller.main(str(Mode.UPGRADE), root_dir, f"/foo", None)

                run_mock.assert_has_calls([
                    mock.call(
                        ["/foo/skyhook_dir/foo"],
                        controller.get_log_file(
                            root_dir, f"/foo/skyhook_dir/foo", f"/foo", config_data
                        ),
                        on_host=True,
                        root_mount=root_dir,
                        env=dict(**os.environ, 
                                **{"PREVIOUS_VERSION": "0.0.9", "CURRENT_VERSION": "1.0.0"}, 
                                **{"STEP_ROOT": f"/foo/skyhook_dir", "SKYHOOK_DIR": "/foo"})
                    )
                ])

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_from_and_to_version_is_given_to_upgradestep_class_as_env_var_and_args(self, run_mock, datetime_mock):
        now_mock = mock.MagicMock()
        datetime_mock.now.return_value = now_mock
        now_mock.strftime.return_value = "12345"
        steps = {
            Mode.UPGRADE: [UpgradeStep("foo", arguments=[])],
            Mode.UPGRADE_CHECK: [UpgradeStep("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
                
            os.makedirs(f"{root_dir}/etc/skyhook/history", exist_ok=True)
            with open(f"{root_dir}/etc/skyhook/history/foo.json", "w") as history_file:
                json.dump({
                    "current-version": "2024.07.28",
                    "history": [
                        {"version": "2024.07.28", "time": "2024-08-28T14:33:20.123456+00:00"}
                    ]
                }, history_file)

            controller.main(str(Mode.UPGRADE), root_dir, f"/foo", None)

            run_mock.assert_has_calls([
                mock.call(
                    ["/foo/skyhook_dir/foo", "2024.07.28", "1.0.0"],
                    controller.get_log_file(
                        root_dir, f"/foo/skyhook_dir/foo", f"/foo", config_data
                    ),
                    on_host=True,
                    root_mount=root_dir,
                    env=dict(**os.environ, 
                            **{"PREVIOUS_VERSION": "2024.07.28", "CURRENT_VERSION": "1.0.0"}, 
                            **{"STEP_ROOT": f"/foo/skyhook_dir", "SKYHOOK_DIR": "/foo"})
                )
            ])

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_unkown_is_given_to_upgrade_step_if_history_file_dont_exist(self, run_mock, datetime_mock):
        now_mock = mock.MagicMock()
        datetime_mock.now.return_value = now_mock
        now_mock.strftime.return_value = "12345"
        steps = {
            Mode.UPGRADE: [Step("foo", arguments=[])],
            Mode.UPGRADE_CHECK: [Step("foo_check", arguments=[])],
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check", arguments=[])],
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):

                controller.main(str(Mode.UPGRADE), root_dir, f"/foo", None)

                run_mock.assert_has_calls([
                    mock.call(
                        ["/foo/skyhook_dir/foo"],
                        controller.get_log_file(
                            root_dir, f"/foo/skyhook_dir/foo", f"/foo", config_data
                        ),
                        on_host=True,
                        root_mount=root_dir,
                        env=dict(**os.environ, 
                                **{"PREVIOUS_VERSION": "unknown", "CURRENT_VERSION": "1.0.0"}, 
                                **{"STEP_ROOT": "/foo/skyhook_dir", "SKYHOOK_DIR": "/foo"})
                    )
                ])

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    @mock.patch("skyhook_agent.controller.subprocess")
    def test_step_root_is_set_correctly(self, subprocess_mock, run_mock, datetime_mock):
        now_mock = mock.MagicMock()
        datetime_mock.now.return_value = now_mock
        now_mock.strftime.return_value = "12345"
        run_mock.return_value = 0
        steps = {
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check")]
        }
        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
                controller.main(str(Mode.APPLY), root_dir, f"/foo", None)

                run_mock.assert_has_calls([
                    mock.call(
                        ["/foo/skyhook_dir/bar"],
                        controller.get_log_file(
                            root_dir, f"/foo/skyhook_dir/bar", f"/foo", config_data
                        ),
                        on_host=True,
                        root_mount=root_dir,
                        env=dict(**os.environ, **{"STEP_ROOT": "/foo/skyhook_dir", "SKYHOOK_DIR": "/foo"})
                    )

                ])

    @mock.patch("skyhook_agent.controller.logger.warning")
    def test_warning_when_running_with_invalid_mode(self, mock_warning):
        controller.main("bogus", "root_dir", f"/foo", None)

        mock_warning.assert_called_with(f"This version of the Agent doesn't support the bogus mode. Options are: {','.join(map(str, Mode))}.")

    @mock.patch("skyhook_agent.controller.logger.warning")
    def test_no_warning_when_running_with_valid_mode(self, mock_warning):
        steps = {
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check")],
        }
        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
            controller.main(str(Mode.APPLY), root_dir, f"/foo", None)

        mock_warning.assert_not_called()

    @mock.patch("skyhook_agent.controller.logger.warning")
    def test_warning_when_running_in_config_mode_with_no_config_steps(self, mock_warning):
        steps = {
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check")]
        }
        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
            controller.main(str(Mode.CONFIG), root_dir, f"/foo", None)

        mock_warning.assert_called_with(f" There are no config steps defined. This will be ran as a no-op.")

    @mock.patch("skyhook_agent.controller.logger.warning")
    def test_no_warning_when_not_running_in_config_mode(self, mock_warning):
        steps = {
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check")]
        }
        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
            controller.main(str(Mode.APPLY), root_dir, f"/foo", None)

        mock_warning.assert_not_called()

    @mock.patch("skyhook_agent.controller.logger.warning")
    def test_no_warning_when_running_in_config_mode_with_config_steps(self, mock_warning):
        steps = {
            Mode.APPLY: [Step("bar", arguments=[])],
            Mode.APPLY_CHECK: [Step("bar_check")],
            Mode.CONFIG: [Step("config", arguments=[])],
            Mode.CONFIG_CHECK: [Step("config_check")]
        }
        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
            controller.main(str(Mode.CONFIG), root_dir, f"/foo", None)

        mock_warning.assert_not_called()

    @mock.patch("skyhook_agent.controller.run_step")
    def test_same_steps_different_args_arent_skipped(self, run_step_mock):
        run_step_mock.return_value = False
        steps = {
            Mode.APPLY: [Step("foo", arguments=[]), Step("foo", arguments=[ "a"])],
            Mode.APPLY_CHECK: [Step("foo_check")]
        }
        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
            controller.main(str(Mode.APPLY), root_dir, "/tmp", None)
            self.assertEqual(run_step_mock.call_count, 2)

    @mock.patch("skyhook_agent.controller.run_step")
    def test_skip_steps_that_have_flags(self, run_step_mock):
        run_step_mock.return_value = False
        steps = {
            Mode.APPLY: [Step("foo", arguments=[]), Step("foo", arguments=[ "a"])],
            Mode.APPLY_CHECK: [Step("foo_check")],
        }
        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
            controller.set_flag(controller.make_flag_path(steps[Mode.APPLY][0], root_dir, config_data))
            controller.main(str(Mode.APPLY), root_dir, "copy_dir", None)
            self.assertEqual(run_step_mock.call_count, 1)
            #run_step_mock.assert_called_once_with(Step("foo", arguments=["a"], returncodes=[0]), root_dir, "copy_dir", config_data)

    @mock.patch("skyhook_agent.controller.run_step")
    def test_steps_that_have_flags_arent_skipped_when_always_run_flag_set(self, run_step_mock):
        run_step_mock.return_value = False
        print(os.getenv("OVERLAY_FRAMEWORK_VERSION"))
        steps = {
            Mode.APPLY: [Step("foo", arguments=[]), Step("foo", arguments=["a"])],
            Mode.APPLY_CHECK: [Step("foo_check")]
        }
        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
            controller.set_flag(controller.make_flag_path(steps[Mode.APPLY][0], root_dir, config_data))
            controller.main(str(Mode.APPLY), root_dir, "copy_dir", None, True)
            run_step_mock.assert_has_calls([
                mock.call(Step("foo", arguments=[], returncodes=[0]), root_dir, "copy_dir", config_data),
                mock.call(Step("foo", arguments=["a"], returncodes=[0]), root_dir, "copy_dir", config_data),
            ])


    # This is invalid. Want to be able to support a re-arrangement of steps?
    # def test_same_steps_same_args_arent_skipped(self): pass

    @mock.patch("skyhook_agent.controller.run_step")
    def test_when_a_step_fails_next_steps_arent_run(self, run_step_mock):
        run_step_mock.side_effect = [False, True, False]

        steps = {
            Mode.APPLY: [
                Step("foo", arguments=[]),
                Step("foo", arguments=[ "a"]),
                Step("bar", arguments=[]),
            ],
            Mode.APPLY_CHECK: [
                Step("foo_check"),
            ]
        }
        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
            controller.main(str(Mode.APPLY), root_dir, "copy", None)
            self.assertEqual(2, len(run_step_mock.mock_calls))

    def test_check_does_not_fail_when_no_steps_are_defined(self):
        """
        Consider there is a Package that only does config. Apply check SHOULD NOT fail
        """
        steps = {
            Mode.CONFIG: [
                Step("foo", arguments=[]),
            ],
            Mode.CONFIG_CHECK: [
                Step("foo_check", arguments=[]),
            ]
        }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
                # False means it DID NOT error
                self.assertFalse(controller.main(str(Mode.APPLY_CHECK), root_dir, "/tmp", None))
                self.assertFalse(
                    os.path.exists(f"{controller.get_flag_dir(root_dir)}/ALL_CHECKED")
                )
    def test_check_fails_if_there_are_steps_but_none_ran(self):
        steps = {
                    Mode.CONFIG: [
                        Step("foo", arguments=[]),
                    ],
                    Mode.CONFIG_CHECK: [
                        Step("foo_check", arguments=[]),
                    ]
                }

        with self._setup_for_main(steps) as (container_dir, config_data, root_dir):
                # False means it DID NOT error
                self.assertTrue(controller.main(str(Mode.CONFIG_CHECK), root_dir, "/tmp", None))
                self.assertFalse(
                    os.path.exists(f"{controller.get_flag_dir(root_dir)}/ALL_CHECKED"))

    @mock.patch("skyhook_agent.controller.run_step")
    def test_any_check_failing_fails_run_but_all_checks_run(self, run_step_mock):
        run_step_mock.side_effect = [False, True, False]
        steps = {
            Mode.APPLY: [
                Step("foo.sh", arguments=[]),
                Step("foo.sh", arguments=[ "a"]),
                Step("bar.sh", arguments=[]),
            ],
            Mode.APPLY_CHECK: [
                Step("foo_check.sh", arguments=[]),
                Step("bar_check.sh", arguments=[])
            ]
        }
        with self._setup_for_main(steps) as (_, _, root_dir):
            os.makedirs(controller.get_flag_dir(root_dir), exist_ok=True)
            result = controller.main(str(Mode.APPLY_CHECK), root_dir, "/tmp", None)
            self.assertFalse(os.path.exists(f"{controller.get_flag_dir(root_dir)}/ALL_CHECKED"))
            self.assertTrue(result)

    @mock.patch("skyhook_agent.controller.get_log_file")
    @mock.patch("skyhook_agent.controller.datetime")
    def test_step_logs_are_sent_to_outputs_and_log_file(
        self, datetime_mock, log_file_mock
    ):
        m = mock.MagicMock()
        datetime_mock.now.return_value = m
        m.isoformat.return_value = "isoformat"
        # Need to close the temp file here because CI doesn't like trying to execute it while a file handle is still open
        with tempfile.TemporaryDirectory() as temp_d:
            os.makedirs(f"{temp_d}/skyhook_dir")
            with open(f"{temp_d}/skyhook_dir/foo.sh", "w") as step_file:
                log_file_mock.return_value = f"{temp_d}/log"

                # Make simple step script that outputs to stdout and stderr
                step_file.write(
                    textwrap.dedent(
                        """
                    #!/bin/bash
                    for i in 1 2; do
                        echo "$i"
                        >&2 echo "$i err"
                        sleep $i
                    done
                    """
                    )
                )

            stdout_buff, stderr_buff = (FakeIO(), FakeIO())
            with mock.patch.object(
                controller.sys, "stderr", stderr_buff
            ), mock.patch.object(controller.sys, "stdout", stdout_buff):
                controller.run_step(Step("foo.sh", arguments=[], returncodes=[0], on_host=False), "", temp_d, config_data=self.config_data)

            os.remove(step_file.name)
            with open(f"{temp_d}/log", "r") as log_f:
                # Compare sorted to avoid any issues wrt to sequencing of the async writes
                self.assertEqual(
                    sorted(log_f.read().split("\n")),
                    sorted(
                        [
                            "[out]isoformat ",
                            "[out]isoformat 1",
                            "[out]isoformat 2",
                        ]
                    ),
                )
            with open(f"{temp_d}/log.err", "r") as log_f:
                self.assertEqual(
                    sorted(log_f.read().strip().split("\n")),
                    sorted(
                        [
                            "[err]isoformat",
                            "[err]isoformat 1 err",
                            "[err]isoformat 2 err",
                        ]
                    ),
                )

            self.assertEqual(
                stdout_buff.read_lines(),
                ["[out]isoformat 1", "[out]isoformat 2", "[out]isoformat SUCEEDED: foo.sh ", ""],
            )

            self.assertEqual(
                stderr_buff.read_lines(),
                ["[err]isoformat 1 err", "[err]isoformat 2 err", "[err]isoformat "],
            )
    
    @mock.patch("skyhook_agent.controller.subprocess")
    @mock.patch("skyhook_agent.controller._run")
    def test_older_log_files_are_cleaned_up(self, run_mock, subprocess_mock):
        with tempfile.TemporaryDirectory() as temp_d:
            for i in range(7):
                with open(controller.get_log_file(temp_d, "foo.sh", "", self.config_data, i), "w") as log_file:
                    log_file.write("old log\n")

            controller.run_step(Step("foo.sh", arguments=[], returncodes=[0], on_host=False), temp_d, "", config_data=self.config_data)
            import glob
            self.assertEqual(5, len(glob.glob(controller.get_log_file(temp_d, "foo.sh", "", self.config_data, "*"))))

    @mock.patch("skyhook_agent.controller.run_step")
    def test_self_managed_idempotency_runs_when_flag_exists(self, run_step_mock):
        run_step_mock.return_value = False
        steps = {
            Mode.APPLY: [
                Step("foo.sh", arguments=[]),
                Step("bar.sh", arguments=[ "a"]),
                Step("baz.sh", arguments=[], idempotence=Idempotence.Disabled),
            ],
            Mode.APPLY_CHECK: [
                Step("foo_check.sh"),
            ]
        }
        with self._setup_for_main(steps) as (container_root_dir, config_data, root_dir):
            for step in steps[Mode.APPLY]:
                controller.set_flag(controller.make_flag_path(step, root_dir, config_data))
            controller.main(str(Mode.APPLY), root_dir, "copy_dir", None)

            self.assertEqual(run_step_mock.call_count, 1)
            # run_step_mock.assert_has_calls(
            #     [mock.call(steps[Mode.APPLY][2], root_dir, "copy_dir", config_data)]
            # )
    

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_interrupt_applies_all_commands(self, run_mock, datetime_mock):
        now_mock = mock.MagicMock()
        datetime_mock.now.return_value = now_mock
        now_mock.strftime.return_value = "12345"
        run_mock.return_value = 0
        steps = {
            Mode.APPLY: [
                Step("foo.sh", arguments=[]),
            ],
            Mode.APPLY_CHECK: [
                Step("foo_check.sh", arguments=[]),
            ],
        }
        with self._setup_for_main(steps) as (container_root_dir, config_data, root_dir):
            with set_env(SKYHOOK_RESOURCE_ID="scr-id-1_package_version"):
                controller.main(
                    Mode.INTERRUPT,
                    root_dir,
                    "copy_dir",
                    interrupts.ServiceRestart(["containerd",]).make_controller_input()
                )

        config_data = {
            "package_name": "package",
            "package_version": "version"
        }
        run_mock.assert_has_calls([
            mock.call(["systemctl", "daemon-reload"], controller.get_log_file(root_dir, "interrupts/service_restart_0", "copy_dir", config_data), on_host=True, root_mount=root_dir, write_cmds=True),
            mock.call(["systemctl", "restart", "containerd"], controller.get_log_file(root_dir, "interrupts/service_restart_1", "copy_dir", config_data), on_host=True, root_mount=root_dir, write_cmds=True)
        ])

    @mock.patch("skyhook_agent.controller._run")
    def test_interrupt_isnt_run_when_skyhook_resource_id_flag_is_there(self, run_mock):
        run_mock.return_value = 0
        SKYHOOK_RESOURCE_ID="scr-id-1_package_version"
        with (tempfile.TemporaryDirectory() as dir,
              set_env(SKYHOOK_RESOURCE_ID=SKYHOOK_RESOURCE_ID)):
            os.makedirs(f"{controller.get_skyhook_directory(dir)}/interrupts/flags/{SKYHOOK_RESOURCE_ID}", exist_ok=True)
            with open(f"{controller.get_skyhook_directory(dir)}/interrupts/flags/{SKYHOOK_RESOURCE_ID}/node_restart_0.complete", 'w') as f:
                f.write("")
            controller.do_interrupt(interrupts.NodeRestart().make_controller_input(), dir, "copy_dir", on_host=False)

            run_mock.assert_not_called()

    @mock.patch("skyhook_agent.controller._run")
    def test_interrupt_create_flags_per_cmd(self, run_mock):
        run_mock.return_value = 0
        SKYHOOK_RESOURCE_ID="scr-id-1_package_version"
        with (tempfile.TemporaryDirectory() as dir,
              set_env(SKYHOOK_RESOURCE_ID=SKYHOOK_RESOURCE_ID)):
            interrupt_dir = f"{controller.get_skyhook_directory(dir)}/interrupts/flags/{SKYHOOK_RESOURCE_ID}"
            interrupt = interrupts.ServiceRestart(["foo", "bar"])
            controller.do_interrupt(interrupt.make_controller_input(), dir, "copy_dir", on_host=False)

            for i in range(len(interrupt.interrupt_cmd)):
                self.assertTrue(os.path.exists(f"{interrupt_dir}/{interrupt._type()}_{i}.complete"))

    @mock.patch("skyhook_agent.controller._run")
    def test_interrupt_failures_remove_flag(self, run_mock):
        run_mock.side_effect = [0,1,0]
        SKYHOOK_RESOURCE_ID="scr-id-1_package_version"
        with (tempfile.TemporaryDirectory() as dir,
              set_env(SKYHOOK_RESOURCE_ID=SKYHOOK_RESOURCE_ID)):
            interrupt_dir = f"{controller.get_skyhook_directory(dir)}/interrupts/flags/{SKYHOOK_RESOURCE_ID}"
            interrupt = interrupts.ServiceRestart(["foo", "bar"])
            controller.do_interrupt(interrupt.make_controller_input(), dir, "copy_dir", on_host=False)

            self.assertTrue(os.path.exists(f"{interrupt_dir}/{interrupt._type()}_0.complete"))
            self.assertFalse(os.path.exists(f"{interrupt_dir}/{interrupt._type()}_1.complete"))
            self.assertFalse(os.path.exists(f"{interrupt_dir}/{interrupt._type()}_1.complete"))

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_interrupt_failure_fails_controller(self, run_mock, datetime_mock):
        now_mock = mock.MagicMock()
        datetime_mock.now.return_value = now_mock
        now_mock.strftime.return_value = "12345"
        run_mock.return_value = 1
        steps = {
            Mode.APPLY: [
                Step("foo.sh", arguments=[]),
            ],
            Mode.APPLY_CHECK: [
                Step("foo_check.sh", arguments=[]),
            ],
        }
        with self._setup_for_main(steps) as (container_root_dir, config_data, root_dir):
            with set_env(SKYHOOK_RESOURCE_ID="scr-id-1_package_version"):
                result = controller.main(
                    Mode.INTERRUPT,
                    root_dir,
                    "/tmp",
                    interrupts.ServiceRestart("containerd").make_controller_input()
                )
        config_data = {
            "package_name": "package",
            "package_version": "version"
        }
        run_mock.assert_has_calls([
            mock.call(["systemctl", "daemon-reload"], controller.get_log_file(root_dir, "interrupts/service_restart_0", "copy_dir", config_data), on_host=True, root_mount=root_dir, write_cmds=True)
        ])

        self.assertEqual(result, True)

    @mock.patch("skyhook_agent.controller.datetime")
    @mock.patch("skyhook_agent.controller._run")
    def test_interrupt_makes_config_from_skyhook_resource_id(self, run_mock, datetime_mock):
        now_mock = mock.MagicMock()
        datetime_mock.now.return_value = now_mock
        now_mock.strftime.return_value = "12345"
        run_mock.return_value = 0
        steps = {
            Mode.APPLY: [
                Step("foo.sh", arguments=[]),
            ],
            Mode.APPLY_CHECK: [
                Step("foo_check.sh", arguments=[]),
            ],
        }
        with self._setup_for_main(steps) as (container_root_dir, config_data, root_dir):
            with set_env(SKYHOOK_RESOURCE_ID="scr-id-1_package_version"):
                result = controller.main(
                    Mode.INTERRUPT,
                    root_dir,
                    "/tmp",
                    interrupts.ServiceRestart("containerd").make_controller_input()
                )
        config_data = {
            "package_name": "package",
            "package_version": "version"
        }
        run_mock.assert_has_calls([
            mock.call(["systemctl", "daemon-reload"], controller.get_log_file(root_dir, "interrupts/service_restart_0", "copy_dir", config_data), on_host=True, root_mount=root_dir, write_cmds=True)
        ])

    @mock.patch("skyhook_agent.controller.main")
    def test_interrupt_mode_reads_extra_argument(self, main_mock):
        argv = ["controller.py", str(Mode.INTERRUPT), "root_mount", "copy_dir", "interrupt_data"]
        with set_env(COPY_RESOLV="false"):
            controller.cli(argv)
        
        main_mock.assert_called_once_with(str(Mode.INTERRUPT), "root_mount", "copy_dir", "interrupt_data", False)

    @mock.patch("skyhook_agent.controller.main")
    def test_cli_overlay_always_run_step_is_correct(self, main_mock):
        with set_env(OVERLAY_ALWAYS_RUN_STEP="true", COPY_RESOLV="false"):
            controller.cli(["controller.py", str(Mode.APPLY), "root_mount", "copy_dir"])

        main_mock.assert_called_once_with(str(Mode.APPLY), "root_mount", "copy_dir", None, True)
        main_mock.reset_mock()

        with set_env(OVERLAY_ALWAYS_RUN_STEP="false", COPY_RESOLV="false"):
            controller.cli(["controller.py", str(Mode.APPLY), "root_mount", "copy_dir"])
        main_mock.assert_called_once_with(str(Mode.APPLY), "root_mount", "copy_dir", None, False)

    @mock.patch("skyhook_agent.controller.main")
    @mock.patch("skyhook_agent.controller.shutil")
    def test_cli_COPY_RESOLV(self, shutil_mock, main_mock):
        argv = ["controller.py", str(Mode.APPLY), "root_mount", "copy_dir"]
        with set_env(COPY_RESOLV="true"):
            controller.cli(argv)
        
        shutil_mock.copyfile.assert_called_once()
        shutil_mock.copyfile.reset_mock()

        with set_env(COPY_RESOLV="false"):
            controller.cli(argv)
        
        shutil_mock.copyfile.assert_not_called()

    @mock.patch("skyhook_agent.controller.shutil")
    @mock.patch("skyhook_agent.controller.agent_main")
    @mock.patch("skyhook_agent.controller.config")
    def test_main_checks_for_legacy_mode(self, config_mock, agent_main_mock, shutil_mock):
        with tempfile.TemporaryDirectory() as temp_dir:
            with mock.patch("builtins.open", mock.mock_open(read_data="{}")):
                controller.main(str(Mode.APPLY), temp_dir, "copy_dir", None)
            shutil_mock.copytree.assert_called_once_with("/skyhook-package", f"{temp_dir}/copy_dir", dirs_exist_ok=True)

        shutil_mock.copytree.reset_mock()

        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(f"{temp_dir}/copy_dir")
            # Write a fake config file
            with open(f"{temp_dir}/copy_dir/config.json", "w") as f:
                f.write("")
            
            with mock.patch("builtins.open", mock.mock_open(read_data="{}")):
                controller.main(str(Mode.APPLY), temp_dir, "copy_dir", None)

        shutil_mock.copytree.assert_not_called()

    @mock.patch("skyhook_agent.controller.shutil")
    @mock.patch("skyhook_agent.controller.agent_main")
    @mock.patch("skyhook_agent.controller.config")
    def test_main_doesnt_copy_root_dir_on_uninstall(self, config_mock, agent_main_mock, shutil_mock):
        config_mock.load.return_value = {
            "schema_version": "v1", 
            "root_dir": "/", 
            "expected_config_files": ["configmap"],
            "package_name": "package",
            "package_version": "1.0.0",
            "modes": {
                "apply": [
                    {
                        "name": "a",
                        "path": "a-path",
                        "arguments": [],
                        "returncodes": [0],
                        "on_host": True,
                        "env": {"hello": "world"},
                        "idempotence": False,
                        "upgrade_step": False
                    }
                ], 
                "apply-check": [
                    {
                        "name": "b",
                        "path": "b-path",
                        "arguments": [],
                        "returncodes": [0],
                        "on_host": True,
                        "idempotence": False,
                        "upgrade_step": False
                    }
                ]
            }
        }

        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(f"{temp_dir}/copy_dir")
            # Write a fake config file
            with open(f"{temp_dir}/copy_dir/config.json", "w") as f:
                f.write("")
            
            # This SHOULD NOT ERROR
            for mode in (str(Mode.UNINSTALL), str(Mode.UNINSTALL_CHECK)):
                with mock.patch("builtins.open", mock.mock_open(read_data="{}")):
                    controller.main(mode, temp_dir, "copy_dir", None)

            # This SHOULD ERROR
            with mock.patch("builtins.open", mock.mock_open(read_data="{}")):
                self.assertRaises(controller.SkyhookValidationError, controller.main, str(Mode.APPLY), temp_dir, "copy_dir", None)

    @mock.patch("skyhook_agent.controller.os.path.exists")
    @mock.patch("skyhook_agent.controller.shutil")
    @mock.patch("skyhook_agent.controller.agent_main")
    @mock.patch("skyhook_agent.controller.config")
    def test_main_doesnt_copy_root_dir_on_uninstall(self, config_mock, agent_main_mock, shutil_mock, os_mock):
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(f"{temp_dir}/copy_dir")
            with open(f"{temp_dir}/copy_dir/config.json", "w") as f:
                f.write("{}")

            for mode in (str(Mode.UNINSTALL), str(Mode.UNINSTALL_CHECK)):
                controller.main(mode, temp_dir, "copy_dir", None)
                for call in os_mock.mock_calls:
                    self.assertNotEqual(call, mock.call(f"{temp_dir}/copy_dir/root_dir"))

            # It should copy now
            os_mock.reset_mock()
            os_mock.return_value = True
            controller.main(Mode.APPLY, temp_dir, "copy_dir", None)
            os_mock.assert_has_calls([mock.call(f"{temp_dir}/copy_dir/root_dir")])
