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
RESULT := \$(KATI_shell_no_rerun cat a.txt)
all:
	echo \$(RESULT)
EOF
else
  cat <<EOF > Makefile
RESULT := \$(shell cat a.txt)
all:
	echo \$(RESULT)
EOF
fi

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  ./ninja.sh
fi

# Only change the file for kati so that make matches kati's broken output of printing foo 2 times.
# ("broken" because the user forgot to add a.txt to $(KATI_extra_file_deps))
if echo "${mk}" | grep -q kati; then
echo bar > a.txt
fi

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if grep -q regenerating ${log}; then
    echo 'Should not be regenerated'
  fi
  ./ninja.sh
fi
