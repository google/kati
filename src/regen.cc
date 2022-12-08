// Copyright 2016 Google Inc. All rights reserved
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

#include "regen.h"

#include <sys/stat.h>

#include <algorithm>
#include <future>
#include <memory>
#include <mutex>
#include <vector>

#include "affinity.h"
#include "fileutil.h"
#include "find.h"
#include "func.h"
#include "io.h"
#include "log.h"
#include "ninja.h"
#include "stats.h"
#include "strutil.h"

namespace {

#define RETURN_TRUE              \
  do {                           \
    if (g_flags.dump_kati_stamp) \
      needs_regen_ = true;       \
    else                         \
      return true;               \
  } while (0)

bool ShouldIgnoreDirty(std::string_view s) {
  Pattern pat(g_flags.ignore_dirty_pattern ? g_flags.ignore_dirty_pattern : "");
  Pattern nopat(
      g_flags.no_ignore_dirty_pattern ? g_flags.no_ignore_dirty_pattern : "");
  return pat.Match(s) && !nopat.Match(s);
}

class StampChecker {
  struct GlobResult {
    std::string pat;
    std::vector<std::string> result;
  };

  struct ShellResult {
    CommandOp op;
    std::string shell;
    std::string shellflag;
    std::string cmd;
    std::string result;
    std::vector<std::string> missing_dirs;
    std::vector<std::string> files;
    std::vector<std::string> read_dirs;
  };

 public:
  StampChecker() : needs_regen_(false) {}

  ~StampChecker() {
    for (GlobResult* gr : globs_) {
      delete gr;
    }
    for (ShellResult* sr : commands_) {
      delete sr;
    }
  }

  bool NeedsRegen(double start_time, const std::string& orig_args) {
    if (IsMissingOutputs())
      RETURN_TRUE;

    if (CheckStep1(orig_args))
      RETURN_TRUE;

    if (CheckStep2())
      RETURN_TRUE;

    if (!needs_regen_) {
      FILE* fp = fopen(GetNinjaStampFilename().c_str(), "rb+");
      if (!fp)
        return true;
      ScopedFile sfp(fp);
      if (fseek(fp, 0, SEEK_SET) < 0)
        PERROR("fseek");
      size_t r = fwrite(&start_time, sizeof(start_time), 1, fp);
      CHECK(r == 1);
    }
    return needs_regen_;
  }

 private:
  bool IsMissingOutputs() {
    if (!Exists(GetNinjaFilename())) {
      fprintf(stderr, "%s is missing, regenerating...\n",
              GetNinjaFilename().c_str());
      return true;
    }
    if (!Exists(GetNinjaShellScriptFilename())) {
      fprintf(stderr, "%s is missing, regenerating...\n",
              GetNinjaShellScriptFilename().c_str());
      return true;
    }
    return false;
  }

  bool CheckStep1(const std::string& orig_args) {
#define LOAD_INT(fp)                                               \
  ({                                                               \
    int v = LoadInt(fp);                                           \
    if (v < 0) {                                                   \
      fprintf(stderr, "incomplete kati_stamp, regenerating...\n"); \
      RETURN_TRUE;                                                 \
    }                                                              \
    v;                                                             \
  })

#define LOAD_STRING(fp, s)                                         \
  ({                                                               \
    if (!LoadString(fp, s)) {                                      \
      fprintf(stderr, "incomplete kati_stamp, regenerating...\n"); \
      RETURN_TRUE;                                                 \
    }                                                              \
  })

    const std::string& stamp_filename = GetNinjaStampFilename();
    FILE* fp = fopen(stamp_filename.c_str(), "rb");
    if (!fp) {
      if (g_flags.regen_debug)
        printf("%s: %s\n", stamp_filename.c_str(), strerror(errno));
      return true;
    }
    ScopedFile sfp(fp);

    double gen_time;
    size_t r = fread(&gen_time, sizeof(gen_time), 1, fp);
    gen_time_ = gen_time;
    if (r != 1) {
      fprintf(stderr, "incomplete kati_stamp, regenerating...\n");
      RETURN_TRUE;
    }
    if (g_flags.regen_debug)
      printf("Generated time: %f\n", gen_time);

    std::string s, s2;
    int num_files = LOAD_INT(fp);
    for (int i = 0; i < num_files; i++) {
      LOAD_STRING(fp, &s);
      double ts = GetTimestamp(s);
      if (gen_time < ts) {
        if (g_flags.regen_ignoring_kati_binary) {
          if (s == GetExecutablePath()) {
            fprintf(stderr, "%s was modified, ignored.\n", s.c_str());
            continue;
          }
        }
        if (ShouldIgnoreDirty(s)) {
          if (g_flags.regen_debug)
            printf("file %s: ignored (%f)\n", s.c_str(), ts);
          continue;
        }
        if (g_flags.dump_kati_stamp)
          printf("file %s: dirty (%f)\n", s.c_str(), ts);
        else
          fprintf(stderr, "%s was modified, regenerating...\n", s.c_str());
        RETURN_TRUE;
      } else if (g_flags.dump_kati_stamp) {
        printf("file %s: clean (%f)\n", s.c_str(), ts);
      }
    }

    int num_undefineds = LOAD_INT(fp);
    for (int i = 0; i < num_undefineds; i++) {
      LOAD_STRING(fp, &s);
      if (getenv(s.c_str())) {
        if (g_flags.dump_kati_stamp) {
          printf("env %s: dirty (unset => %s)\n", s.c_str(), getenv(s.c_str()));
        } else {
          fprintf(stderr, "Environment variable %s was set, regenerating...\n",
                  s.c_str());
        }
        RETURN_TRUE;
      } else if (g_flags.dump_kati_stamp) {
        printf("env %s: clean (unset)\n", s.c_str());
      }
    }

    int num_envs = LOAD_INT(fp);
    for (int i = 0; i < num_envs; i++) {
      LOAD_STRING(fp, &s);
      std::string_view val(getenv(s.c_str()));
      LOAD_STRING(fp, &s2);
      if (val != s2) {
        if (g_flags.dump_kati_stamp) {
          printf("env %s: dirty (%s => %.*s)\n", s.c_str(), s2.c_str(),
                 SPF(val));
        } else {
          fprintf(stderr,
                  "Environment variable %s was modified (%s => %.*s), "
                  "regenerating...\n",
                  s.c_str(), s2.c_str(), SPF(val));
        }
        RETURN_TRUE;
      } else if (g_flags.dump_kati_stamp) {
        printf("env %s: clean (%.*s)\n", s.c_str(), SPF(val));
      }
    }

    int num_globs = LOAD_INT(fp);
    std::string pat;
    for (int i = 0; i < num_globs; i++) {
      GlobResult* gr = new GlobResult;
      globs_.push_back(gr);

      LOAD_STRING(fp, &gr->pat);
      int num_files = LOAD_INT(fp);
      gr->result.resize(num_files);
      for (int j = 0; j < num_files; j++) {
        LOAD_STRING(fp, &gr->result[j]);
      }
    }

    int num_crs = LOAD_INT(fp);
    for (int i = 0; i < num_crs; i++) {
      ShellResult* sr = new ShellResult;
      commands_.push_back(sr);
      sr->op = static_cast<CommandOp>(LOAD_INT(fp));
      LOAD_STRING(fp, &sr->shell);
      LOAD_STRING(fp, &sr->shellflag);
      LOAD_STRING(fp, &sr->cmd);
      LOAD_STRING(fp, &sr->result);

      std::string file;
      // Ignore debug info
      LOAD_STRING(fp, &file);
      LOAD_INT(fp);

      if (sr->op == CommandOp::FIND) {
        int num_missing_dirs = LOAD_INT(fp);
        for (int j = 0; j < num_missing_dirs; j++) {
          LOAD_STRING(fp, &s);
          sr->missing_dirs.push_back(s);
        }
        int num_files = LOAD_INT(fp);
        for (int j = 0; j < num_files; j++) {
          LOAD_STRING(fp, &s);
          sr->files.push_back(s);
        }
        int num_read_dirs = LOAD_INT(fp);
        for (int j = 0; j < num_read_dirs; j++) {
          LOAD_STRING(fp, &s);
          sr->read_dirs.push_back(s);
        }
      }
    }

    LoadString(fp, &s);
    if (orig_args != s) {
      fprintf(stderr, "arguments changed, regenerating...\n");
      RETURN_TRUE;
    }

    return needs_regen_;
  }

  bool CheckGlobResult(const GlobResult* gr, std::string* err) {
    COLLECT_STATS("glob time (regen)");
    const auto& files = Glob(gr->pat.c_str());
    bool needs_regen = files.size() != gr->result.size();
    for (size_t i = 0; i < gr->result.size(); i++) {
      if (!needs_regen) {
        if (files[i] != gr->result[i]) {
          needs_regen = true;
          break;
        }
      }
    }
    if (needs_regen) {
      if (ShouldIgnoreDirty(gr->pat)) {
        if (g_flags.dump_kati_stamp) {
          printf("wildcard %s: ignored\n", gr->pat.c_str());
        }
        return false;
      }
      if (g_flags.dump_kati_stamp) {
        printf("wildcard %s: dirty\n", gr->pat.c_str());
      } else {
        *err = StringPrintf("wildcard(%s) was changed, regenerating...\n",
                            gr->pat.c_str());
      }
    } else if (g_flags.dump_kati_stamp) {
      printf("wildcard %s: clean\n", gr->pat.c_str());
    }
    return needs_regen;
  }

  bool ShouldRunCommand(const ShellResult* sr) {
    if (sr->op != CommandOp::FIND)
      return true;

    COLLECT_STATS("stat time (regen)");
    for (const std::string& dir : sr->missing_dirs) {
      if (Exists(dir))
        return true;
    }
    for (const std::string& file : sr->files) {
      if (!Exists(file))
        return true;
    }
    for (const std::string& dir : sr->read_dirs) {
      // We assume we rarely do a significant change for the top
      // directory which affects the results of find command.
      if (dir == "" || dir == "." || ShouldIgnoreDirty(dir))
        continue;

      struct stat st;
      if (lstat(dir.c_str(), &st) != 0) {
        return true;
      }
      double ts = GetTimestampFromStat(st);
      if (gen_time_ < ts) {
        return true;
      }
      if (S_ISLNK(st.st_mode)) {
        ts = GetTimestamp(dir);
        if (ts < 0 || gen_time_ < ts)
          return true;
      }
    }
    return false;
  }

  bool CheckShellResult(const ShellResult* sr, std::string* err) {
    if (sr->op == CommandOp::READ_MISSING) {
      if (Exists(sr->cmd)) {
        if (g_flags.dump_kati_stamp)
          printf("file %s: dirty\n", sr->cmd.c_str());
        else
          *err = StringPrintf("$(file <%s) was changed, regenerating...\n",
                              sr->cmd.c_str());
        return true;
      }
      if (g_flags.dump_kati_stamp)
        printf("file %s: clean\n", sr->cmd.c_str());
      return false;
    }

    if (sr->op == CommandOp::READ) {
      double ts = GetTimestamp(sr->cmd);
      if (gen_time_ < ts) {
        if (g_flags.dump_kati_stamp)
          printf("file %s: dirty\n", sr->cmd.c_str());
        else
          *err = StringPrintf("$(file <%s) was changed, regenerating...\n",
                              sr->cmd.c_str());
        return true;
      }
      if (g_flags.dump_kati_stamp)
        printf("file %s: clean\n", sr->cmd.c_str());
      return false;
    }

    if (sr->op == CommandOp::WRITE || sr->op == CommandOp::APPEND) {
      FILE* f =
          fopen(sr->cmd.c_str(), (sr->op == CommandOp::WRITE) ? "wb" : "ab");
      if (f == NULL) {
        PERROR("fopen");
      }

      if (fwrite(&sr->result[0], sr->result.size(), 1, f) != 1) {
        PERROR("fwrite");
      }

      if (fclose(f) != 0) {
        PERROR("fclose");
      }

      if (g_flags.dump_kati_stamp)
        printf("file %s: clean (write)\n", sr->cmd.c_str());
      return false;
    }

    if (!ShouldRunCommand(sr)) {
      if (g_flags.regen_debug)
        printf("shell %s: clean (no rerun)\n", sr->cmd.c_str());
      return false;
    }

    FindCommand fc;
    if (fc.Parse(sr->cmd) && !fc.chdir.empty() && ShouldIgnoreDirty(fc.chdir)) {
      if (g_flags.dump_kati_stamp)
        printf("shell %s: ignored\n", sr->cmd.c_str());
      return false;
    }

    COLLECT_STATS_WITH_SLOW_REPORT("shell time (regen)", sr->cmd.c_str());
    std::string result;
    RunCommand(sr->shell, sr->shellflag, sr->cmd, RedirectStderr::DEV_NULL,
               &result);
    FormatForCommandSubstitution(&result);
    if (sr->result != result) {
      if (g_flags.dump_kati_stamp) {
        printf("shell %s: dirty\n", sr->cmd.c_str());
      } else {
        *err = StringPrintf("$(shell %s) was changed, regenerating...\n",
                            sr->cmd.c_str());
        //*err += StringPrintf("%s => %s\n", expected.c_str(), result.c_str());
      }
      return true;
    } else if (g_flags.regen_debug) {
      printf("shell %s: clean (rerun)\n", sr->cmd.c_str());
    }
    return false;
  }

  bool CheckStep2() {
    auto glob_future = std::async([this]() {
      std::string err;
      // TODO: Make glob cache thread safe and create a task for each glob.
      SetAffinityForSingleThread();
      for (GlobResult* gr : globs_) {
        if (CheckGlobResult(gr, &err)) {
          std::unique_lock<std::mutex> lock(mu_);
          if (!needs_regen_) {
            needs_regen_ = true;
            msg_ = err;
          }
          break;
        }
      }
    });

    auto shell_future = std::async([this]() {
      SetAffinityForSingleThread();
      for (ShellResult* sr : commands_) {
        std::string err;
        if (CheckShellResult(sr, &err)) {
          std::unique_lock<std::mutex> lock(mu_);
          if (!needs_regen_) {
            needs_regen_ = true;
            msg_ = err;
          }
        }
      }
    });

    glob_future.wait();
    shell_future.wait();
    if (needs_regen_) {
      fprintf(stderr, "%s", msg_.c_str());
    }
    return needs_regen_;
  }

 private:
  double gen_time_;
  std::vector<GlobResult*> globs_;
  std::vector<ShellResult*> commands_;
  std::mutex mu_;
  bool needs_regen_;
  std::string msg_;
};

}  // namespace

bool NeedsRegen(double start_time, const std::string& orig_args) {
  return StampChecker().NeedsRegen(start_time, orig_args);
}
