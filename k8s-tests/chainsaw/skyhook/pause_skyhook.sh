#!/bin/bash -x

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

skyhook="$1"
pause="$2"

## assert is true or false
if [[ "$pause" != "true" && "$pause" != "false" ]]; then
    echo "pause must be true or false"
    exit 1
fi

kubectl patch skyhook ${skyhook} -p '{"spec":{"pause":'$pause'}}' --type=merge
