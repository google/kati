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

cat <<EOF > Makefile
test: out/foo.o
test2:
out/foo.o: foo.c foo.h test2
	@echo "END"
foo.c:
	@exit 0
foo.h: foo.c

.PHONY: test test2
EOF

# TODO: test implicit outputs

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support these warnings, so write the expected output.
  echo 'Makefile:6: warning: writing to readonly directory: "foo.c"'
  echo 'Makefile:7: warning: writing to readonly directory: "foo.h"'
  echo 'END'
else
  ${mk} --writable=out/ 2>&1
fi

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support these warnings, so write the expected output.
  echo 'Makefile:6: *** writing to readonly directory: "foo.c"'
else
  ${mk} --writable=out/ --werror_writable 2>&1
fi
