#!/bin/sh

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

# Handle SIGTERM gracefully
cleanup() {
    echo "Received SIGTERM signal, shutting down gracefully..."
    sleep 3
    exit 0
}
trap cleanup SIGTERM

SLEEP_LEN=${SLEEP_LEN:-$(($RANDOM % 5 + 5))}

echo "agentless ["$@"] sleep for ${SLEEP_LEN} and exit with ${EXIT_CODE}"

sleep ${SLEEP_LEN}
exit ${EXIT_CODE}