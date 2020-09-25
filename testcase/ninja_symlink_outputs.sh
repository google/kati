#!/bin/bash
#
# Copyright 2020 Google Inc. All rights reserved
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

cat <<EOF >Makefile
dangling_symlink:
	ln -sf nil dangling_symlink
dangling_symlink: .KATI_SYMLINK_OUTPUTS := dangling_symlink

file:
	touch file

resolved_symlink: file
	ln -sf file resolved_symlink
resolved_symlink: .KATI_SYMLINK_OUTPUTS := resolved_symlink

incorrectly_declared_symlink:
	ln -sf nil incorrectly_declared_symlink
incorrectly_declared_symlink: .KATI_SYMLINK_OUTPUTS := something_else

foo bar: file
	ln -sf file foo && cp foo bar
foo: .KATI_SYMLINK_OUTPUTS := foo
bar: .KATI_SYMLINK_OUTPUTS := bar
EOF

all="dangling_symlink resolved_symlink foo bar"
if echo "${mk}" | grep kati > /dev/null; then
  mk="${mk} --use_ninja_symlink_outputs"
fi
${mk} -j1 $all

if [ -e ninja.sh ]; then
  ./ninja.sh -j1 $all

  if ! grep -A1 "build dangling_symlink:" build.ninja | grep -q "symlink_outputs = dangling_symlink"; then
    echo "symlink_outputs not present for dangling_symlink in build.ninja"
  fi
  if ! grep -A1 "build resolved_symlink:" build.ninja | grep -q "symlink_outputs = resolved_symlink"; then
    echo "symlink_outputs not present for resolved_symlink in build.ninja"
  fi
  if grep -A1 "build file:" build.ninja | grep -q "symlink_outputs ="; then
    echo "unexpected symlink_outputs present for file in build.ninja"
  fi

  # Even though this was a multi-output Make rule, Kati generates individual build
  # statements for each of the outputs, therefore the symlink_outputs list is a singleton.
  if ! grep -A1 "build foo: rule" build.ninja | grep -q "symlink_outputs = foo"; then
    echo "symlink_outputs not present for foo in build.ninja"
  fi
  if ! grep -A1 "build bar: rule" build.ninja | grep -q "symlink_outputs = bar"; then
    echo "symlink_outputs not present for bar in build.ninja"
  fi

  ${mk} -j1 "incorrectly_declared_symlink" 2> kati.err
  # The actual error message contains the line number in the Makefile.
  if ! grep "Makefile:" kati.err | grep "undeclared symlink output: something_else"; then
    echo "did not get undeclared symlink out error message"
  fi
fi