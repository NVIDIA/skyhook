#!/bin/bash

#  SPDX-FileCopyrightText: Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
#  SPDX-License-Identifier: Apache-2.0

#!/bin/sh 
cmd=${1:-help}

projects="skyhook-agent"

venv_path=""
if [ ${USE_VENV:-true} == "true" ]; then
    venv_path="./venv/bin/"
fi

setup () {
    if [ ${USE_VENV:-true} == "true" ]; then
        python3 -m venv ./venv
    fi
    ${venv_path}pip install hatch coverage
    ${venv_path}hatch config set dirs.project "[\"${PWD}\"]"
}

test () {
    coverage=""
    for project in ${projects}; do
        ${venv_path}hatch -p ${project} test --cover-quiet
        coverage="${coverage} ${project}/.coverage"
    done
    ${venv_path}coverage combine ${coverage}
    ${venv_path}coverage report --show-missing
    ${venv_path}coverage xml
}

build () {
    for project in ${projects}; do
        ${venv_path}hatch -p ${project} version ${1:unkown}
        ${venv_path}hatch -p ${project} build -c
    done
}

publish () {
    for project in ${projects}; do
        ${venv_path}hatch -p ${project} publish
    done
}

format () {
    for file in $(find . -name "*.py" | grep -v venv | grep -v pycache); do
        echo $file
        python3 tools/license_formatter.py boilerplate/license_header.py.txt $file > $file.tmp
        mv $file.tmp $file
    done
}

case $cmd in
    help|h|-h|--help)
        echo "One of setup, test, build, publish, format"
        exit 0
    ;;
    setup)
        setup
    ;;
    test)
        test
    ;;
    build)
        build $2
    ;;
    publish)
        publish
    ;;
    format)
        format
    ;;
    *)
        echo "One of setup, test, build, publish, format"
        exit 1
    ;;
esac

