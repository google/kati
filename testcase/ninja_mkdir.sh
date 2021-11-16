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

log=stderr_log
mk="$@"

cat <<EOF > Makefile
test: a/b

a/b:
	@mkdir -p \$(dir \$@)
	touch \$@
EOF

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  ./ninja.sh
fi
if [[ ! -d a ]]; then
  echo "Created 'a'"
fi
if [ -e ninja.sh ]; then
  if grep -q "mkdir -p" build.ninja; then
    echo "Should not include 'mkdir -p' in build.ninja"
    echo "Ninja will automatically create this directory"
  fi
fi
