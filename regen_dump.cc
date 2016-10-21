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
  if (argc == 1) {
    fprintf(stderr, "Usage: ckati_stamp_dump <stamp>\n");
    return 1;
  }

  FILE *fp = fopen(argv[1], "rb");
  if(!fp)
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
    printf("%s\n", s.c_str());
  }

  return 0;
}
