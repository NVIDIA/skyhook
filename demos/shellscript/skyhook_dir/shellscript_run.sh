#!/bin/bash

file=$1

if [ -f ${SKYHOOK_DIR}/configmaps/${file}.sh ]; then
    . ${SKYHOOK_DIR}/configmaps/${file}.sh
else
    echo "Could not find file ${SKYHOOK_DIR}/configmaps/${file}.sh was this in the configmap?"
fi
