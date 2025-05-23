#!/bin/bash -eu

KATI="${KATI:=$PWD/ckati}"
export KATI

for TESTCASE in testcase/dump/*; do
  if [[ ! -d "$TESTCASE" ]]; then
    continue
  fi

  echo "Running $TESTCASE..."
  "$TESTCASE/test.sh"
done
