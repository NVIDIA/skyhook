# SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import unittest

from skyhook_agent.enums import SortableEnum, get_latest_schema

class SchemaVersionForTest(SortableEnum):
    V1 = "v1"
    V2 = "v2"
    V3 = "v3"
    LATEST = "latest"

class TestSchemaVersionForTest(unittest.TestCase):
    def test_equals(self):
        for mode in list(SchemaVersionForTest):
            self.assertEqual(mode, mode)

        # Test equality with strings
        for mode in list(SchemaVersionForTest):
            self.assertEqual(mode, mode.value)

    def test_not_equals(self):
        # Test inequality
        self.assertNotEqual(SchemaVersionForTest.V1, SchemaVersionForTest.V2)
        # With string
        self.assertNotEqual(SchemaVersionForTest.V1, SchemaVersionForTest.V2.value)

    def test_comparisons(self):
        self.assertLess(SchemaVersionForTest.V1, SchemaVersionForTest.V2)
        self.assertGreater(SchemaVersionForTest.V3, SchemaVersionForTest.V2)
        self.assertGreaterEqual(SchemaVersionForTest.V3, SchemaVersionForTest.V2)
        self.assertLessEqual(SchemaVersionForTest.V2, SchemaVersionForTest.V3)

        # Test comparisons with strings
        self.assertLess(SchemaVersionForTest.V1, SchemaVersionForTest.V2.value)

        # Equal values are not less than each other
        self.assertFalse(SchemaVersionForTest.V1 < SchemaVersionForTest.V1)
        self.assertFalse(SchemaVersionForTest.LATEST < SchemaVersionForTest.LATEST)

        # Equal values are not greater than each other
        self.assertFalse(SchemaVersionForTest.V1 > SchemaVersionForTest.V1)
        self.assertFalse(SchemaVersionForTest.LATEST > SchemaVersionForTest.LATEST)

        # Equal values are greater than or equal to each other
        self.assertTrue(SchemaVersionForTest.V1 >= SchemaVersionForTest.V1)
        self.assertTrue(SchemaVersionForTest.LATEST >= SchemaVersionForTest.LATEST)

        # Equal values are less than or equal to each other
        self.assertTrue(SchemaVersionForTest.V1 <= SchemaVersionForTest.V1)
        self.assertTrue(SchemaVersionForTest.LATEST <= SchemaVersionForTest.LATEST)

    def test_latest(self):
        self.assertEqual(SchemaVersionForTest.LATEST, "latest")
        self.assertEqual(SchemaVersionForTest.LATEST, SchemaVersionForTest.LATEST)

        # Test that latest is greater than all other versions
        for mode in list(SchemaVersionForTest):
            if mode != SchemaVersionForTest.LATEST:
                self.assertGreater(SchemaVersionForTest.LATEST, mode)
                self.assertLess(mode, SchemaVersionForTest.LATEST)

        # Test that latest is greater than all other versions with strings
        for mode in list(SchemaVersionForTest):
            if mode != SchemaVersionForTest.LATEST:
                self.assertGreater(SchemaVersionForTest.LATEST, mode.value)
                self.assertLess(mode.value, SchemaVersionForTest.LATEST)

    def test_equality_for_non_versions(self):
        self.assertNotEqual(SchemaVersionForTest.V1, None)
        self.assertNotEqual(SchemaVersionForTest.V1, 1)

    def test_comparison_for_non_versions(self):
        self.assertRaises(TypeError, lambda: SchemaVersionForTest.V1 < 1)

    def test_get_latest_schema(self):
        self.assertEqual(get_latest_schema(SchemaVersionForTest), SchemaVersionForTest.V3)
