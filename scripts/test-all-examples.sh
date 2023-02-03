#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")/../examples"

# setup

export SOPS_AGE_KEY=AGE-SECRET-KEY-14QUHLE5A6UNSKNYXLF5ZA26P3NCFX8P68JQ066T7VJ6JW5G8FHWQN4HAUQ

if [[ ! -f control-repository/control/demo.cue ]]; then
    cp control-repository/control/demo.cue.template control-repository/control/demo.cue
fi

trap "echo '  FAILED'" EXIT

# run tests

echo configmap...
cuegen configmap | grep -q "2023-01-10T12:00:00Z"
echo "  OK"

echo values...
cuegen values | grep -q "7 replicas configured"
echo "  OK"

echo encrypted...
cuegen encrypted | grep -q IEtFWS0tLS0tCk1JSUV2Z0paTXF
echo "  OK"

echo control-repository
cuegen control-repository/control/dev-cluster/wekan-dev/ | grep -q "namespace: cuegen-demo-dev"
cuegen control-repository/control/prod-cluster/wekan-prod/ | grep -q "namespace: cuegen-demo-prod"
cuegen control-repository/control/prod-cluster/wekan-qa/ | grep -q "namespace: cuegen-demo-qa"
echo "  OK"

# done

trap "" EXIT
echo "all tests successful"
