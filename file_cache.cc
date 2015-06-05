#include "file_cache.h"

#include <unordered_map>

#include "file.h"

MakefileCacheManager::MakefileCacheManager() {}

MakefileCacheManager::~MakefileCacheManager() {}

class MakefileCacheManagerImpl : public MakefileCacheManager {
 public:
  virtual ~MakefileCacheManagerImpl() {
    for (auto p : cache_) {
      delete p.second;
    }
  }

  virtual Makefile* ReadMakefile(const string& filename) override {
    Makefile* result = NULL;
    auto p = cache_.insert(make_pair(filename, result));
    if (p.second) {
      p.first->second = result = new Makefile(filename);
    } else {
      result = p.first->second;
    }
    return result;
  }

private:
  unordered_map<string, Makefile*> cache_;
};

MakefileCacheManager* NewMakefileCacheManager() {
  return new MakefileCacheManagerImpl();
}
