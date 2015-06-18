#include "file.h"

#include <fcntl.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

#include "ast.h"
#include "log.h"
#include "parser.h"

Makefile::Makefile(const string& filename)
    : buf_(NULL), len_(0), mtime_(0), filename_(filename) {
  int fd = open(filename.c_str(), O_RDONLY);
  if (fd < 0) {
    return;
  }

  struct stat st;
  if (fstat(fd, &st) < 0) {
    PERROR("fstat failed for %s", filename.c_str());
  }

  len_ = st.st_size;
  mtime_ = st.st_mtime;
  buf_ = new char[len_];
  ssize_t r = read(fd, buf_, len_);
  if (r != static_cast<ssize_t>(len_)) {
    if (r < 0)
      PERROR("read failed for %s", filename.c_str());
    ERROR("Unexpected read length=%zd expected=%zu", r, len_);
  }

  if (close(fd) < 0) {
    PERROR("close failed for %s", filename.c_str());
  }

  Parse(this);
}

Makefile::~Makefile() {
  delete[] buf_;
  for (AST* ast : asts_)
    delete ast;
}
