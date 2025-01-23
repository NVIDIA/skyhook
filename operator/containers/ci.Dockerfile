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

ARG GO_VERSION

FROM golang:${GO_VERSION}-bookworm as builder

ARG GO_VERSION
ARG PIP_EXTRA_INDEX_URL
ARG GIT_SHA

## https://github.com/opencontainers/image-spec/blob/main/annotations.md
LABEL org.opencontainers.image.base.name="golang:${GO_VERSION}-bookworm" \
      org.opencontainers.image.description="Container used in CI for Skyhook Operator" \
      org.opencontainers.image.revision="${GIT_SHA}" \
      go.version="${GO_VERSION}"

RUN apt-get update && \
    apt-get -y upgrade && \
    apt-get install -y --no-install-recommends \
        python3 python3-pip python3-venv \ 
        gnupg software-properties-common \
        apt-transport-https ca-certificates curl awscli && \
    rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.29/deb/Release.key | \
    gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
RUN echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.29/deb/ /' | \
    tee /etc/apt/sources.list.d/kubernetes.list
RUN apt-get update && apt-get install -y kubectl && rm -rf /var/lib/apt/lists/*

RUN curl -sL https://aka.ms/InstallAzureCLIDeb | bash && \
    python3 -m venv /workspace/venv && \
    /workspace/venv/bin/pip install --upgrade pip && \
    /workspace/venv/bin/pip install NVDevopsUtilities --extra-index-url ${PIP_EXTRA_INDEX_URL}

## install vault and terraform
RUN wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | tee /etc/apt/sources.list.d/hashicorp.list && \
    apt update && apt install vault terraform && \
    setcap -r /usr/bin/vault && \
    vault --version && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

COPY deps.mk go.mod go.sum .
RUN make -f deps.mk && \ 
    go clean -modcache -cache && \
    go mod download && \
    rm go.mod go.sum

## todo still need to mess with path
