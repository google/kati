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

#include "regen.h"

#include <sys/stat.h>

#include <algorithm>

#include "fileutil.h"
#include "find.h"
#include "io.h"
#include "log.h"
#include "ninja.h"
#include "stats.h"
#include "strutil.h"

static bool ShouldIgnoreDirty(StringPiece s) {
  Pattern pat(g_flags.ignore_dirty_pattern);
  Pattern nopat(g_flags.no_ignore_dirty_pattern);
  return pat.Match(s) && !nopat.Match(s);
}

bool NeedsRegen(double start_time, const string& orig_args) {
  bool retval = false;
#define RETURN_TRUE do {                         \
    if (g_flags.dump_kati_stamp)                 \
      retval = true;                             \
    else                                         \
      return true;                               \
  } while (0)

#define LOAD_INT(fp) ({                                                 \
      int v = LoadInt(fp);                                              \
      if (v < 0) {                                                      \
        fprintf(stderr, "incomplete kati_stamp, regenerating...\n");    \
        RETURN_TRUE;                                                    \
      }                                                                 \
      v;                                                                \
    })

#define LOAD_STRING(fp, s) ({                                           \
      if (!LoadString(fp, s)) {                                         \
        fprintf(stderr, "incomplete kati_stamp, regenerating...\n");    \
        RETURN_TRUE;                                                    \
      }                                                                 \
    })

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

  const string& stamp_filename = GetNinjaStampFilename();
  FILE* fp = fopen(stamp_filename.c_str(), "rb+");
  if (!fp) {
    if (g_flags.dump_kati_stamp)
      printf("%s: %s\n", stamp_filename.c_str(), strerror(errno));
    return true;
  }
  ScopedFile sfp(fp);

  double gen_time;
  size_t r = fread(&gen_time, sizeof(gen_time), 1, fp);
  if (r != 1) {
    fprintf(stderr, "incomplete kati_stamp, regenerating...\n");
    RETURN_TRUE;
  }
  if (g_flags.dump_kati_stamp)
    printf("Generated time: %f\n", gen_time);

  string s, s2;
  int num_files = LOAD_INT(fp);
  for (int i = 0; i < num_files; i++) {
    LOAD_STRING(fp, &s);
    double ts = GetTimestamp(s);
    if (gen_time < ts) {
      if (g_flags.regen_ignoring_kati_binary) {
        string kati_binary;
        GetExecutablePath(&kati_binary);
        if (s == kati_binary) {
          fprintf(stderr, "%s was modified, ignored.\n", s.c_str());
          continue;
        }
      }
      if (ShouldIgnoreDirty(s)) {
        if (g_flags.dump_kati_stamp)
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
    StringPiece val(getenv(s.c_str()));
    LOAD_STRING(fp, &s2);
    if (val != s2) {
      if (g_flags.dump_kati_stamp) {
        printf("env %s: dirty (%s => %.*s)\n",
               s.c_str(), s2.c_str(), SPF(val));
      } else {
        fprintf(stderr, "Environment variable %s was modified (%s => %.*s), "
                "regenerating...\n",
                s.c_str(), s2.c_str(), SPF(val));
      }
      RETURN_TRUE;
    } else if (g_flags.dump_kati_stamp) {
      printf("env %s: clean (%.*s)\n", s.c_str(), SPF(val));
    }
  }

  {
    int num_globs = LOAD_INT(fp);
    string pat;
    for (int i = 0; i < num_globs; i++) {
      COLLECT_STATS("glob time (regen)");
      LOAD_STRING(fp, &pat);
#if 0
      bool needs_reglob = false;
      int num_dirs = LOAD_INT(fp);
      for (int j = 0; j < num_dirs; j++) {
        LOAD_STRING(fp, &s);
        // TODO: Handle removed files properly.
        needs_reglob |= gen_time < GetTimestamp(s);
      }
#endif
      int num_files = LOAD_INT(fp);
      vector<string>* files;
      Glob(pat.c_str(), &files);
      sort(files->begin(), files->end());
      bool needs_regen = files->size() != static_cast<size_t>(num_files);
      for (int j = 0; j < num_files; j++) {
        LOAD_STRING(fp, &s);
        if (!needs_regen) {
          if ((*files)[j] != s) {
            needs_regen = true;
            break;
          }
        }
      }
      if (needs_regen) {
        if (ShouldIgnoreDirty(pat)) {
          if (g_flags.dump_kati_stamp) {
            printf("wildcard %s: ignored\n", pat.c_str());
          }
          continue;
        }
        if (g_flags.dump_kati_stamp) {
          printf("wildcard %s: dirty\n", pat.c_str());
        } else {
          fprintf(stderr, "wildcard(%s) was changed, regenerating...\n",
                  pat.c_str());
        }
        RETURN_TRUE;
      } else if (g_flags.dump_kati_stamp) {
        printf("wildcard %s: clean\n", pat.c_str());
      }
    }
  }

  int num_crs = LOAD_INT(fp);
  for (int i = 0; i < num_crs; i++) {
    string cmd, expected;
    LOAD_STRING(fp, &cmd);
    LOAD_STRING(fp, &expected);

    {
      COLLECT_STATS("stat time (regen)");
      bool has_condition = LOAD_INT(fp);
      if (has_condition) {
        bool should_run_command = false;

        int num_missing_dirs = LOAD_INT(fp);
        for (int j = 0; j < num_missing_dirs; j++) {
          LOAD_STRING(fp, &s);
          should_run_command |= Exists(s);
        }

        int num_read_dirs = LOAD_INT(fp);
        for (int j = 0; j < num_read_dirs; j++) {
          LOAD_STRING(fp, &s);
          // We assume we rarely do a significant change for the top
          // directory which affects the results of find command.
          if (s == "" || s == "." || ShouldIgnoreDirty(s))
            continue;

          struct stat st;
          if (lstat(s.c_str(), &st) != 0) {
            should_run_command = true;
            continue;
          }
          double ts = GetTimestampFromStat(st);
          if (gen_time < ts) {
            should_run_command = true;
            continue;
          }
          if (S_ISLNK(st.st_mode)) {
            ts = GetTimestamp(s);
            should_run_command |= (ts < 0 || gen_time < ts);
          }
        }

        if (!should_run_command) {
          if (g_flags.dump_kati_stamp)
            printf("shell %s: clean (no rerun)\n", cmd.c_str());
          continue;
        }
      }
    }

    FindCommand fc;
    if (fc.Parse(cmd) && !fc.chdir.empty() && ShouldIgnoreDirty(fc.chdir)) {
      if (g_flags.dump_kati_stamp)
        printf("shell %s: ignored\n", cmd.c_str());
      continue;
    }

    {
      COLLECT_STATS_WITH_SLOW_REPORT("shell time (regen)", cmd.c_str());
      string result;
      RunCommand("/bin/sh", cmd, RedirectStderr::DEV_NULL, &result);
      FormatForCommandSubstitution(&result);
      if (expected != result) {
        if (g_flags.dump_kati_stamp) {
          printf("shell %s: dirty\n", cmd.c_str());
        } else {
          fprintf(stderr, "$(shell %s) was changed, regenerating...\n",
                  cmd.c_str());
#if 0
          fprintf(stderr, "%s => %s\n",
                  expected.c_str(), result.c_str());
#endif
        }
        RETURN_TRUE;
      } else if (g_flags.dump_kati_stamp) {
        printf("shell %s: clean (rerun)\n", cmd.c_str());
      }
    }
  }

  LoadString(fp, &s);
  if (orig_args != s) {
    fprintf(stderr, "arguments changed, regenerating...\n");
    RETURN_TRUE;
  }

  if (!retval) {
    if (fseek(fp, 0, SEEK_SET) < 0)
      PERROR("fseek");
    size_t r = fwrite(&start_time, sizeof(start_time), 1, fp);
    CHECK(r == 1);
  }

  return retval;
}
