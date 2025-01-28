#!/bin/bash
if [ -f $PWD/monitor.txt ]; then
    echo "cpu"
    cut -d" " -f1 ~/git_repos/dgx/infra/skyhook-operator/monitor.txt |sed 's/m//g'| sort -rn | head -5
    echo "memory"
    cut -d" " -f2 ~/git_repos/dgx/infra/skyhook-operator/monitor.txt |sed 's/Mi//g'| sort -rn | head -5
    rm $PWD/monitor.txt
fi


pods=$(kubectl get pods -n skyhook-operator | grep skyhook-operator-controller-manager | awk '{print $1}')
while true; do
    for pod in ${pods}; do
        kubectl top pod $pod -n skyhook-operator --no-headers | tr -s ' ' | cut -d" " -f2,3 >> $PWD/monitor.txt
    done
    sleep 2
done
