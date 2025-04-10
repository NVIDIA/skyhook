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


import unittest
import base64
import json

from skyhook_agent import interrupts

class TestInterrupts(unittest.TestCase):
    def test_make_map(self):
        interrupt_map = interrupts._make_map()

        for k, v in {"node_restart": interrupts.NodeRestart, "service_restart": interrupts.ServiceRestart}.items():
            self.assertEqual(interrupt_map[k], v)

    def test_name_creation_on_class(self):
        self.assertEqual(interrupts.NodeRestart._type(), "node_restart")

    def test_name_as_property(self):
        self.assertEqual(interrupts.NodeRestart().type, "node_restart")

    def test_node_restart_command(self):
        self.assertEqual([["reboot",]], interrupts.NodeRestart().interrupt_cmd)

    def test_service_restart_command(self):
         self.assertEqual(
             [["systemctl", "daemon-reload"],
              ["systemctl", "restart", "foo"],
              ["systemctl", "restart", "bar"]],
             interrupts.ServiceRestart(["foo", "bar"]).interrupt_cmd)

    def test_round_trip_serialization(self):
        starts = [
            interrupts.ServiceRestart(["containerd"]),
            interrupts.NodeRestart(),
            interrupts.ScriptInterrupt(),
            interrupts.RestartAllServices(),
            interrupts.NoOp()
        ]
        for start in starts:
            ends = [
                interrupts.inflate(base64.b64encode(str.encode(json.dumps(start.serialize()), 'utf-8'))),
                interrupts.inflate(start.make_controller_input())
            ]

            for i, end in enumerate(ends):
                self.assertEqual(start.type, end.type, f"{start.type} end {i}")
                self.assertEqual(start.data, end.data, f"{start.type} end {i}")
                self.assertEqual(start.interrupt_cmd, end.interrupt_cmd, f"{start.type} end {i}")
