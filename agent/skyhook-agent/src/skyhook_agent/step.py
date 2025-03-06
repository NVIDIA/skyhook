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








from typing import IO
from enum import Enum
import json
import os

import logging as logger

from .enums import SchemaVersion

"""
THIS FILE IS SYMLINKED IN ./builds/validator/frameworks/ TO NOT COPY CODE... IF WE HAVE MORE OF THIS
WE WILL NEED TO MAKE A MODULE
"""

class Mode(str, Enum):
    UNINSTALL = "uninstall"
    UNINSTALL_CHECK = "uninstall-check"
    UPGRADE = "upgrade"
    UPGRADE_CHECK = "upgrade-check"
    APPLY = "apply"
    APPLY_CHECK = "apply-check"
    CONFIG = "config"
    CONFIG_CHECK = "config-check"
    INTERRUPT = "interrupt"
    POST_INTERRUPT = "post-interrupt"
    POST_INTERRUPT_CHECK = "post-interrupt-check"

    def __str__(self):
        return self.value
    
NON_STEP_MODES = [Mode.INTERRUPT]

APPLY_TO_CHECK = {
    Mode.UNINSTALL: Mode.UNINSTALL_CHECK,
    Mode.UPGRADE: Mode.UPGRADE_CHECK,
    Mode.APPLY: Mode.APPLY_CHECK,
    Mode.CONFIG: Mode.CONFIG_CHECK,
    Mode.POST_INTERRUPT: Mode.POST_INTERRUPT_CHECK
}

CHECK_TO_APPLY = {v: k for k,v in APPLY_TO_CHECK.items()}


class Idempotence(str, Enum):
    Auto = "auto"  # Skyhook fully manages this, your script will be called once or until success
    Disabled = (
        "disabled"  # you do it yourself however you like, called it many times possibly
    )

    @staticmethod
    def validate(i):
        match i:
            case Idempotence.Auto | Idempotence.Disabled:
                return
            case _:
                raise ValueError(f"{i} is not a valid mode")


class Step:
    name: str
    path: str
    arguments: list[str]
    on_host: bool
    returncodes: set[int]
    env: dict[str, str]
    idempotence: Idempotence
    requires_interrupt: bool

    def __init__(self, path: str, name: str = None, arguments: list[str] = None, returncodes: set[int] = [0], idempotence: Idempotence|str = Idempotence.Auto, requires_interrupt: bool=False, on_host: bool=True, env: dict[str, str] = {}) -> None:
        Idempotence.validate(idempotence)
        self.name = name if name is not None else path
        self.path = path
        self.arguments = arguments if arguments is not None else []
        self.on_host = on_host
        self.returncodes = returncodes
        self.env = env
        self.idempotence = idempotence
        self.requires_interrupt = requires_interrupt

    def __eq__(self, other):
        if not isinstance(other, Step):
            return False
        return self.__dict__ == other.__dict__

    def __str__(self) -> str:
        return f'{self.name} {self.path} [{", ".join(self.arguments)}] -> {self.returncodes}: mode {self.idempotence} requires_interrupt: {self.requires_interrupt} on_host: {self.on_host} env: {self.env}'
    
    def __repr__(self) -> str:
        return str(self)
    
    @classmethod
    def load(cls, data: dict) -> 'Step':
        if data.get('env') is None:
            data['env'] = {}
        data['idempotence'] = Idempotence.Disabled if data['idempotence'] else Idempotence.Auto

        upgrade_step = data.pop('upgrade_step', False)
        if upgrade_step:
            return UpgradeStep(**data)
        return cls(**data)
    
    def dump(self) -> dict:
        data = {
            "name": self.name,
            "path": self.path,
            "arguments": self.arguments,
            "returncodes": list(self.returncodes),
            "on_host": self.on_host,
            "idempotence":  self.idempotence == Idempotence.Disabled,
            "upgrade_step": False
        }
        if self.env:
            data['env'] = self.env
        return data

    
class UpgradeStep(Step):
    def __init__(self, path: str, name: str = None, arguments: list[str] = None, returncodes: set[int] = [0], idempotence: Idempotence|str = Idempotence.Auto, requires_interrupt: bool=False, on_host: bool=True, env: dict[str, str] = None) -> None:
        super().__init__(path, name, arguments=arguments, returncodes=returncodes, idempotence=idempotence, requires_interrupt=requires_interrupt, on_host=on_host, env=env)
        if arguments: 
            if len(arguments) != 0:
                raise StepError(f"UpgradeStep {self.name} can not have any arguments, but found: {arguments}")

    def dump(self):
        data = super().dump()
        data['upgrade_step'] = True
        return data
    
class StepError(Exception):
    pass

class Steps:

    @classmethod
    def load(cls, data: dict[str, list[dict]], root_dir: str) -> dict[Mode, list[Step]]:
        steps = {}
        for mode, step_list in data.items():
            steps[Mode(mode)] = [Step.load(step) for step in step_list]
        cls.validate(steps, root_dir=root_dir)

        return steps
    
    @classmethod
    def dump(cls, steps: dict[Mode, list[Step|UpgradeStep]], validate: bool=True, root_dir: str=None) -> dict:
        if validate:
            if root_dir is None:
                raise ValueError("root_dir must be provided to validate steps")
            cls.validate(steps, root_dir)

        return {mode.value: [step.dump() for step in step_list] for mode, step_list in steps.items()}

    @classmethod
    def validate(self, steps: dict[Mode, list[Step|UpgradeStep]], root_dir: str) -> None:
        """
        This method validates that the given steps dict is valid.
        Checks will be searched for by appending {step_name}_check.{extension}
        to the name of each step.
        For every non check mode there has to be at least one matching check mode.
        """
        if all(len(steps.get(mode, [])) == 0 for mode in list(Mode)):
            raise StepError("There are no defined steps.")
        
        check_modes = CHECK_TO_APPLY.keys()
        non_check_modes = APPLY_TO_CHECK.keys()

        if all(mode not in steps for mode in non_check_modes):
            raise StepError(f"There are only check modes defined. You must define at least one of {', '.join(m.name for m in non_check_modes)}")
            
        for mode in [m for m in non_check_modes if m not in NON_STEP_MODES]:
            if steps.get(mode, []) and not steps.get(Mode[f"{mode.name}_CHECK"], []):
                raise StepError(f"Couldn't validate steps. There are no checks for any of the {mode} steps.")
            
        # Validation for UpgradeStep: No args allowed and must be only defined in the 
        # UPGRADE or UPGRADE_CHECK modes
        for mode, step_list in steps.items():
            for step in step_list:
                if mode not in [Mode.UPGRADE, Mode.UPGRADE_CHECK] and isinstance(step, UpgradeStep):
                    raise StepError(f"UpgradeStep {step.path} defined in the {mode} mode but can only be defined in the UPGRADE or UPGRADE_CHECK modes.")
                
        stepSet = set()
        checkSet = set([step.path for mode in check_modes for step in steps.get(mode, [])])

        for step in (step for mode in non_check_modes if mode not in NON_STEP_MODES for step in steps.get(mode, [])):
            name, extension = step.path.rsplit(".", 1) if "." in step.path else (step.path, "")
            target = name + "_check." + extension if extension else name + "_check"
            stepSet.add(target)

        warnings = stepSet.difference(checkSet)
        for name in warnings:
            logger.warning(f" {name} doesn't exist. Checks ensure that all tasks in the step will complete.")

        errors = []
        for mode, step_list in steps.items():
            for step in step_list:
                if not os.path.exists(f"{root_dir}/{step.path}"):
                    errors.append(step.path)
        
        if errors:
            raise ValueError("Following steps did not exist:" + ", ".join(errors))