# SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM python:3.12-bookworm AS builder

ARG AGENT_VERSION

COPY . /code
WORKDIR /code
RUN echo "AGENT_VERSION=${AGENT_VERSION}"
RUN apt-get update && apt-get install -y \
    bash \
    make \
    build-essential \
    gcc \
    python3-dev \
    linux-headers-generic
#RUN make test
RUN make clean
RUN make venv
RUN make build build_version=${AGENT_VERSION}

# Install the wheel in the builder stage
RUN python3 -m venv venv && ./venv/bin/pip install /code/skyhook-agent/dist/skyhook_agent*.whl

FROM nvcr.io/nvidia/distroless/python:3.12-v3.4.10

ARG AGENT_VERSION
ARG GIT_SHA

## https://github.com/opencontainers/image-spec/blob/main/annotations.md
LABEL org.opencontainers.image.base.name="nvcr.io/nvidia/distroless/python:3.12-v3.4.10" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="skyhook-agent" \
      org.opencontainers.image.version="${AGENT_VERSION}" \
      org.opencontainers.image.revision="${GIT_SHA}"

# Copy the installed packages and scripts from builder
COPY --from=builder /code/venv/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY --from=builder /code/venv/bin/controller /usr/local/bin/

# Run as root so we can chroot
USER 0:0

# Use Python to run the controller script
ENTRYPOINT [ "python", "-m", "skyhook_agent.controller" ]
