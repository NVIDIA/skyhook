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

FROM python:3.12-alpine

RUN mkdir -p /skyhook-agent-wheels
COPY --from=builder /code/skyhook-agent/dist/* /skyhook-agent-wheels

RUN pip install /skyhook-agent-wheels/skyhook_agent*.whl

ENTRYPOINT [ "controller" ]