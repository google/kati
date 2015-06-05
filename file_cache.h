#ifndef FILE_CACHE_H_
#define FILE_CACHE_H_

#include <string>

using namespace std;

class Makefile;

class MakefileCacheManager {
 public:
  virtual ~MakefileCacheManager();

  virtual Makefile* ReadMakefile(const string& filename) = 0;

 protected:
  MakefileCacheManager();

};

MakefileCacheManager* NewMakefileCacheManager();

#endif  // FILE_CACHE_H_
