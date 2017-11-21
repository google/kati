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

#include "io.h"
#include "log.h"
#include "strutil.h"

int main(int argc, char* argv[]) {
  bool dump_files = false;
  bool dump_env = false;

  if (argc == 1) {
    fprintf(stderr, "Usage: ckati_stamp_dump [--env] [--files] <stamp>\n");
    return 1;
  }

  for (int i = 1; i < argc - 1; i++) {
    const char* arg = argv[i];
    if (!strcmp(arg, "--env")) {
      dump_env = true;
    } else if (!strcmp(arg, "--files")) {
      dump_files = true;
    } else {
      fprintf(stderr, "Unknown option: %s", arg);
      return 1;
    }
  }

  if (!dump_files && !dump_env) {
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

  int num_files = LoadInt(fp);
  if (num_files < 0)
    ERROR("Incomplete stamp file");
  for (int i = 0; i < num_files; i++) {
    string s;
    if (!LoadString(fp, &s))
      ERROR("Incomplete stamp file");
    if (dump_files)
      printf("%s\n", s.c_str());
  }

  int num_undefineds = LoadInt(fp);
  if (num_undefineds < 0)
    ERROR("Incomplete stamp file");
  for (int i = 0; i < num_undefineds; i++) {
    string s;
    if (!LoadString(fp, &s))
      ERROR("Incomplete stamp file");
    if (dump_env)
      printf("undefined: %s\n", s.c_str());
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

  return 0;
}
