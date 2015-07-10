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

mk="$@"

cat <<EOF > Makefile
all: foo

foo:
	echo foo
EOF
# Pretend to be a very old Makefile.
touch -t 197101010000 Makefile

"$@"

if [ -e .kati_cache.Makefile ]; then
  if ! grep -q 'Cache not found' kati.INFO; then
    echo 'Cache unexpectedly found'
  fi
fi

"$@"

if [ -e .kati_cache.Makefile ]; then
  if ! grep -q 'Cache found' kati.INFO; then
    echo 'Cache unexpectedly not found'
  fi
fi

cat <<EOF >> Makefile
	echo bar
EOF

"$@"

if [ -e .kati_cache.Makefile ]; then
  if ! grep -q 'Cache expired' kati.INFO; then
    echo 'Cache unexpectedly not expired'
  fi
fi
