#!/bin/bash
#
# Copyright 2026 Google Inc. All rights reserved
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http:#www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -u

mk="$@"

cat <<EOF > Makefile
test:
	echo hello
EOF

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support --emit_sandbox_disabled.
  echo "Default: sandbox_disabled NOT found"
  echo "With flag: sandbox_disabled found"
else
  # 1. Default case
  ${mk} --ninja > /dev/null
  if grep -q "sandbox_disabled = true" build.ninja; then
    echo "Default: sandbox_disabled found"
  else
    echo "Default: sandbox_disabled NOT found"
  fi

  # 2. With flag
  ${mk} --ninja --emit_sandbox_disabled > /dev/null
  if grep -q "sandbox_disabled = true" build.ninja; then
    echo "With flag: sandbox_disabled found"
  else
    echo "With flag: sandbox_disabled NOT found"
  fi
fi
