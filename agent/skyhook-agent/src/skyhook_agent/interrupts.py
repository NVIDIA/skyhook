#  SPDX-FileCopyrightText: Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
#  SPDX-License-Identifier: Apache-2.0
import string
import base64
import json
import inspect
import sys

class Interrupt(object):
    data: dict[str, any] = {}

    # These are the command(s) to be run to perform the interrupt. 
    # It is a list of subprocess.run list form.
    # Set this to empty list if you do NOT want anything run.

    interrupt_cmd: list[list[str]]

    @classmethod
    def _type(cls):
        return ''.join(c.lower() if c in string.ascii_lowercase or i == 0 else f"_{c.lower()}" for i,c in enumerate(cls.__name__))

    @property
    def type(self):
        return self._type()

    def serialize(self) -> dict[str, str|dict[str,str]]:
        return dict(type=self.type, **self.data)
    
    def make_controller_input(self) -> str:
        """
        Available as an example of the expected input to the inflate method
        """
        return base64.b64encode(str.encode(json.dumps(self.serialize()), 'utf-8'))

class NodeRestart(Interrupt):
    """
    Operator will do the interrupt for you by restarting the node.
    """
    
    interrupt_cmd = [["reboot",]]

class ServiceRestart(Interrupt):
    """
    Operator will do the interrupt for you by restarting the given service.
    """

    def __init__(self, services: list[str]):
        self.data = {
            "services": services
        }

        self.interrupt_cmd = [
            ["systemctl", "daemon-reload"]
        ]
        self.interrupt_cmd += [["systemctl", "restart", s] for s in services]

class RestartAllServices(Interrupt):
    """
    Operator will do the interrupt for you by restarting all services
    """

    interrupt_cmd = [
        ["service" ,"procps", "force-reload"]
    ]

class NoOp(Interrupt):
    """
    Operator will do nothing
    """
    interrupt_cmd = []

class ScriptInterrupt(Interrupt):
    """
    The apply script will do the interrupt.
    """
    interrupt_cmd = []

### DO NOT PUT INTERRUPTS BELOW THIS

def _make_map():
    """
    Generate a map of all classes above this to use with inflate.
    """
    interrupt_map = {}
    for _, member in inspect.getmembers(sys.modules[__name__], lambda x: inspect.isclass(x) and issubclass(x, Interrupt) and x != Interrupt):
        interrupt_map[member._type()] = member

    return interrupt_map


def inflate(serialized_value: str) -> Interrupt:
    """
    Convert base64 encoded Interrupt.serialize() back into its original 
    Interrupt.
    """
    interrupt_map = _make_map()

    try:
        serialized_data = json.loads(base64.b64decode(serialized_value))
    except:
        raise ValueError("Serialized interrupt must be base64 encoded {'type': str, **kwargs: dict[str, any]}")

    try:
        interrupt_type = serialized_data['type']
    except KeyError:
        raise ValueError("Serialized interrupt must be base64 encoded {'type': str, **kwargs: dict[str, any]}")

    interrupt_data = dict((k,v) for k, v in serialized_data.items() if k != "type")
    try:
        interrupt = interrupt_map[interrupt_type](**interrupt_data)
    except KeyError:
        raise ValueError(f"Unknown interrupt {interrupt_type} must be one of: {', '.join(interrupt_map.keys())}")
    
    return interrupt

