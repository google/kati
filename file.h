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

#ifndef FILE_H_
#define FILE_H_

#include <stdint.h>

#include <string>
#include <vector>

using namespace std;

struct AST;

class Makefile {
 public:
  explicit Makefile(const string& filename);
  ~Makefile();

  const char* buf() const { return buf_; }
  size_t len() const { return len_; }
  const string& filename() const { return filename_; }

  const vector<AST*>& asts() const { return asts_; }
  vector<AST*>* mutable_asts() { return &asts_; }

  bool Exists() const { return buf_; }

 private:
  char* buf_;
  size_t len_;
  uint64_t mtime_;
  string filename_;
  vector<AST*> asts_;
};

#endif  // FILE_H_
