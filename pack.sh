#!/bin/sh
#
# Copyright 2015 Google Inc. All rights reserved
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

make kati ckati

rm -fr out/kati
mkdir out/kati
git archive --prefix src/ master | tar -C out/kati -xvf -

cd out/kati
rm src/repo/android.tgz
cp ../../m2n ../../kati ../../ckati .
cd ..
tar -cvzf ../kati.tgz kati
