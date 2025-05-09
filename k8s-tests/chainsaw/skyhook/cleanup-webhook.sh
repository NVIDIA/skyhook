#!/bin/bash

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


## This script will cleanup the webhook secret and configurations
## Some if you want to remove  
NAMESPACE=${NAMESPACE:-skyhook}
WEBHOOK_SECRET_NAME=${WEBHOOK_SECRET_NAME:-webhook-cert}
VALIDATING_WEBHOOK_CONFIGURATION_NAME=${VALIDATING_WEBHOOK_CONFIGURATION_NAME:-skyhook-operator-validating-webhook}
MUTATING_WEBHOOK_CONFIGURATION_NAME=${MUTATING_WEBHOOK_CONFIGURATION_NAME:-skyhook-operator-mutating-webhook}

# Delete the webhook secret
kubectl delete secret -n $NAMESPACE $WEBHOOK_SECRET_NAME

# Get the webhook configurations
kubectl delete validatingwebhookconfiguration $VALIDATING_WEBHOOK_CONFIGURATION_NAME
kubectl delete mutatingwebhookconfiguration  $MUTATING_WEBHOOK_CONFIGURATION_NAME
