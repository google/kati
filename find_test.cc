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

#include "find.h"

#include <string>

#include "strutil.h"

int main(int argc, char* argv[]) {
  if (argc == 1) {
    fprintf(stderr, "TODO: Write unit tests\n");
    return 1;
  }

  InitFindEmulator();
  string cmd;
  for (int i = 1; i < argc; i++) {
    if (i > 1)
      cmd += ' ';
    cmd += argv[i];
  }
  FindCommand fc;
  if (!fc.Parse(cmd)) {
    fprintf(stderr, "Find emulator does not support this command\n");
    return 1;
  }
  string out;
  if (!FindEmulator::Get()->HandleFind(cmd, fc, &out)) {
    fprintf(stderr, "Find emulator does not support this command\n");
    return 1;
  }

  for (StringPiece tok : WordScanner(out)) {
    printf("%.*s\n", SPF(tok));
  }
}
