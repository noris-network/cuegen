#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")/../examples"

# setup

export SOPS_AGE_KEY=AGE-SECRET-KEY-14QUHLE5A6UNSKNYXLF5ZA26P3NCFX8P68JQ066T7VJ6JW5G8FHWQN4HAUQ
cleanup_demo_cue=0

if [[ ! -f control-repository/control/demo.cue ]]; then
    cleanup_demo_cue=1
    cp control-repository/control/demo.cue.template control-repository/control/demo.cue
fi

trap 'echo "  FAILED"; rm -rf ${tempdir:-/tmp/noSuchDir1234}' EXIT

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

echo remote
cuegen https://github.com/nxcc/cuegen-remote-test.git | grep -q 'field1: test text 123'
cuegen "https://github.com/nxcc/cuegen-remote-test.git?ref=subpath#apps/app_b" | grep -q 'field1: test yaml 5678'
echo "  OK"

(
    echo embed-experiment
    cd embed-experiment
    export CUE_EXPERIMENT=embed
    cuegen | grep -q t0p53cr3t
    CUEGEN_SKIP_DECRYPT=true cuegen | grep example-pass-123
)

# done

if [[ $cleanup_demo_cue == 1 ]]; then
    rm control-repository/control/demo.cue
fi

trap 'rm -rf ${tempdir:-/tmp/noSuchDir1234}' EXIT
echo "all tests successful"
