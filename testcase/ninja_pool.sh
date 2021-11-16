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

test: test_pool test_none test_default test_blank test_gomacc

test_pool: .KATI_NINJA_POOL := test_pool
test_pool:
	echo "PASS"

test_none: .KATI_NINJA_POOL := none
test_none:
	echo "PASS"

test_default:
	echo "PASS"

test_blank: .KATI_NINJA_POOL :=
test_blank:
	echo "PASS"

test_gomacc:
	echo ~/goma/gomacc > /dev/null
	echo "PASS"
EOF


# Test with no arguments
${mk} 2>${log}
if [ -e ninja.sh ]; then
  mv build.ninja kati.ninja
  cat <<EOF > build.ninja
pool test_pool
  depth = 1
pool default_pool
  depth = 1
include kati.ninja
EOF
  ./ninja.sh
fi
if [ -e ninja.sh ]; then
  if ! grep -A1 "build test_pool:" kati.ninja | grep -q "pool = test_pool"; then
    echo "test_pool not present for test_pool rule in build.ninja"
  fi
  if grep -A1 "build test_none:" kati.ninja | grep -q "pool ="; then
    echo "unexpected pool present for test_none rule in build.ninja"
  fi
  if grep -A1 "build test_default:" kati.ninja | grep -q "pool ="; then
    echo "unexpected pool present for test_default rule in build.ninja"
  fi
  if grep -A1 "build test_blank:" kati.ninja | grep -q "pool ="; then
    echo "unexpected pool present for test_blank rule in build.ninja"
  fi
  if grep -A1 "build test_blank:" kati.ninja | grep -q "pool ="; then
    echo "unexpected pool present for test_gomacc rule in build.ninja"
  fi
fi

# Test with --default_pool set
args=
if ! echo "${mk}" | grep -qv "kati"; then
  args=--default_pool=default_pool
fi

${mk} ${args} 2>${log}
if [ -e ninja.sh ]; then
  mv build.ninja kati.ninja
  cat <<EOF > build.ninja
pool test_pool
  depth = 1
pool default_pool
  depth = 1
include kati.ninja
EOF
  ./ninja.sh
fi
if [ -e ninja.sh ]; then
  if ! grep -A1 "build test_pool:" kati.ninja | grep -q "pool = test_pool"; then
    echo "test_pool not present for test_pool rule in build.ninja"
  fi
  if grep -A1 "build test_none:" kati.ninja | grep -q "pool = "; then
    echo "unexpected pool present for test_none rule in build.ninja"
  fi
  if ! grep -A1 "build test_default:" kati.ninja | grep -q "pool = default_pool"; then
    echo "default_pool not present for test_default rule in build.ninja"
  fi
  if ! grep -A1 "build test_blank:" kati.ninja | grep -q "pool = default_pool"; then
    echo "default_pool not present for test_blank rule in build.ninja"
  fi
  if ! grep -A1 "build test_gomacc:" kati.ninja | grep -q "pool = default_pool"; then
    echo "default_pool not present for test_gomacc rule in build.ninja"
  fi
fi

# Test with USE_GOMA=true set
${mk} USE_GOMA=true 2>${log}
if [ -e ninja.sh ]; then
  mv build.ninja kati.ninja
  cat <<EOF > build.ninja
pool test_pool
  depth = 1
pool default_pool
  depth = 1
include kati.ninja
EOF
  ./ninja.sh
fi
if [ -e ninja.sh ]; then
  if ! grep -A1 "build test_pool:" kati.ninja | grep -q "pool = test_pool"; then
    echo "test_pool not present for test_pool rule in build.ninja"
  fi
  if grep -A1 "build test_none:" kati.ninja | grep -q "pool = "; then
    echo "unexpected pool present for test_none rule in build.ninja"
  fi
  if ! grep -A1 "build test_default:" kati.ninja | grep -q "pool = local_pool"; then
    echo "local_pool not present for test_default rule in build.ninja"
  fi
  if ! grep -A1 "build test_blank:" kati.ninja | grep -q "pool = local_pool"; then
    echo "local_pool not present for test_blank rule in build.ninja"
  fi
  if grep -A1 "build test_gomacc:" kati.ninja | grep -q "pool = "; then
    echo "unexpected pool present for test_gomacc rule in build.ninja"
  fi
fi
