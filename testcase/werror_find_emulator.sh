#!/bin/bash
#
# Copyright 2017 Google Inc. All rights reserved
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
FOO := \$(shell find does/not/exist -name '*.txt')
all:
EOF

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't use find emulator, or support --werror_find_emulator, so write
  # expected output.
  echo 'find: "does/not/exist": No such file or directory'
  echo 'Nothing to be done for "all".'
  echo 'Clean exit'
else
  ${mk} --use_find_emulator 2>&1 && echo "Clean exit"
fi

if echo "${mk}" | grep -qv "kati"; then
  echo 'find: "does/not/exist": No such file or directory'
else
  ${mk} --use_find_emulator --werror_find_emulator 2>&1 && echo "Clean exit"
fi
