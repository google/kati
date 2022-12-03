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

#include "io.h"

#include "log.h"

void DumpInt(FILE* fp, int v) {
  size_t r = fwrite(&v, sizeof(v), 1, fp);
  CHECK(r == 1);
}

void DumpString(FILE* fp, std::string_view s) {
  DumpInt(fp, s.size());
  size_t r = fwrite(s.data(), 1, s.size(), fp);
  CHECK(r == s.size());
}

int LoadInt(FILE* fp) {
  int v;
  size_t r = fread(&v, sizeof(v), 1, fp);
  if (r != 1)
    return -1;
  return v;
}

bool LoadString(FILE* fp, std::string* s) {
  int len = LoadInt(fp);
  if (len < 0)
    return false;
  s->resize(len);
  size_t r = fread(&(*s)[0], 1, s->size(), fp);
  if (r != s->size())
    return false;
  return true;
}
