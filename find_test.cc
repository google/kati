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
#include <unistd.h>

#include "fileutil.h"
#include "strutil.h"

int FindUnitTests();

int main(int argc, char* argv[]) {
  if (argc == 1) {
    return FindUnitTests();
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
  if (!FindEmulator::Get()->HandleFind(cmd, fc, Loc(), &out)) {
    fprintf(stderr, "Find emulator does not support this command\n");
    return 1;
  }

  for (StringPiece tok : WordScanner(out)) {
    printf("%.*s\n", SPF(tok));
  }
}

string Run(const string& cmd) {
  string s;
  int ret = RunCommand("/bin/sh", "-c", cmd, RedirectStderr::NONE, &s);

  if (ret != 0) {
    fprintf(stderr, "Failed to run `%s`\n", cmd.c_str());
    exit(ret);
  }

  return s;
}

static bool unit_test_failed = false;

void CompareFind(const string& cmd) {
  string native = Run(cmd);

  FindCommand fc;
  if (!fc.Parse(cmd)) {
    fprintf(stderr, "Find emulator cannot parse `%s`\n", cmd.c_str());
    exit(1);
  }
  string emulated;
  if (!FindEmulator::Get()->HandleFind(cmd, fc, Loc(), &emulated)) {
    fprintf(stderr, "Find emulator cannot parse `%s`\n", cmd.c_str());
    exit(1);
  }

  vector<StringPiece> nativeWords;
  vector<StringPiece> emulatedWords;

  WordScanner(native).Split(&nativeWords);
  WordScanner(emulated).Split(&emulatedWords);

  if (nativeWords != emulatedWords) {
    fprintf(stderr, "Failed to match `%s`:\n", cmd.c_str());

    auto nativeIter = nativeWords.begin();
    auto emulatedIter = emulatedWords.begin();
    fprintf(stderr, "%-20s %-20s\n", "Native:", "Emulated:");
    while (nativeIter != nativeWords.end() || emulatedIter != emulatedWords.end()) {
      fprintf(stderr, " %-20s %-20s\n",
              (nativeIter == nativeWords.end()) ? "" : (*nativeIter++).as_string().c_str(),
              (emulatedIter == emulatedWords.end()) ? "" : (*emulatedIter++).as_string().c_str());
    }
    fprintf(stderr, "------------------------------------------\n");
    unit_test_failed = true;
  }
}

int FindUnitTests() {
  Run("rm -rf out/find");
  Run("mkdir -p out/find");
  if (chdir("out/find")) {
    perror("Failed to chdir(out/find)");
    return 1;
  }

  // Set up files under out/find:
  //  drwxr-x--- top
  //  lrwxrwxrwx top/E -> missing
  //  lrwxrwxrwx top/C -> A
  //  -rw-r----- top/a
  //  drwxr-x--- top/A
  //  lrwxrwxrwx top/A/D -> B
  //  -rw-r----- top/A/b
  //  drwxr-x--- top/A/B
  //  -rw-r----- top/A/B/z
  Run("mkdir -p top/A/B");
  Run("cd top && ln -s A C");
  Run("cd top/A && ln -s B D");
  Run("cd top && ln -s missing E");
  Run("touch top/a top/A/b top/A/B/z");

  InitFindEmulator();

  CompareFind("find .");
  CompareFind("find -L .");

  CompareFind("cd top && find C");
  CompareFind("cd top && find -L C");

  CompareFind("cd top/C && find D/./z");

  return unit_test_failed ? 1 : 0;
}
