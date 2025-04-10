#!/bin/bash -xe

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


## need to specify different paths for the helm binary
## depending on whether or not this is being ran in CI
if [ -n "$GITLAB_CI" ]; then
    HELM=/workspace/bin/helm
else 
    HELM=$(which helm)
fi

## remove cert-manager chart
${HELM} delete cert-manager -n cert-manager