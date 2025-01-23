#  SPDX-FileCopyrightText: Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
#  SPDX-License-Identifier: Apache-2.0
import unittest
from tempfile import TemporaryDirectory

from jsonschema import ValidationError

from skyhook_agent import config, step

class TestConfig(unittest.TestCase):
    def setUp(self):
        self._config = {
            "schema_version": "v1", 
            "root_dir": "/", 
            "expected_config_files": ["path"],
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
    

    _steps = {
        step.Mode.APPLY: [
            step.Step("a-path", "a", env={"hello": "world"}),
        ],
        step.Mode.APPLY_CHECK: [
            step.Step("b-path", "b"),
        ]
    }

    def test_load(self):
        this_config = self._config.copy()
        with TemporaryDirectory() as temp_dir:
            for conf in this_config["modes"].values():
                with open(f"{temp_dir}/{conf[0]['path']}", "w") as f:
                    f.write("")
            config.load(this_config, step_root_dir=temp_dir)

    def test_dump(self):
        dumped_config = config.dump("package", "1.0.0", "/", self._steps, expected_config_files=["path"])
        self.assertDictEqual(dumped_config, self._config)

    def test_check_smoke(self):
        registry = config.load_schema_registry()
        this_config = self._config.copy()
        config.check(this_config, registry)

    def test_check_error_on_bad_version(self):
        with self.assertRaises(ValidationError):
            config.check({"schema_version": "bad"}, config.load_schema_registry())

    def test_check_error_on_bad_config(self):
        with self.assertRaises(ValidationError):
            config.check({"schema_version": "v1"}, config.load_schema_registry())

    def test_migrate(self):
        """
        Not much of a test right now as there is only one schema version
        """
        this_config = self._config.copy()
        self.assertEqual(config.migrate(this_config), self._config)

    def test_load_schema_restiry(self):
        registry = config.load_schema_registry()
        self.assertIsNotNone(registry.get("v1/skyhook-agent-schema.json"))
        self.assertIsNotNone(registry.get("v1/step-schema.json"))
        self.assertIsNone(registry.get("bad/skyhook-agent-schema.json"))

    def test_package_version_validation(self):
        valid_versions = [
            "1.0.0",
            "1.0.0-dev",
            "1.0.0+build1",
            "2024.12.19"
        ]

        invalid_versions = [
            "1.0",
            "a",
            "1.0.0-",
            "1.0.0_",
            "2024.01.01"
        ]
        registry = config.load_schema_registry()
        this_config = self._config.copy()
        for v in valid_versions:
            this_config["package_version"] = v
            config.check(this_config, registry)

        for v in invalid_versions:
            this_config["package_version"] = v
            with self.assertRaises(ValidationError):
                config.check(this_config, registry)


