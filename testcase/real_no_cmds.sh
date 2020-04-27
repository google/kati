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
test: out/bad.baz out/good.baz
	@echo "END"
.PHONY: test

out/bad.bar:
	@echo bar >out/bad.bar
	@echo baz >out/bad.baz

out/bad.baz: out/bad.bar

out/good.bar: .KATI_IMPLICIT_OUTPUTS := out/good.baz
out/good.bar:
	@echo bar >out/good.bar
	@echo baz >out/good.baz
EOF

mkdir -p out

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support these warnings, so write the expected output.
  echo 'Makefile:9: warning: target "out/bad.baz" has no commands. Should "out/bad.bar" be using .KATI_IMPLICIT_OUTPUTS?'
  echo 'END'
else
  ${mk} --werror_phony_looks_real --writable=out/ --werror_writable --warn_real_no_cmds 2>&1
fi

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support these warnings, so write the expected output.
  echo 'Makefile:9: *** target "out/bad.baz" has no commands. Should "out/bad.bar" be using .KATI_IMPLICIT_OUTPUTS?'
else
  ${mk} --werror_phony_looks_real --writable=out/ --werror_writable --werror_real_no_cmds 2>&1
fi
