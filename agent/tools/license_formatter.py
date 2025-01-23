#  SPDX-FileCopyrightText: Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
#  SPDX-License-Identifier: Apache-2.0
import sys

license_text=open(sys.argv[1]).read()
file_text=open(sys.argv[2]).read()
if license_text not in file_text:
    license_lines = license_text.split('\n')
    new_lines = []
    added_license = False
    for i, l in enumerate(file_text.split('\n')):
        if l.startswith("# SPDX-FileCopyrightText") or l.startswith("# SPDX-License-Identifier"):
            continue

        if i == 0 and l.startswith("#!"):
            new_lines.append(l)
            continue

        if not added_license:
            new_lines.extend(license_lines)
            added_license = True

        new_lines.append(l)


    print("\n".join(new_lines),end='') 
else:
    print(file_text,end='')