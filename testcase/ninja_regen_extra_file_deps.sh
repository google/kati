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

touch a.txt b.txt

cat <<EOF > Makefile
EXTRA_DEPS := a.txt b.txt
\$(KATI_extra_file_deps \$(EXTRA_DEPS))
all:
	echo foo
EOF

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  ./ninja.sh
fi

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if grep -q regenerating ${log}; then
    echo 'Should not be regenerated'
  fi
  ./ninja.sh
fi

touch a.txt

${mk} 2> ${log}
if [ -e ninja.sh ]; then
  if ! grep -q regenerating ${log}; then
    echo 'Should have regenerated due to touched file'
  fi
  ./ninja.sh
fi

rm a.txt

# Ignore the error about a.txt missing on this run, we only care that kati tried to regenerate
${mk} 2> ${log} || true
if [ -e ninja.sh ]; then
  if ! grep -q regenerating ${log}; then
    echo 'Should have regenerated due to removed file'
  fi
  ./ninja.sh
fi
