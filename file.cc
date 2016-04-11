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

#include "file.h"

#include <fcntl.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

#include "fileutil.h"
#include "log.h"
#include "parser.h"
#include "stmt.h"

Makefile::Makefile(const string& filename)
    : mtime_(0), filename_(filename), exists_(false) {
  int fd = open(filename.c_str(), O_RDONLY);
  if (fd < 0) {
    return;
  }

  struct stat st;
  if (fstat(fd, &st) < 0) {
    PERROR("fstat failed for %s", filename.c_str());
  }

  size_t len = st.st_size;
  mtime_ = st.st_mtime;
  buf_.resize(len);
  exists_ = true;
  ssize_t r = HANDLE_EINTR(read(fd, &buf_[0], len));
  if (r != static_cast<ssize_t>(len)) {
    if (r < 0)
      PERROR("read failed for %s", filename.c_str());
    ERROR("Unexpected read length=%zd expected=%zu", r, len);
  }

  if (close(fd) < 0) {
    PERROR("close failed for %s", filename.c_str());
  }

  Parse(this);
}

Makefile::~Makefile() {
  for (Stmt* stmt : stmts_)
    delete stmt;
}
