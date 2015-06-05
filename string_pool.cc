#include "string_pool.h"

#include <stdlib.h>

StringPool::StringPool() {
}

StringPool::~StringPool() {
  for (char* b : pool_) {
    free(b);
  }
}

StringPiece StringPool::Add(StringPiece s) {
  char* b = static_cast<char*>(malloc(s.size()));
  memcpy(b, s.data(), s.size());
  pool_.push_back(b);
  return StringPiece(b, s.size());
}
