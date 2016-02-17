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

#include "file_cache.h"

#include <unordered_map>

#include "file.h"

static MakefileCacheManager* g_instance;

MakefileCacheManager::MakefileCacheManager() {}

MakefileCacheManager::~MakefileCacheManager() {}

MakefileCacheManager* MakefileCacheManager::Get() {
  return g_instance;
}

class MakefileCacheManagerImpl : public MakefileCacheManager {
 public:
  MakefileCacheManagerImpl() {
    g_instance = this;
  }

  virtual ~MakefileCacheManagerImpl() {
    for (auto p : cache_) {
      delete p.second;
    }
  }

  virtual Makefile* ReadMakefile(const string& filename) override {
    Makefile* result = NULL;
    auto p = cache_.emplace(filename, result);
    if (p.second) {
      p.first->second = result = new Makefile(filename);
    } else {
      result = p.first->second;
    }
    return result;
  }

  virtual void GetAllFilenames(unordered_set<string>* out) override {
    for (const auto& p : cache_)
      out->insert(p.first);
  }

private:
  unordered_map<string, Makefile*> cache_;
};

MakefileCacheManager* NewMakefileCacheManager() {
  return new MakefileCacheManagerImpl();
}
