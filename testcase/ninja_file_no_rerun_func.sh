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

set -e

log=stderr_log
mk="$@"

echo "PASS" >testfile

cat <<EOF > Makefile
ifdef KATI
SUPPORTS_FILE := 1
endif
ifneq (,\$(filter 4.2%,\$(MAKE_VERSION)))
SUPPORTS_FILE := 1
endif

ifdef KATI
  \$(KATI_file_no_rerun >testwrite,PASS)
  RESULT := \$(KATI_file_no_rerun <testwrite)
else
ifdef SUPPORTS_FILE
  \$(file >testwrite,PASS)
  RESULT := \$(file <testwrite)
else
  # Make <4 does not support \$(file ...)
  \$(info Read back: PASS)
  RESULT := PASS
endif
endif

all: ;@echo \$(RESULT)
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
