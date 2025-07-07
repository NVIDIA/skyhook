import os
import stat
import json
import sys
import subprocess


def chroot_exec(cmds: list[str], chroot_dir: str):
    if chroot_dir != "local":
        os.chroot(chroot_dir)
    try:
        # chmod +x the step
        os.chmod(cmds[0], os.stat(cmds[0]).st_mode | stat.S_IXGRP | stat.S_IXUSR | stat.S_IXOTH)
        subprocess.run(cmds, shell=True, check=True)
    except:
        raise
        sys.exit(1)


if __name__ == "__main__":
    control_file = sys.argv[1]
    chroot_dir = sys.argv[2]

    with open(control_file, "r") as f:
        cmds = json.load(f)
    
    chroot_exec(cmds, chroot_dir)