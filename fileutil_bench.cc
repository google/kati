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

#include "fileutil.h"
#include <benchmark/benchmark.h>
#include <cstdio>

static void BM_RunCommand(benchmark::State& state) {
  std::string shell = "/bin/bash";
  std::string shellflag = "-c";
  std::string cmd = "echo $((1+3))";
  while (state.KeepRunning()) {
    std::string result;
    RunCommand(shell, shellflag, cmd, RedirectStderr::NONE, &result);
  }
}
BENCHMARK(BM_RunCommand);

static void BM_RunCommand_ComplexShell(benchmark::State& state) {
  std::string shell = "/bin/bash ";
  std::string shellflag = "-c";
  std::string cmd = "echo $((1+3))";
  while (state.KeepRunning()) {
    std::string result;
    RunCommand(shell, shellflag, cmd, RedirectStderr::NONE, &result);
  }
}
BENCHMARK(BM_RunCommand_ComplexShell);

BENCHMARK_MAIN();
