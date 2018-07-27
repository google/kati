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

set -e
mk="$@"

cat <<EOF >Makefile
all: secondary_dep

secondary_dep: secondary
	@touch \$@
	@echo Made \$@

primary: .KATI_IMPLICIT_OUTPUTS := secondary
primary:
	@touch primary secondary
	@echo Made primary+secondary
EOF

if [[ "${mk}" =~ ^make ]]; then
  echo Made primary+secondary
  echo Made secondary_dep
  echo Made secondary_dep
  echo Nothing to do
else
  ${mk} -j1
  ./ninja.sh -j1 -w dupbuild=err;
  sleep 1
  touch secondary
  ./ninja.sh -j1 -w dupbuild=err;
  sleep 1
  echo Nothing to do
  touch primary
  ./ninja.sh -j1 -w dupbuild=err;
fi
