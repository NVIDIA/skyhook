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
from skyhook_agent.chroot_exec import _get_process_env


class TestChrootExec(unittest.TestCase):
    
    def test_get_process_env_basic_functionality(self):
        """Test _get_process_env with non-overlapping keys"""
        container_env = {"CONTAINER_VAR": "container_value"}
        chroot_env = {"CHROOT_VAR": "chroot_value"}
        skyhook_env = {"SKYHOOK_VAR": "skyhook_value"}
        
        result = _get_process_env(container_env, skyhook_env, chroot_env)
        
        expected = {
            "CONTAINER_VAR": "container_value",
            "CHROOT_VAR": "chroot_value",
            "SKYHOOK_VAR": "skyhook_value"
        }
        self.assertEqual(result, expected)
    
    def test_get_process_env_chroot_overrides_container(self):
        """Test that chroot_env overrides container_env for same keys"""
        container_env = {"SAME_VAR": "container_value", "CONTAINER_VAR": "container_value"}
        chroot_env = {"SAME_VAR": "chroot_value", "CHROOT_VAR": "chroot_value"}
        skyhook_env = {"SKYHOOK_VAR": "skyhook_value"}
        
        result = _get_process_env(container_env, skyhook_env, chroot_env)
        
        expected = {
            "SAME_VAR": "chroot_value",  # chroot overrides container
            "CONTAINER_VAR": "container_value",
            "CHROOT_VAR": "chroot_value",
            "SKYHOOK_VAR": "skyhook_value"
        }
        self.assertEqual(result, expected)
    
    def test_get_process_env_skyhook_overrides_all(self):
        """Test that skyhook_env has highest priority and overrides both chroot and container"""
        container_env = {"SAME_VAR": "container_value", "CONTAINER_VAR": "container_value"}
        chroot_env = {"SAME_VAR": "chroot_value", "CHROOT_VAR": "chroot_value"}
        skyhook_env = {"SAME_VAR": "skyhook_value", "SKYHOOK_VAR": "skyhook_value"}
        
        result = _get_process_env(container_env, skyhook_env, chroot_env)
        
        expected = {
            "SAME_VAR": "skyhook_value",  # skyhook overrides both chroot and container
            "CONTAINER_VAR": "container_value",
            "CHROOT_VAR": "chroot_value",
            "SKYHOOK_VAR": "skyhook_value"
        }
        self.assertEqual(result, expected)
    
    def test_get_process_env_with_empty_dicts(self):
        """Test _get_process_env with empty dictionaries"""
        result = _get_process_env({}, {}, {})
        self.assertEqual(result, {})
        
        # Test with only one dict having values
        container_env = {"VAR": "value"}
        result = _get_process_env(container_env, {}, {})
        self.assertEqual(result, {"VAR": "value"})
        
        chroot_env = {"VAR": "value"}
        result = _get_process_env({}, {}, chroot_env)
        self.assertEqual(result, {"VAR": "value"})
        
        skyhook_env = {"VAR": "value"}
        result = _get_process_env({}, skyhook_env, {})
        self.assertEqual(result, {"VAR": "value"})
    
    def test_get_process_env_precedence_order(self):
        """Test complete precedence order: skyhook > chroot > container"""
        container_env = {
            "PATH": "/container/path",
            "HOME": "/container/home",
            "USER": "container_user",
            "ONLY_CONTAINER": "container_only"
        }
        chroot_env = {
            "PATH": "/chroot/path",
            "HOME": "/chroot/home",
            "ONLY_CHROOT": "chroot_only"
        }
        skyhook_env = {
            "PATH": "/skyhook/path",
            "ONLY_SKYHOOK": "skyhook_only"
        }
        
        result = _get_process_env(container_env, skyhook_env, chroot_env)
        
        expected = {
            "PATH": "/skyhook/path",      # skyhook wins
            "HOME": "/chroot/home",       # chroot wins over container
            "USER": "container_user",     # only in container
            "ONLY_CONTAINER": "container_only",
            "ONLY_CHROOT": "chroot_only",
            "ONLY_SKYHOOK": "skyhook_only"
        }
        self.assertEqual(result, expected)
    
    def test_get_process_env_does_not_modify_input_dicts(self):
        """Test that input dictionaries are not modified"""
        container_env = {"VAR": "container"}
        chroot_env = {"VAR": "chroot"}
        skyhook_env = {"VAR": "skyhook"}
        
        # Keep original references
        original_container = container_env.copy()
        original_chroot = chroot_env.copy()
        original_skyhook = skyhook_env.copy()
        
        result = _get_process_env(container_env, skyhook_env, chroot_env)
        
        # Verify input dicts weren't modified
        self.assertEqual(container_env, original_container)
        self.assertEqual(chroot_env, original_chroot)
        self.assertEqual(skyhook_env, original_skyhook)
        
        # Verify result is correct
        self.assertEqual(result, {"VAR": "skyhook"})


if __name__ == '__main__':
    unittest.main() 