#!/bin/bash
# TODO(ninja): enable once upstream ninja supports validations
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

cat <<EOF >Makefile
all: a c

a:
	echo a

b:
	echo b

a: .KATI_VALIDATIONS := b

c:
	echo c

d: c
	echo d

c: .KATI_VALIDATIONS := d
EOF

all="a b c d"
if echo "${mk}" | grep kati > /dev/null; then
  mk="${mk} --use_ninja_validations"
  all="a c"
fi
${mk} -j1 $all
if [ -e ninja.sh ]; then
  ./ninja.sh -j1 -w dupbuild=err $all
fi  

