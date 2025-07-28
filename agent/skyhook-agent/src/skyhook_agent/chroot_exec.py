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


import os
import stat
import json
import sys
import subprocess
import shutil


def _get_process_env(container_env: dict, skyhook_env: dict, chroot_env: dict):
    # Set this first with the container environment.
    # We need to do this because the skyhook package could set any env var they want so that needs
    # to get replicated down. BUT we are in distroless so we then need to overwrite with the chroot environment
    # so things like path/user resolution work.
    process_env = dict(container_env)
    # Overwrite the container environment with the chroot environment
    process_env.update(chroot_env)
    # Inject the skyhook environment variables
    process_env.update(skyhook_env)
    return process_env

def chroot_exec(config: dict, chroot_dir: str):
    cmds = config["cmd"]
    no_chmod = config["no_chmod"]
    skyhook_env = config["env"]

    # Capture container environment before chroot
    container_env = dict(os.environ)
   
    if chroot_dir != "local":
        os.chroot(chroot_dir)
    try:
        if not no_chmod:
            # chmod +x the step
            os.chmod(cmds[0], os.stat(cmds[0]).st_mode | stat.S_IXGRP | stat.S_IXUSR | stat.S_IXOTH)
        # Re-import os to get the chroot environment
        import os as chroot_os
        process_env = _get_process_env(container_env, skyhook_env, chroot_os.environ)
        subprocess.run(cmds, check=True, env=process_env)
    except:
        raise


if __name__ == "__main__":
    control_file = sys.argv[1]
    chroot_dir = sys.argv[2]

    with open(control_file, "r") as f:
        config = json.load(f)
    
    chroot_exec(config, chroot_dir)
