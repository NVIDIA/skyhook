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

ARG BUSYBOX_TAG=1.36.1

FROM busybox:${BUSYBOX_TAG}

## might need a better mock than empty folders existing
RUN mkdir -p /skyhook_package/skyhook_dir /skyhook_package/root_dir

ENV EXIT_CODE=0

COPY ./entrypoint.sh /

ENTRYPOINT ["/entrypoint.sh"]
