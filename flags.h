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

#ifndef FLAGS_H_
#define FLAGS_H_

#include <string>
#include <vector>

#include "string_piece.h"
#include "symtab.h"

using namespace std;

struct Flags {
  bool detect_android_echo;
  bool detect_depfiles;
  bool dump_kati_stamp;
  bool enable_debug;
  bool enable_kati_warnings;
  bool enable_stat_logs;
  bool gen_all_targets;
  bool generate_ninja;
  bool is_dry_run;
  bool is_silent_mode;
  bool is_syntax_check_only;
  bool regen;
  bool regen_ignoring_kati_binary;
  bool use_find_emulator;
  const char* goma_dir;
  const char* ignore_dirty_pattern;
  const char* no_ignore_dirty_pattern;
  const char* ignore_optional_include_pattern;
  const char* makefile;
  const char* ninja_dir;
  const char* ninja_suffix;
  int num_cpus;
  int num_jobs;
  int remote_num_jobs;
  vector<const char*> subkati_args;
  vector<Symbol> targets;
  vector<StringPiece> cl_vars;

  void Parse(int argc, char** argv);
};

extern Flags g_flags;

#endif  // FLAGS_H_
