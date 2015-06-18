#ifndef FILE_H_
#define FILE_H_

#include <stdint.h>

#include <string>
#include <vector>

#include "string_pool.h"

using namespace std;

class AST;

class Makefile {
 public:
  explicit Makefile(const string& filename);
  ~Makefile();

  const char* buf() const { return buf_; }
  size_t len() const { return len_; }
  const string& filename() const { return filename_; }

  StringPool* mutable_pool() { return &pool_; }
  const vector<AST*>& asts() const { return asts_; }
  vector<AST*>* mutable_asts() { return &asts_; }

  bool Exists() const { return buf_; }

 private:
  char* buf_;
  size_t len_;
  uint64_t mtime_;
  string filename_;
  StringPool pool_;
  vector<AST*> asts_;
};

#endif  // FILE_H_
