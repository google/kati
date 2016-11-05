#!/bin/sh
#
# Copyright 2016 Google Inc. All rights reserved
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

set -e

log=/tmp/log
mk="$@"

cat <<EOF > Makefile

test: .KATI_NINJA_POOL := test_pool
test:
	echo "PASS"
EOF

${mk} 2>${log}
if [ -e ninja.sh ]; then
  mv build.ninja kati.ninja
  cat <<EOF > build.ninja
pool test_pool
  depth = 1
include kati.ninja
EOF
  ./ninja.sh
fi
if [ -e ninja.sh ]; then
  if ! grep -q "pool = test_pool" kati.ninja; then
    echo "Pool not present in build.ninja"
  fi
fi
