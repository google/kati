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
  sps.insert(StringPiece("foo"));
  sps.insert(StringPiece("foo"));
  sps.insert(StringPiece("bar"));
  assert(sps.size() == 2);
  assert(sps.count(StringPiece("foo")) == 1);
  assert(sps.count(StringPiece("bar")) == 1);

  assert(StringPiece("hogefugahige") == StringPiece("hogefugahige"));
  assert(StringPiece("hogefugahoge") != StringPiece("hogefugahige"));
  assert(StringPiece("hogefugahige") != StringPiece("higefugahige"));
}
