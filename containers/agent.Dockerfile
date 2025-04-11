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
FROM python:3.12-alpine AS builder

ARG AGENT_VERSION

COPY . /code
WORKDIR /code
RUN echo "AGENT_VERSION=${AGENT_VERSION}"
RUN apk update && apk add bash make build-base gcc python3-dev musl-dev linux-headers
RUN make test
RUN make clean
RUN make venv
RUN make build build_version=${AGENT_VERSION}

FROM nvcr.io/nvidia/distroless/python:3.12-v3.4.10

RUN mkdir -p /skyhook-agent-wheels
COPY --from=builder /code/skyhook-agent/dist/* /skyhook-agent-wheels

RUN pip install /skyhook-agent-wheels/skyhook_agent*.whl

ENTRYPOINT [ "controller" ]