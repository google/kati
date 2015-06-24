// Copyright 2015 Google Inc. All rights reserved
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build ignore

#include "string_piece.h"

#include <assert.h>

#include <unordered_set>

using namespace std;

int main() {
  unordered_set<StringPiece> sps;
  sps.insert(STRING_PIECE("foo"));
  sps.insert(STRING_PIECE("foo"));
  sps.insert(STRING_PIECE("bar"));
  assert(sps.size() == 2);
  assert(sps.count(STRING_PIECE("foo")) == 1);
  assert(sps.count(STRING_PIECE("bar")) == 1);
}
