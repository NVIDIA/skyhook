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

VERSION=${1:-v1.16.2}

## add chart repo
$HELM repo add jetstack https://charts.jetstack.io --force-update

## add cert-manager chart
### since cert-manager will be installed and deleted on every test run
# we need to enable this flag so that the secrets and certs are deleted
# when everything else is deleted. For more info on this flag: 
# https://cert-manager.io/docs/installation/reinstall/

## using --set so this script is not relitive to the current directory
values="--set crds.enabled=true"
$HELM install cert-manager --namespace cert-manager --create-namespace --version ${VERSION} jetstack/cert-manager ${values}