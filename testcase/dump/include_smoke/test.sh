#!/bin/bash -eux

CKATI="$PWD/ckati"
TMPDIR="$(mktemp -td kati.test.XXXXXX)"
trap 'rm -fr "$TMPDIR"' EXIT
JSON="$TMPDIR/include.json"

cd "$(dirname $(readlink -f $0))"

if ! "$CKATI" -f include_smoke.mk nop --dump_include_graph "$JSON"; then
  exit 1
fi

if [[ "$(cat $JSON | jq '.include_graph | length')" != "3" ]]; then
  exit 1
fi

FILES="$(cat $JSON | jq '.include_graph | .[] | .file' | sort | tr '\n' ' ')"
if [[ "$FILES" != '"bottom.mk" "include_smoke.mk" "middle.mk" ' ]]; then
  exit 1
fi

TOP_INCLUDES="$(cat $JSON \
  | jq '.include_graph | .[] | select(.file == "include_smoke.mk") | .includes | .[]' \
  | tr '\n' ' ')"
if [[ "$TOP_INCLUDES" != '"bottom.mk" "middle.mk" ' ]]; then
  exit 1
fi

echo "OK"
