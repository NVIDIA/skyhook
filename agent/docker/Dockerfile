FROM python:3.10-alpine as builder

ARG CI_COMMIT_TAG:-0.0.0

COPY . /code
WORKDIR /code
RUN apk add bash
RUN USE_VENV=false /code/cmds.sh setup
RUN USE_VENV=false /code/cmds.sh build ${CI_COMMIT_TAG}

FROM python:3.10-alpine

RUN mkdir -p /skyhook-agent-wheels
COPY --from=builder /code/skyhook-agent/dist/* /skyhook-agent-wheels

RUN pip install /skyhook-agent-wheels/skyhook_agent*.whl

ENTRYPOINT [ "controller" ]