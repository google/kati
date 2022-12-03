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
#include <string_view>
#include <vector>

#include "dep.h"

class Evaluator;

void GenerateNinja(const std::vector<NamedDepNode>& nodes,
                   Evaluator* ev,
                   const std::string& orig_args,
                   double start_time);

std::string GetNinjaFilename();
std::string GetNinjaShellScriptFilename();
std::string GetNinjaStampFilename();

// Exposed only for test.
bool GetDepfileFromCommand(std::string* cmd, std::string* out);
size_t GetGomaccPosForAndroidCompileCommand(std::string_view cmdline);

#endif  // NINJA_H_
