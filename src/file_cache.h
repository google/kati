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

#ifndef FILE_CACHE_H_
#define FILE_CACHE_H_

#include <string>
#include <unordered_set>

class Makefile;

class MakefileCacheManager {
 public:
  virtual ~MakefileCacheManager();

  virtual const Makefile& ReadMakefile(const std::string& filename) = 0;
  virtual void GetAllFilenames(std::unordered_set<std::string>* out) = 0;

  static MakefileCacheManager& Get();

 protected:
  MakefileCacheManager();
};

#endif  // FILE_CACHE_H_
