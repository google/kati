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

// This command will dump the contents of a kati stamp file into a more portable
// format for use by other tools. For now, it just exports the files read.
// Later, this will be expanded to include the Glob and Shell commands, but
// those require a more complicated output format.

#include <stdio.h>

#include <string>
#include <vector>

#include "func.h"
#include "io.h"
#include "log.h"
#include "strutil.h"

using namespace std;

vector<std::string> LoadVecString(FILE* fp) {
  int count = LoadInt(fp);
  if (count < 0) {
    ERROR("Incomplete stamp file");
  }
  std::vector<std::string> ret(count);
  for (int i = 0; i < count; i++) {
    if (!LoadString(fp, &ret[i])) {
      ERROR("Incomplete stamp file");
    }
  }
  return ret;
}

int main(int argc, char* argv[]) {
  bool dump_files = false;
  bool dump_env = false;
  bool dump_globs = false;
  bool dump_cmds = false;
  bool dump_finds = false;

  if (argc == 1) {
    fprintf(stderr,
            "Usage: ckati_stamp_dump [--env] [--files] [--globs] [--cmds] "
            "[--finds] <stamp>\n");
    return 1;
  }

  for (int i = 1; i < argc - 1; i++) {
    const char* arg = argv[i];
    if (!strcmp(arg, "--env")) {
      dump_env = true;
    } else if (!strcmp(arg, "--files")) {
      dump_files = true;
    } else if (!strcmp(arg, "--globs")) {
      dump_globs = true;
    } else if (!strcmp(arg, "--cmds")) {
      dump_cmds = true;
    } else if (!strcmp(arg, "--finds")) {
      dump_finds = true;
    } else {
      fprintf(stderr, "Unknown option: %s", arg);
      return 1;
    }
  }

  if (!dump_files && !dump_env && !dump_globs && !dump_cmds && !dump_finds) {
    dump_files = true;
  }

  FILE* fp = fopen(argv[argc - 1], "rb");
  if (!fp)
    PERROR("fopen");

  ScopedFile sfp(fp);
  double gen_time;
  size_t r = fread(&gen_time, sizeof(gen_time), 1, fp);
  if (r != 1)
    ERROR("Incomplete stamp file");

  //
  // See regen.cc CheckStep1 for how this is read normally
  //

  {
    auto files = LoadVecString(fp);
    if (dump_files) {
      for (const auto& f : files) {
        printf("%s\n", f.c_str());
      }
    }
  }

  {
    auto undefined = LoadVecString(fp);
    if (dump_env) {
      for (const auto& s : undefined) {
        printf("undefined: %s\n", s.c_str());
      }
    }
  }

  int num_envs = LoadInt(fp);
  if (num_envs < 0)
    ERROR("Incomplete stamp file");
  for (int i = 0; i < num_envs; i++) {
    string name;
    string val;
    if (!LoadString(fp, &name))
      ERROR("Incomplete stamp file");
    if (!LoadString(fp, &val))
      ERROR("Incomplete stamp file");
    if (dump_env)
      printf("%s: %s\n", name.c_str(), val.c_str());
  }

  int num_globs = LoadInt(fp);
  if (num_globs < 0)
    ERROR("Incomplete stamp file");
  for (int i = 0; i < num_globs; i++) {
    string pat;
    if (!LoadString(fp, &pat))
      ERROR("Incomplete stamp file");

    auto files = LoadVecString(fp);
    if (dump_globs) {
      printf("%s\n", pat.c_str());

      for (const auto& s : files) {
        printf("  %s\n", s.c_str());
      }
    }
  }

  int num_cmds = LoadInt(fp);
  if (num_cmds < 0)
    ERROR("Incomplete stamp file");
  for (int i = 0; i < num_cmds; i++) {
    CommandOp op = static_cast<CommandOp>(LoadInt(fp));
    string shell, shellflag, cmd, result, file;
    if (!LoadString(fp, &shell))
      ERROR("Incomplete stamp file");
    if (!LoadString(fp, &shellflag))
      ERROR("Incomplete stamp file");
    if (!LoadString(fp, &cmd))
      ERROR("Incomplete stamp file");
    if (!LoadString(fp, &result))
      ERROR("Incomplete stamp file");
    if (!LoadString(fp, &file))
      ERROR("Incomplete stamp file");
    int line = LoadInt(fp);
    if (line < 0)
      ERROR("Incomplete stamp file");

    if (op == CommandOp::FIND) {
      auto missing_dirs = LoadVecString(fp);
      auto files = LoadVecString(fp);
      auto read_dirs = LoadVecString(fp);

      if (dump_finds) {
        printf("cmd type: FIND\n");
        printf("  shell: %s\n", shell.c_str());
        printf("  shell flags: %s\n", shellflag.c_str());
        printf("  loc: %s:%d\n", file.c_str(), line);
        printf("  cmd: %s\n", cmd.c_str());
        if (result.length() > 0 && result.length() < 500 &&
            result.find('\n') == std::string::npos) {
          printf("  output: %s\n", result.c_str());
        } else {
          printf("  output: <%zu bytes>\n", result.length());
        }
        printf("  missing dirs:\n");
        for (const auto& d : missing_dirs) {
          printf("    %s\n", d.c_str());
        }
        printf("  files:\n");
        for (const auto& f : files) {
          printf("    %s\n", f.c_str());
        }
        printf("  read dirs:\n");
        for (const auto& d : read_dirs) {
          printf("    %s\n", d.c_str());
        }
        printf("\n");
      }
    } else if (dump_cmds) {
      if (op == CommandOp::SHELL) {
        printf("cmd type: SHELL\n");
        printf("  shell: %s\n", shell.c_str());
        printf("  shell flags: %s\n", shellflag.c_str());
      } else if (op == CommandOp::READ) {
        printf("cmd type: READ\n");
      } else if (op == CommandOp::READ_MISSING) {
        printf("cmd type: READ_MISSING\n");
      } else if (op == CommandOp::WRITE) {
        printf("cmd type: WRITE\n");
      } else if (op == CommandOp::APPEND) {
        printf("cmd type: APPEND\n");
      }
      printf("  loc: %s:%d\n", file.c_str(), line);
      printf("  cmd: %s\n", cmd.c_str());
      if (result.length() > 0 && result.length() < 500 &&
          result.find('\n') == std::string::npos) {
        printf("  output: %s\n", result.c_str());
      } else {
        printf("  output: <%zu bytes>\n", result.length());
      }
      printf("\n");
    }
  }

  return 0;
}
