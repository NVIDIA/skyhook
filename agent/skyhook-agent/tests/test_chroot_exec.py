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
from unittest import mock
from skyhook_agent.chroot_exec import _get_process_env, _get_chroot_env


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

    @mock.patch('skyhook_agent.chroot_exec.subprocess.run')
    def test_get_chroot_env_basic_functionality(self, mock_subprocess):
        """Test _get_chroot_env with typical environment output"""
        mock_result = mock.MagicMock()
        mock_result.stdout = "PATH=/usr/bin:/bin\nHOME=/root\nUSER=root\n"
        mock_subprocess.return_value = mock_result
        
        result = _get_chroot_env()
        
        expected = {
            "PATH": "/usr/bin:/bin",
            "HOME": "/root",
            "USER": "root"
        }
        self.assertEqual(result, expected)
        mock_subprocess.assert_called_once_with(["env"], capture_output=True, text=True)
    
    @mock.patch('skyhook_agent.chroot_exec.subprocess.run')
    def test_get_chroot_env_with_multiple_equals(self, mock_subprocess):
        """Test _get_chroot_env correctly handles lines with multiple '=' characters"""
        mock_result = mock.MagicMock()
        mock_result.stdout = "VAR1=value=with=equals\nVAR2=simple_value\n"
        mock_subprocess.return_value = mock_result
        
        result = _get_chroot_env()
        
        expected = {
            "VAR1": "value=with=equals",  # Should split only on first =
            "VAR2": "simple_value"
        }
        self.assertEqual(result, expected)
    
    @mock.patch('skyhook_agent.chroot_exec.subprocess.run')
    def test_get_chroot_env_ignores_lines_without_equals(self, mock_subprocess):
        """Test _get_chroot_env ignores lines that don't contain '='"""
        mock_result = mock.MagicMock()
        mock_result.stdout = "PATH=/usr/bin\ninvalid_line_no_equals\nHOME=/root\n\n"
        mock_subprocess.return_value = mock_result
        
        result = _get_chroot_env()
        
        expected = {
            "PATH": "/usr/bin",
            "HOME": "/root"
        }
        self.assertEqual(result, expected)
    
    @mock.patch('skyhook_agent.chroot_exec.subprocess.run')
    def test_get_chroot_env_with_empty_output(self, mock_subprocess):
        """Test _get_chroot_env with empty subprocess output"""
        mock_result = mock.MagicMock()
        mock_result.stdout = ""
        mock_subprocess.return_value = mock_result
        
        result = _get_chroot_env()
        
        self.assertEqual(result, {})
        mock_subprocess.assert_called_once_with(["env"], capture_output=True, text=True)
    
    @mock.patch('skyhook_agent.chroot_exec.subprocess.run')
    def test_get_chroot_env_with_empty_values(self, mock_subprocess):
        """Test _get_chroot_env handles environment variables with empty values"""
        mock_result = mock.MagicMock()
        mock_result.stdout = "EMPTY_VAR=\nNORM_VAR=value\nANOTHER_EMPTY=\n"
        mock_subprocess.return_value = mock_result
        
        result = _get_chroot_env()
        
        expected = {
            "EMPTY_VAR": "",
            "NORM_VAR": "value", 
            "ANOTHER_EMPTY": ""
        }
        self.assertEqual(result, expected)
    
    @mock.patch('skyhook_agent.chroot_exec.subprocess.run')
    def test_get_chroot_env_with_whitespace_and_special_chars(self, mock_subprocess):
        """Test _get_chroot_env handles values with whitespace and special characters"""
        mock_result = mock.MagicMock()
        mock_result.stdout = "VAR_WITH_SPACES=value with spaces\nSPECIAL_CHARS=!@#$%^&*()\nPATH=/usr/bin:/bin\n"
        mock_subprocess.return_value = mock_result
        
        result = _get_chroot_env()
        
        expected = {
            "VAR_WITH_SPACES": "value with spaces",
            "SPECIAL_CHARS": "!@#$%^&*()",
            "PATH": "/usr/bin:/bin"
        }
        self.assertEqual(result, expected)


if __name__ == '__main__':
    unittest.main() 