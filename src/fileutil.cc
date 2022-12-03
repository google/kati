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
#include <fcntl.h>
#include <glob.h>
#include <limits.h>
#include <signal.h>
#include <spawn.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>
#if defined(__APPLE__)
#include <mach-o/dyld.h>
#endif

#include <unordered_map>

#include "log.h"
#include "strutil.h"

extern char** environ;

bool Exists(std::string_view filename) {
  CHECK(filename.size() < PATH_MAX);
  struct stat st;
  if (stat(std::string(filename).c_str(), &st) < 0) {
    return false;
  }
  return true;
}

double GetTimestampFromStat(const struct stat& st) {
#if defined(__linux__)
  return st.st_mtime + st.st_mtim.tv_nsec * 0.001 * 0.001 * 0.001;
#else
  return st.st_mtime;
#endif
}

double GetTimestamp(std::string_view filename) {
  CHECK(filename.size() < PATH_MAX);
  struct stat st;
  if (stat(std::string(filename).c_str(), &st) < 0) {
    return -2.0;
  }
  return GetTimestampFromStat(st);
}

int RunCommand(const std::string& shell,
               const std::string& shellflag,
               const std::string& cmd,
               RedirectStderr redirect_stderr,
               std::string* s) {
  const char* argv[] = {NULL, NULL, NULL, NULL};
  std::string cmd_with_shell;
  if (shell[0] != '/' || shell.find_first_of(" $") != std::string::npos) {
    std::string cmd_escaped = cmd;
    EscapeShell(&cmd_escaped);
    cmd_with_shell = shell + " " + shellflag + " \"" + cmd_escaped + "\"";
    argv[0] = "/bin/sh";
    argv[1] = "-c";
    argv[2] = cmd_with_shell.c_str();
  } else {
    // If the shell isn't complicated, we don't need to wrap in /bin/sh
    argv[0] = shell.c_str();
    argv[1] = shellflag.c_str();
    argv[2] = cmd.c_str();
  }

  int pipefd[2];
  if (pipe(pipefd) != 0)
    PERROR("pipe failed");

  posix_spawn_file_actions_t action;
  int err = posix_spawn_file_actions_init(&action);
  if (err != 0) {
    ERROR("posix_spawn_file_actions_init: %s", strerror(err));
  }

  err = posix_spawn_file_actions_addclose(&action, pipefd[0]);
  if (err != 0) {
    ERROR("posix_spawn_file_actions_addclose: %s", strerror(err));
  }

  if (redirect_stderr == RedirectStderr::STDOUT) {
    err = posix_spawn_file_actions_adddup2(&action, pipefd[1], 2);
    if (err != 0) {
      ERROR("posix_spawn_file_actions_adddup2: %s", strerror(err));
    }
  } else if (redirect_stderr == RedirectStderr::DEV_NULL) {
    err =
        posix_spawn_file_actions_addopen(&action, 2, "/dev/null", O_WRONLY, 0);
    if (err != 0) {
      ERROR("posix_spawn_file_actions_addopen: %s", strerror(err));
    }
  }
  err = posix_spawn_file_actions_adddup2(&action, pipefd[1], 1);
  if (err != 0) {
    ERROR("posix_spawn_file_actions_adddup2: %s", strerror(err));
  }
  err = posix_spawn_file_actions_addclose(&action, pipefd[1]);
  if (err != 0) {
    ERROR("posix_spawn_file_actions_addclose: %s", strerror(err));
  }

  posix_spawnattr_t attr;
  err = posix_spawnattr_init(&attr);
  if (err != 0) {
    ERROR("posix_spawnattr_init: %s", strerror(err));
  }

  short flags = 0;
#ifdef POSIX_SPAWN_USEVFORK
  flags |= POSIX_SPAWN_USEVFORK;
#endif

  err = posix_spawnattr_setflags(&attr, flags);
  if (err != 0) {
    ERROR("posix_spawnattr_setflags: %s", strerror(err));
  }

  pid_t pid;
  err = posix_spawn(&pid, argv[0], &action, &attr, const_cast<char**>(argv),
                    environ);
  if (err != 0) {
    ERROR("posix_spawn: %s", strerror(err));
  }

  err = posix_spawnattr_destroy(&attr);
  if (err != 0) {
    ERROR("posix_spawnattr_destroy: %s", strerror(err));
  }
  err = posix_spawn_file_actions_destroy(&action);
  if (err != 0) {
    ERROR("posix_spawn_file_actions_destroy: %s", strerror(err));
  }

  int status;
  close(pipefd[1]);
  while (true) {
    int result = waitpid(pid, &status, WNOHANG);
    if (result < 0)
      PERROR("waitpid failed");

    while (true) {
      char buf[4096];
      ssize_t r = HANDLE_EINTR(read(pipefd[0], buf, 4096));
      if (r < 0)
        PERROR("read failed");
      if (r == 0)
        break;
      s->append(buf, buf + r);
    }

    if (result != 0) {
      break;
    }
  }
  close(pipefd[0]);

  return status;
}

std::string GetExecutablePath() {
#if defined(__linux__)
  char mypath[PATH_MAX + 1];
  ssize_t l = readlink("/proc/self/exe", mypath, PATH_MAX);
  if (l < 0) {
    PERROR("readlink for /proc/self/exe");
  }
  mypath[l] = '\0';
  return {mypath};
#elif defined(__APPLE__)
  char mypath[PATH_MAX + 1];
  uint32_t size = PATH_MAX;
  if (_NSGetExecutablePath(mypath, &size) != 0) {
    ERROR("_NSGetExecutablePath failed");
  }
  mypath[size] = 0;
  return {mypath};
#else
#error "Unsupported OS"
#endif
}

namespace {

class GlobCache {
 public:
  ~GlobCache() { Clear(); }
  const GlobMap::mapped_type& Get(const char* pat) {
    auto [it, inserted] = cache_.try_emplace(pat);
    auto& files = it->second;
    if (inserted) {
      if (strcspn(pat, "?*[\\") != strlen(pat)) {
        glob_t gl;
        glob(pat, 0, NULL, &gl);
        for (size_t i = 0; i < gl.gl_pathc; i++) {
          files.push_back(gl.gl_pathv[i]);
        }
        globfree(&gl);
      } else {
        if (Exists(pat))
          files.push_back(pat);
      }
    }
    return files;
  }

  const GlobMap& GetAll() const { return cache_; }

  void Clear() { cache_.clear(); }

 private:
  GlobMap cache_;
};

static GlobCache g_gc;

}  // namespace

const GlobMap::mapped_type& Glob(const char* pat) {
  return g_gc.Get(pat);
}

const GlobMap& GetAllGlobCache() {
  return g_gc.GetAll();
}

void ClearGlobCache() {
  g_gc.Clear();
}
