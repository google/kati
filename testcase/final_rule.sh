#!/bin/bash
#
# Copyright 2018 Google Inc. All rights reserved
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

if echo "${mk}" | grep -q "^make"; then
  # Make doesn't support final assignment
  echo "Makefile:3: *** cannot assign to readonly variable: FOO"
else
  cat <<EOF > Makefile
all: FOO :=$= bar
FOO +=$= foo
all: FOO +=$= baz
all:
EOF

  ${mk} 2>&1 && echo "Clean exit"
fi
