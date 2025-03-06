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


from enum import Enum
import json
import os, sys

from . import step, enums

from referencing import Registry, Resource
from jsonschema import validate, ValidationError

default_schema_directory = f"{os.path.dirname(os.path.abspath(__file__))}/schemas"

def load(data: dict, step_root_dir: str, schema_root: str=default_schema_directory) -> dict:
    registry = load_schema_registry(schema_root=schema_root)
    # Check the data as it comes in that it is valid
    check(data, registry)

    # Migrate to the latest
    data = migrate(data)

    # Check the migrated version is valid
    check(data, registry)

    # Convert the steps section to step classes
    data['modes'] = step.Steps.load(data['modes'], step_root_dir)

    return data


def dump(package_name: str, package_version: str, root_dir: str, steps: dict[step.Mode, list[step.Step|step.UpgradeStep]], expected_config_files: list[str]=[]):
    """
    Only ever dump to the latest schema version
    """
    return {
        "schema_version": enums.get_latest_schema().value,
        "root_dir": root_dir,
        "expected_config_files": expected_config_files,
        "package_name": package_name,
        "package_version": package_version,
        "modes": step.Steps.dump(steps, validate=False, root_dir=root_dir)
    }
        
def check(config: dict, registry: Registry):
    config_schema_version = config['schema_version']
    schema = registry.get(f"{config_schema_version}/skyhook-agent-schema.json")
    
    if schema is None:
        raise ValidationError(f"Unknown skyhook-agent schema version: {config_schema_version}")
    
    validate(config, schema.contents, registry=registry)

        
def migrate(config: dict) -> dict:
    """
    Migrate config up to latest version
    """
    config_schema_version = config['schema_version']
    if config_schema_version ==  enums.get_latest_schema():
        return config
   
    return  ValidationError(f"Unknown skyhook-agent schema version: {config_schema_version}")

def load_schema_registry(schema_root: str=default_schema_directory) -> Registry:
    uri_to_schema = {}
    for dirpath, _, filenames in os.walk(schema_root):
        for filename in filenames:
            if filename.endswith(".json"):
                with open(f"{dirpath}/{filename}") as f:
                    data = Resource.from_contents(json.load(f))
                    uri_to_schema[f"{dirpath.replace(schema_root + '/','')}/{filename}"] = data
    registry = Registry().with_resources(uri_to_schema.items())

    return registry