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

#include <unordered_map>

#include "file.h"
#include "file_cache.h"

const Makefile& MakefileCacheManager::ReadMakefile(
    const std::string& filename) {
  auto iter = cache_.find(filename);
  if (iter != cache_.end()) {
    return iter->second;
  }
  return (cache_.emplace(filename, filename).first)->second;
}

void MakefileCacheManager::GetAllFilenames(
    std::unordered_set<std::string>* out) {
  for (const auto& p : cache_)
    out->insert(p.first);
  for (const auto& f : extra_file_deps_)
    out->insert(f);
}

void MakefileCacheManager::AddExtraFileDep(std::string_view dep) {
  extra_file_deps_.emplace(dep);
}

MakefileCacheManager& MakefileCacheManager::Get() {
  static MakefileCacheManager instance;
  return instance;
}
