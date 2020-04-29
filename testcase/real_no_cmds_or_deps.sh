#!/bin/bash
#
# Copyright 2020 Google Inc. All rights reserved
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
test: out/bad out/good
	@echo "END"
.PHONY: test

out/bad:

out/good:
	@echo bar >out/good
EOF

mkdir -p out

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support these warnings, so write the expected output.
  echo 'Makefile:5: warning: target "out/bad" has no commands or deps that could create it'
  echo 'END'
else
  ${mk} --werror_phony_looks_real --writable=out/ --werror_writable --warn_real_no_cmds_or_deps 2>&1
fi

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support these warnings, so write the expected output.
  echo 'Makefile:5: *** target "out/bad" has no commands or deps that could create it'
else
  ${mk} --werror_phony_looks_real --writable=out/ --werror_writable --werror_real_no_cmds_or_deps 2>&1
fi
