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

sleep_if_necessary() {
  if [ x$(uname) != x"Linux" -o x"${TRAVIS}" != x"" ]; then
    sleep "$@"
  fi
}

touch xe.mk yc.mk xa.mk yb.mk xd.mk

cat <<EOF > Makefile
include *.mk
all:
	echo foo
EOF

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  ./ninja.sh
fi

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if grep regenerating ${log}; then
    echo 'Should not be regenerated'
  fi
  ./ninja.sh
fi
