#!/bin/bash
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

set -u

mk="$@"

function build() {
  cat <<EOF > Makefile
FOO $1 bar
.KATI_READONLY $2 FOO
FOO $3 baz
all:
EOF

  echo "Testcase: $1 $2 $3"
  if echo "${mk}" | grep -q "^make"; then
    # Make doesn't support .KATI_READONLY
    echo "Makefile:3: *** cannot assign to readonly variable: FOO"
  else
    ${mk} 2>&1 && echo "Clean exit"
  fi
}

build "=" "=" "="
build "=" "+=" "="
build "=" ":=" "="

build "=" ":=" ":="
build "=" ":=" "+="

build ":=" ":=" ":="
build ":=" ":=" "+="
build ":=" ":=" "="
