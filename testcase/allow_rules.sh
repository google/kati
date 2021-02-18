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
FOO = A \$(BAR) C
BAR = B

# Should not issue warnings or errors
a:
	echo $@

# Should issue a warning
.KATI_ALLOW_RULES := warning
b:
	echo $@

\$(FOO) :
	echo $@

# Should not issue warnings or errors
.KATI_ALLOW_RULES := asdfasdfa
d:
	echo $@

# Should not issue warnings or errors
.KATI_ALLOW_RULES := 
e:
	echo $@

# Should issue an error
.KATI_ALLOW_RULES := error
c:
	echo $@
EOF

if echo "${mk}" | grep -qv "kati"; then
  # Make doesn't support these warnings, so write the expected output.
  echo 'Makefile:10: warning: Rule not allowed here for target: b'
  echo 'Makefile:13: warning: Rule not allowed here for target: A B C'
  echo 'Makefile:28: *** Rule not allowed here for target: c'
else
  ${mk} --no_builtin_rules --warn 2>&1
fi

