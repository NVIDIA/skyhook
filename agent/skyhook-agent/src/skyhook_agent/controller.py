#!/bin/python
#  SPDX-FileCopyrightText: Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
#  SPDX-License-Identifier: Apache-2.0

import sys
import os
import subprocess
import base64
import asyncio
from datetime import datetime, timezone
from io import TextIOBase
import time
import json
import os
import shutil
import glob

from skyhook_agent.step import Step, UpgradeStep, Idempotence, Mode, CHECK_TO_APPLY
from skyhook_agent import interrupts, config
from typing import List

import logging as logger

class SkyhookValidationError(Exception):
    pass


buff_size = int(os.getenv("SKYHOOK_AGENT_BUFFER_LIMIT", 1024 * 8))

def _get_env_config() -> tuple[str]:

    # Used as an identifier in operator mode to be able to know if it has already run
    # an interrupt or not. This id is 1:1 with a given skyhook custom resource instance
    SKYHOOK_RESOURCE_ID = os.getenv("SKYHOOK_RESOURCE_ID", "") 

    # This needs to be set to support legacy mode and should be where skyhook files are on the container
    SKYHOOK_DATA_DIR = os.getenv("SKYHOOK_DATA_DIR", "/skyhook-package")

    return SKYHOOK_RESOURCE_ID, SKYHOOK_DATA_DIR

def _get_package_information(config_data: dict) -> tuple[str, str]:
   return config_data["package_name"], config_data["package_version"]

async def _stream_process(
    stream: asyncio.StreamReader, sinks: List[TextIOBase], label: str = "", limit: int=buff_size
):
    """
    Timestamp each line read from stream into the sinks flushing on each write. 
    Each line can optionally be prefixed with the label.
    """

    # This is to be able to add the prefix to the very first data read
    is_first_line = True
    while True:
        try:
            data = str(await stream.read(limit), "UTF-8")
        except asyncio.IncompleteReadError as e:
            data = str(e.partial, "UTF-8").strip()
    
        if not data:
            break

        t = datetime.now().isoformat()
        if is_first_line:
            data = f"{label}{t} {data}"
            is_first_line = False
        data = data.replace("\n", f"\n{label}{t} ")
        for sink in sinks:
            sink.write(data)
            sink.flush()


async def tee(cmd: List[str], stdout_sink_path: str, stderr_sink_path: str, write_cmds=False, **kwargs):
    """
    Run the cmd in a subprocess and keep the stream of stdout/stderr and merge both into
    the sink_path as a log.
    """
    with open(stdout_sink_path, "w") as stdout_sink_f, open(stderr_sink_path, "w") as stderr_sink_f:
        if write_cmds:
            sys.stdout.write(" ".join(cmd) + "\n")
            stdout_sink_f.write(" ".join(cmd) + "\n")
        p = await asyncio.create_subprocess_shell(
            " ".join(cmd),
            limit=buff_size,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            **kwargs,
        )
        await asyncio.gather(
            _stream_process(p.stdout, [sys.stdout, stdout_sink_f], label="[out]"),
            _stream_process(p.stderr, [sys.stderr, stderr_sink_f], label="[err]"),
        )

    return subprocess.CompletedProcess(cmd, await p.wait())

def get_host_path_for_steps(copy_dir: str):
    return f"{copy_dir}/skyhook_dir"

def get_skyhook_directory(root_mount: str) -> str:
    return f"{root_mount}/etc/skyhook"


def get_flag_dir(root_mount: str) -> str:
    return f"{get_skyhook_directory(root_mount)}/flags"

def get_history_dir(root_mount: str) -> str:
    return f"{get_skyhook_directory(root_mount)}/history"

def get_log_file(root_mount: str, step_path: str, copy_dir: str, config_data: dict, timestamp: str=None) -> str:
    if timestamp is None:
        timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%d-%H%M%S")
    package_name, package_current_version = _get_package_information(config_data)
    log_file = f"{root_mount}/var/log/skyhook/{package_name}/{package_current_version}/{step_path.replace(get_host_path_for_steps(copy_dir), '')}-{timestamp}.log"
    os.makedirs(os.path.dirname(log_file), exist_ok=True)
    return log_file

def cleanup_old_logs(log_file_glob: str) -> None:
    """
    Remove all logs except the current and last 4 (5 total)
    """
    log_dir = os.path.dirname(log_file_glob)
    if not os.path.exists(log_dir):
        return
    log_files = sorted(((log_file, os.stat(log_file).st_mtime) for log_file in glob.glob(log_file_glob)), key=lambda x: x[1], reverse=True)
    for log_file, _ in log_files[5:]:
        os.remove(log_file)


def make_flag_path(
        step: Step|UpgradeStep, root_mount: str, config_data: dict
) -> str:
    flag_dir = get_flag_dir(root_mount)
    package_name, package_current_version = _get_package_information(config_data)
    marker = base64.b64encode(bytes(f"{step.arguments}_{step.returncodes}", "utf-8")).decode("utf-8")
    return f"{flag_dir}/{package_name}/{package_current_version}/{step.path}_{marker}"


def set_flag(flag_file: str, msg: str = "") -> None:
    os.makedirs(os.path.dirname(flag_file), exist_ok=True)
    with open(flag_file, "w") as f:
        f.write(msg)


def _run(cmds: list[str], log_path: str, on_host: bool, root_mount: str, write_cmds=False, **kwargs) -> int:
    """
    Synchronous wrapper around the tee command to have logs written to disk
    """
    # "tee" the stdout and stderr to a file to log the step results
    result = asyncio.run(
        tee(
            (["chroot", root_mount] if on_host else []) + cmds,
            log_path,
            f"{log_path}.err",
            write_cmds=write_cmds,
            **kwargs
        )
    )
    return result.returncode


def run_step(
    step: Step|UpgradeStep,
    root_mount: str,
    copy_dir: str,
    config_data: dict
) -> bool:
    """
    Run the given Step via chroot on the given root_mount path if Step.on_host is True.
    Any arguments for the step that start with "env:" will be sourced from their environment variable. 
    Any environment variables that do not exist will fail the run. 
    The following environment variables are also set into the steps execution environment:
        STEP_ROOT: The path on the host to the root directory of all the steps
        SKYHOOK_DIR: The path on the host to where the skyhook is run. This includes all configmaps and any artifacts packaged with the Overlay.

    Args:
        step(Step): Object of class Step.
        root_mount(str): Path to the mount dir
        copy_dir(str): Directory path containing all the step scripts.
        config_data(dict): The config data. Must contain package_name and package_version
    Returns: bool of return codes
    """

    step_path = f"{get_host_path_for_steps(copy_dir)}/{step.path}"

    # Loop through the arguments to check if any need to be replaced by an environment value
    # The pattern for this is "env:{environment variable name}" ie "env:FOO"
    errors = []
    for i, arg in enumerate(step.arguments):
        if arg.startswith("env:"):
            env_var_name = arg.split("env:")[1]
            try:
                step.arguments[i] = os.environ[env_var_name]
            except KeyError:
                ## TODO: would be nice if there were not required or could have defaults
                errors.append(
                    f"{step.path}: Expected environment variable did not exist: {env_var_name}"
                )
    if errors:
        for msg in errors:
            print(msg)
        return True

    subprocess.run(
        (f"chroot {root_mount} " if step.on_host else "") + f"chmod +x {step_path}",
        shell=True,
    )
    time.sleep(1)
    log_file = get_log_file(root_mount, step_path, copy_dir, config_data)
    return_code = _run(
        [step_path, *step.arguments],
        log_file,
        on_host=step.on_host,
        root_mount=root_mount,
        # Make sure to include the original environment here or else things like path resolution dont work
        env=dict(**os.environ, **step.env, **{"STEP_ROOT": get_host_path_for_steps(copy_dir), "SKYHOOK_DIR": copy_dir}),)
    
    cleanup_old_logs(get_log_file(root_mount, step_path, copy_dir, config_data, "*"))
    if return_code not in step.returncodes:
        print(f"FAILED: {step.path} {' '.join(step.arguments)} {return_code}")
        return True
    print(f"SUCEEDED: {step.path} {' '.join(step.arguments)}")
    return False


def check_flag_file(
        step: Step|UpgradeStep, flag_file: str, always_run_step: bool, mode: Mode
) -> bool:
    """
    Checks if the flag file exists.
    Always returns False if mode config is uninstall or idempotency is disabled.
    """
    if os.path.exists(flag_file):
        if always_run_step:
            print(
                f"WARNING: {flag_file} exists for {step.path} but OVERLAY_ALWAYS_RUN_STEP flag is set so running anyway."
            )
            return False
        if mode in [Mode.CONFIG, Mode.UNINSTALL, Mode.UPGRADE]:
            print(
                f"Flag exists but CONFIG, UNINSTALL, and UPGRADE mode don't support idempotence so running step anyway."
            )
            return False
        if step.idempotence == Idempotence.Disabled:
            print(
                f"Flag exists but idempotence is {Idempotence.Disabled} so running step."
            )
            return False
        print(f"Skipping {step.path} because {flag_file} exists.")
        return True
    return False

def get_or_update_history(root_mount: str, config_data: dict, write: bool = False, step: Step|UpgradeStep = None, mode: Mode = None) -> None:
    """
    Manages the history file for tracking version changes, and auditing purposes.

    Args:
        root_mount (str): The root directory.
        write (bool): If True, updates the history file. If False, reads from the history file. Defaults to False.
        step (Step | UpgradeStep): The current step being processed. Required when reading the history.
        mode (Mode): The mode the controller is running in. Required when writing to the history.

    Write Mode:
        Updates the history file with the current version and time based on the given mode. When writing with the
        UNINSTALL_CHECK mode the history file's current version will be updated to be "uninstalled".

    Read Mode:
        Retrieves the current and previous versions from the history file and sets them as environment variables for the step.
        If the step is an UpgradeStep, the versions are also passed as arguments.
    """
    package_name, package_current_version = _get_package_information(config_data)
    # Create history dir if it doesn't already exist
    history_dir = get_history_dir(root_mount)
    os.makedirs(history_dir, exist_ok=True)

    history_file = f"{history_dir}/{package_name}.json"

    # Load existing history if it exists
    history_data = {
        "current-version": "",
        "history": [],
    }
    if os.path.exists(history_file):
        with open(history_file, "r") as f:
            try:
                history_data = json.load(f)
            except json.JSONDecodeError:
                # Move the corrupted file to a backup location
                os.rename(history_file, f"{history_file}.backup")
                print(f"Failed to load existing history, corrupt history file moved to {history_file}.backup...")
    else:
        ## history doesn't exist for this framework so pass to the upgrade step
        ## that it's currently uninstalled
        history_data["current-version"] = "unknown"
        print(f"no existing history found for {package_name}, could be first installation...")

    if write:
        if mode == Mode.UNINSTALL_CHECK:
            # Update history that package was uninstalled
            history_data["current-version"] = "uninstalled"
            history_data["history"].insert(0, {"version": "uninstalled", "time": datetime.now(timezone.utc).isoformat()}) 
        else:
            # Update history
            history_data["current-version"] = package_current_version
            history_data["history"].insert(0, {"version": package_current_version, "time": datetime.now(timezone.utc).isoformat()})

        # Save updated history
        with open(history_file, "w") as f:
            json.dump(history_data, f)
    else:
        # Set from and to versions for upgrade and upgrade-check steps
        step.env["CURRENT_VERSION"] = package_current_version
        step.env["PREVIOUS_VERSION"] =  history_data["current-version"]
        
        if step and isinstance(step, UpgradeStep):
            step.arguments.extend([history_data["current-version"], package_current_version])

def summarize_check_results(results: list[bool], step_data: dict[Mode, list[Step|UpgradeStep]], step_selector: Mode, root_mount: str) -> bool:
    """
    Returning True means there is at least one failure
    """
    flag_dir = get_flag_dir(root_mount)
    if not os.path.exists(flag_dir) or len(results) == 0:
        print("It does not look like you have successfully applied any steps yet.")
        return True

    # Any failure fails the whole thing
    with open(f"{flag_dir}/check_results", "w") as f:
        f.write(
            "\n".join(
                map(" ".join, zip((step.path for step in step_data[step_selector]), map(str, results)))
            )
        )

    if any(results):
        return True

    with open(f"{flag_dir}/{str(step_selector)}_ALL_CHECKED", "w") as f:
        f.write("")

    return False

def do_interrupt(interrupt_data: str, root_mount: str, copy_dir: str, on_host: bool) -> bool:
    """
    Run an interrupt if there hasn't been an interrupt already for the skyhook ID.
    """
    
    SKYHOOK_RESOURCE_ID, _ = _get_env_config()

    # Interrupts don't really have config data we can read from the Package as it is run standalone.
    # So read it off of SKYHOOK_RESOURCE_ID  instead
    # customer-f5a1d42e-74e5-4606-8bbc-b504fbe0074d-1_tuning_2.0.2
    _, package, version = SKYHOOK_RESOURCE_ID.split("_")
    config_data = {
        "package_name": package,
        "package_version": version,
    }

    interrupt = interrupts.inflate(interrupt_data)

    # Check if the interrupt has already been run for this particular skyhook resource
    interrupt_dir = f"{get_skyhook_directory(root_mount)}/interrupts/flags/{SKYHOOK_RESOURCE_ID}"
    os.makedirs(interrupt_dir, exist_ok=True)
    for i, cmd in enumerate(interrupt.interrupt_cmd):
        interrupt_id = f"{interrupt._type()}_{i}"
        interrupt_flag = f"{interrupt_dir}/{interrupt_id}.complete"

        if os.path.exists(interrupt_flag):
            print(f"Skipping interrupt {interrupt_id} because it was already run for {SKYHOOK_RESOURCE_ID}")
            continue

        with open(interrupt_flag, 'w') as f:
            f.write(str(time.time()))

        return_code = _run(
            cmd,
            get_log_file(root_mount, f"interrupts/{interrupt_id}", copy_dir, config_data),
            root_mount=root_mount,
            on_host=on_host,
            write_cmds=True
        )

        if return_code != 0:
            print(f"INTERRUPT FAILED: {cmd} return_code: {return_code}")
            # If this is not removed then we will skip all failing interrupts and it will look
            # like the interrupt was successful when it was not.
            os.remove(interrupt_flag)
            return True
        
    return False

## Remove all step flags after uninstall
def remove_flags(step_data: dict[Mode, list[Step|UpgradeStep]], root_mount: str, config_data: dict) -> None:
    for step in [step for steps in step_data.values() for step in steps]:
        flag_file = make_flag_path(step, root_mount, config_data)
        if os.path.exists(flag_file):  # Check if the file exists before trying to remove it
            os.remove(flag_file)

def main(mode: Mode, root_mount: str, copy_dir: str, interrupt_data: None|str, always_run_step=False):
    if mode not in set(map(str, Mode)):
        logger.warning(f"This version of the Agent doesn't support the {mode} mode. Options are: {','.join(map(str, Mode))}.")
        return False
    
    if mode == Mode.INTERRUPT:
        return do_interrupt(interrupt_data, root_mount, copy_dir, True)
    
    _, SKYHOOK_DATA_DIR = _get_env_config()

    # Check to see if the directory has already been copied down. If it hasn't assume that we
    # are running in legacy mode and copy the directory down.
    if not os.path.exists(f"{root_mount}/{copy_dir}"):
        shutil.copytree(SKYHOOK_DATA_DIR, f"{root_mount}/{copy_dir}", dirs_exist_ok=True)

        # Copy the legacy node files that are created by the operator
        if os.path.exists("/etc/nvidia-bootstrap/node-files"):
            shutil.copytree("/etc/nvidia-bootstrap/node-files/", f"{root_mount}/{copy_dir}/", dirs_exist_ok=True)

    # Read the configuration file
    with open(f"{root_mount}/{copy_dir}/config.json", 'r') as f:
        config_data = config.load(json.load(f), step_root_dir=f"{root_mount}/{copy_dir}/skyhook_dir")
    
    # Some things we DONT want to do in uninstall modes because they alter the state of the system
    # or expect things to exist that don't exist in uninstall mode
    if mode not in (Mode.UNINSTALL, Mode.UNINSTALL_CHECK):
         # Copy the root_dir to the root_mount if it exists to allow packages to populate files easily
        if os.path.exists(f"{root_mount}/{copy_dir}/root_dir"):
            shutil.copytree(f"{root_mount}/{copy_dir}/root_dir", root_mount, dirs_exist_ok=True)

        for f in config_data["expected_config_files"]:
            if not os.path.exists(f"{root_mount}/{copy_dir}/configmaps/{f}"):
                raise SkyhookValidationError(f"Expected config file {f} not found in configmaps directory.")

    return agent_main(mode, root_mount, copy_dir, config_data, interrupt_data, always_run_step)

def agent_main(mode: Mode, root_mount: str, copy_dir: str, config_data: dict, interrupt_data: None|str, always_run_step=False):
            
    # Pull out step_data so it matches with existing code
    step_data = config_data["modes"]
        
    # Make a flag to mark Skyhook has started
    set_flag(f"{get_flag_dir(root_mount)}/START")
    results = []

    # If no steps configured for this mode but being run output warning that this is a no-op
    if not step_data.get(mode, []):
        logger.warning(f" There are no {mode} steps defined. This will be ran as a no-op.")

    for step in step_data.get(mode, []):
        # Make the flag file without the host path argument (first one). This is because in operator world
        # the host path is going to change every time the Skyhook Custom Resource changes so it would
        # look like a step hasn't been run when it fact it had.
        flag_file = make_flag_path(step, root_mount, config_data)

        # If upgrading get the from and to versions from the history file
        # so it can be passed to the upgrade steps via args or environment vars
        if mode == Mode.UPGRADE or mode == Mode.UPGRADE_CHECK:
            get_or_update_history(root_mount, config_data, step=step)

        if not str(mode).endswith("-check"):
            if check_flag_file(step, flag_file, always_run_step, mode):
                continue
            print(f"{mode} {step.path} {step.arguments} {step.returncodes} {step.idempotence} {step.on_host}")

            failed = run_step(step, root_mount, copy_dir, config_data)
            if failed:
                return True

            set_flag(
                flag_file,
                f"last_run: {datetime.now().isoformat()}\nstep_always_runs: {step.idempotence == Idempotence.Disabled}",
            )
        else:
            print(f"{mode} {step.path} {step.arguments} {step.returncodes} {step.idempotence} {step.on_host}")
            results.append(run_step(step, root_mount, copy_dir, config_data))
                    

    if mode in CHECK_TO_APPLY and len(step_data.get(mode, [])) > 0:
        if summarize_check_results(results, step_data, mode, root_mount):
            return True

    ## If APPLY_CHECK, UPGRADE_CHECK, or UNINSTALL_CHECK finished successfully update installed version history
    if mode in [Mode.APPLY_CHECK, Mode.UPGRADE_CHECK, Mode.UNINSTALL_CHECK]:
        get_or_update_history(root_mount, config_data, write=True, mode=mode)

        ## We also want to remove the flags if the package was uninstalled
        if mode == Mode.UNINSTALL_CHECK:
            remove_flags(step_data, root_mount, config_data)

    return False

def cli(sys_argv: list[str]=sys.argv):
    # Old way
    # controller mode copy_dir interrupt_data

    # new way
    # controller ${mode} ${mount_dir} ${copy_dir} ${interrupt_data}
    args = sys_argv[1:]
    interrupt_data = None

    if len(args) == 2:
        # old way
        mode, copy_dir = args
        root_mount = "/root"

    if len(args) == 3:
        if args[0] == str(Mode.INTERRUPT):
            # old way
            mode, copy_dir, interrupt_data = args
            root_mount = "/root"
        else:
            # new way
            mode, root_mount, copy_dir = args

    if len(args) == 4:
        # new way with interrupt data
        mode, root_mount, copy_dir, interrupt_data = args

    if os.getenv("COPY_RESOLV", "true").lower() == "true":
        shutil.copyfile("/etc/resolv.conf", f"{root_mount}/etc/resolv.conf")

    always_run_step = os.getenv("OVERLAY_ALWAYS_RUN_STEP", "false").lower() == "true"

    return main(mode, root_mount, copy_dir, interrupt_data, always_run_step)


if __name__ == "__main__":
    if cli(sys.argv):
        sys.exit(1)
