#!/bin/bash
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
if echo "${mk}" | grep kati > /dev/null; then
  mk="${mk} --use_find_emulator"
fi
function build() {
  ${mk} $@ 2> /dev/null
  if [ -e ninja.sh ]; then ./ninja.sh -j1 $@; fi
}

cat <<EOF > Makefile
V := \$(shell find -L linkdir/d/link)
all:
	@echo \$(V)
EOF

mkdir -p dir1 dir2 linkdir/d
touch dir1/file1 dir2/file2
ln -s ../../dir1 linkdir/d/link
build

sleep 1
touch dir1/file1_2
build

rm linkdir/d/link
ln -s ../../dir2 linkdir/d/link
build
