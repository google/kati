#!/bin/sh
#
# Copyright 2015 Google Inc. All rights reserved
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

export VAR=hoge

cat <<EOF > Makefile
all:
	echo foo
EOF

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  ./ninja.sh
fi

sleep 1
cat <<EOF > Makefile
\$(KATI_deprecated_var VAR4)
\$(KATI_obsolete_var VAR5)
VAR3 := unused
all:
	echo bar
	echo VAR=\$(VAR)
	echo VAR2=\$(VAR2)
	echo VAR3=\$(VAR3)
	echo wildcard=\$(wildcard *.mk)
other:
	echo foo
EOF

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if ! grep regenerating ${log} > /dev/null; then
    echo 'Should be regenerated (Makefile)'
  fi
  ./ninja.sh
fi

export VAR=fuga
${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if ! grep regenerating ${log} > /dev/null; then
    echo 'Should be regenerated (env changed)'
  fi
  ./ninja.sh
fi

export VAR2=OK
${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if ! grep regenerating ${log} > /dev/null; then
    echo 'Should be regenerated (env added)'
  fi
  ./ninja.sh
fi

export VAR3=testing
${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if grep regenerating ${log} >/dev/null; then
    echo 'Should not regenerate (unused env added)'
  fi
  ./ninja.sh
fi

export VAR3=test2
${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if grep regenerating ${log} >/dev/null; then
    echo 'Should not regenerate (unused env changed)'
  fi
  ./ninja.sh
fi

export VAR4=foo
${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if grep regenerating ${log} >/dev/null; then
    echo 'Should not regenerate (deprecated env added)'
  fi
  ./ninja.sh
fi

export VAR5=foo
${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if grep regenerating ${log} >/dev/null; then
    echo 'Should not regenerate (obsolete env added)'
  fi
  ./ninja.sh
fi

export PATH=/random_path:$PATH
${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if ! grep regenerating ${log} > /dev/null; then
    echo 'Should be regenerated (PATH changed)'
  fi
  ./ninja.sh
fi

sleep 1
touch PASS.mk
${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if ! grep regenerating ${log} > /dev/null; then
    echo 'Should be regenerated (wildcard)'
  fi
  ./ninja.sh
fi

sleep 1
touch XXX
${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if grep regenerating ${log}; then
    echo 'Should not be regenerated'
  fi
  ./ninja.sh
fi

${mk} other 2> ${log}
if [ -e ninja.sh ]; then
  if ! grep regenerating ${log} >/dev/null; then
    echo 'Should be regenerated (argument)'
  fi
  ./ninja.sh other
fi
