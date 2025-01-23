#!/bin/bash

# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

## This is the list of versions that will be tagged for the test container so it can be
## used in the e2e tests. 
##
## NOTE: CI has to be ran for these changes to take place, meaning you can't use a version
## that you just added in here until you've ran it in CI otherwise you'll get an image
## pull back off as that tag doesn't exist yet.
export TEST_VERSIONS=$(cat <<EOF
1.2
1.2.3
1.2.5
1.2.6
1.3.2
2.0.0
2.0.1
2.1.4
2.3.1-test
3.2.1
3.2.3
3.3
5.4.3
6.0.0
6.2.0
2024.7.7
2024.7.7-test
EOF
)