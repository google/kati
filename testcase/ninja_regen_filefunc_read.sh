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
# Make 4.1 does not support file reading, which was added in 4.2
# We don't actually care though, since we're just testing kati's regen
ifdef KATI
A := \$(file <file_a)
endif
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

sleep 1
echo regen >file_a

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if ! grep regenerating ${log} >/dev/null; then
    echo 'Should be regenerated (file add)'
  fi
  ./ninja.sh
fi

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if grep regenerating ${log}; then
    echo 'Should not be regenerated'
  fi
  ./ninja.sh
fi

sleep 1
echo regen >>file_a

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if ! grep regenerating ${log} >/dev/null; then
    echo 'Should be regenerated (file change)'
  fi
  ./ninja.sh
fi
