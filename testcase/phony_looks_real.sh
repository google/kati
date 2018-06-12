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
test: foo/bar foo/baz
foo/bar: .KATI_IMPLICIT_OUTPUTS := foo/baz
foo/bar:
	@echo "END"
.PHONY: test foo/bar
EOF

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support these warnings, so write the expected output.
  echo 'Makefile:4: warning: PHONY target "foo/bar" looks like a real file (contains a "/")'
  echo 'Makefile:4: warning: PHONY target "foo/baz" looks like a real file (contains a "/")'
  echo 'END'
else
  ${mk} --warn_phony_looks_real 2>&1
fi

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support these warnings, so write the expected output.
  echo 'Makefile:4: *** PHONY target "foo/bar" looks like a real file (contains a "/")'
else
  ${mk} --werror_phony_looks_real 2>&1
fi
