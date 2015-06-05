#ifndef STRING_POOL_H_
#define STRING_POOL_H_

#include <string>
#include <vector>

#include "string_piece.h"

using namespace std;

class StringPool {
 public:
  StringPool();
  ~StringPool();

  StringPiece Add(StringPiece s);

 private:
  vector<char*> pool_;
};

#endif  // STRING_POOL_H_
