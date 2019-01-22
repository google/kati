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
.PHONY: test
test: out/foo
out/foo: bar
	@echo "END"
EOF

touch bar

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support these warnings, so write the expected output.
  echo 'Makefile:4: warning: real file "out/foo" depends on PHONY target "bar"'
  echo 'END'
else
  ${mk} --warn_real_to_phony --top_level_phony 2>&1
fi

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support these warnings, so write the expected output.
  echo 'Makefile:4: *** real file "out/foo" depends on PHONY target "bar"'
else
  ${mk} --werror_real_to_phony --top_level_phony 2>&1
fi
