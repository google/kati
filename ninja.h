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

#ifndef NINJA_H_
#define NINJA_H_

#include <time.h>

#include <string>
#include <vector>

#include "string_piece.h"

using namespace std;

struct DepNode;
class Evaluator;

void GenerateNinja(const vector<DepNode*>& nodes,
                   Evaluator* ev,
                   const string& orig_args,
                   double start_time);

string GetNinjaFilename();
string GetNinjaShellScriptFilename();
string GetNinjaStampFilename();

// Exposed only for test.
bool GetDepfileFromCommand(string* cmd, string* out);
size_t GetGomaccPosForAndroidCompileCommand(StringPiece cmdline);

#endif  // NINJA_H_
