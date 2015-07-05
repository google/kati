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

#include "fileutil.h"

#include <errno.h>
#include <glob.h>
#include <limits.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#include <unordered_map>

#include "log.h"

bool Exists(StringPiece filename) {
  CHECK(filename.size() < PATH_MAX);
  struct stat st;
  if (stat(filename.as_string().c_str(), &st) < 0) {
    return false;
  }
  return true;
}

double GetTimestamp(StringPiece filename) {
  CHECK(filename.size() < PATH_MAX);
  struct stat st;
  if (stat(filename.as_string().c_str(), &st) < 0) {
    return -2.0;
  }
  return st.st_mtime;
}

int RunCommand(const string& shell, const string& cmd, bool redirect_stderr,
               string* s) {
  int pipefd[2];
  if (pipe(pipefd) != 0)
    PERROR("pipe failed");
  int pid;
  if ((pid = vfork())) {
    int status;
    close(pipefd[1]);
    while (true) {
      int result = waitpid(pid, &status, WNOHANG);
      if (result < 0)
        PERROR("waitpid failed");

      while (true) {
        char buf[4096];
        ssize_t r = read(pipefd[0], buf, 4096);
        if (r < 0)
          PERROR("read failed");
        if (r == 0)
          break;
        s->append(buf, buf+r);
      }

      if (result != 0) {
        break;
      }
    }
    close(pipefd[0]);

    return status;
  } else {
    close(pipefd[0]);
    if (redirect_stderr) {
      if (dup2(pipefd[1], 2) < 0)
        PERROR("dup2 failed");
    }
    if (dup2(pipefd[1], 1) < 0)
      PERROR("dup2 failed");
    close(pipefd[1]);

    const char* argv[] = {
      shell.c_str(), "-c", cmd.c_str(), NULL
    };
    execvp(argv[0], const_cast<char**>(argv));
  }
  abort();
}

namespace {

class GlobCache {
 public:
  ~GlobCache() {
    for (auto& p : cache_) {
      delete p.second;
    }
  }

  void Get(const char* pat, vector<string>** files) {
    auto p = cache_.emplace(pat, nullptr);
    if (p.second) {
      vector<string>* files = p.first->second = new vector<string>;
      if (strcspn(pat, "?*[\\") != strlen(pat)) {
        glob_t gl;
        glob(pat, GLOB_NOSORT, NULL, &gl);
        for (size_t i = 0; i < gl.gl_pathc; i++) {
          files->push_back(gl.gl_pathv[i]);
        }
        globfree(&gl);
      } else {
        if (Exists(pat))
          files->push_back(pat);
      }
    }
    *files = p.first->second;
  }

 private:
  unordered_map<string, vector<string>*> cache_;
};

}  // namespace

void Glob(const char* pat, vector<string>** files) {
  static GlobCache gc;
  gc.Get(pat, files);
}
