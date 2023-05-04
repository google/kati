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

cat <<EOF > Makefile
\$(KATI_extra_file_deps a.txt)
all:
	echo foo
EOF

${mk} 2> ${log} || true
if echo "${mk}" | grep -q kati; then
  if grep -q "file does not exist: a.txt" ${log}; then
    echo 'foo'
  else
    echo 'Expected a missing file error message'
  fi
fi
