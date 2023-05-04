#!/bin/sh
#
# Copyright 2023 Google Inc. All rights reserved
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

set -eu

log=stderr_log
mk="$@"

echo foo > a.txt

if echo "${mk}" | grep -q kati; then
  cat <<EOF > Makefile
all:
	echo \$(KATI_shell_no_rerun echo foo)
EOF
else
  cat <<EOF > Makefile
all:
	echo foo
EOF
fi

${mk} 2> ${log} || true
if grep -q "KATI_shell_no_rerun provides no benefit over regular \$(shell) inside of a rule" ${log}; then
  echo foo
fi
