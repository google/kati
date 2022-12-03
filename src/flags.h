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
#include <string_view>
#include <vector>

#include "strutil.h"
#include "symtab.h"

struct Flags {
  bool detect_android_echo;
  bool detect_depfiles;
  bool dump_kati_stamp;
  const char* dump_include_graph;
  const char* dump_variable_assignment_trace;
  bool enable_debug;
  bool enable_kati_warnings;
  bool enable_stat_logs;
  bool gen_all_targets;
  bool generate_ninja;
  bool generate_empty_ninja;
  bool is_dry_run;
  bool is_silent_mode;
  bool is_syntax_check_only;
  bool regen;
  bool regen_debug;
  bool regen_ignoring_kati_binary;
  bool use_find_emulator;
  bool color_warnings;
  bool no_builtin_rules;
  bool no_ninja_prelude;
  bool use_ninja_phony_output;
  bool use_ninja_symlink_outputs;
  bool use_ninja_validations;
  bool werror_find_emulator;
  bool werror_overriding_commands;
  bool warn_implicit_rules;
  bool werror_implicit_rules;
  bool warn_suffix_rules;
  bool werror_suffix_rules;
  bool top_level_phony;
  bool warn_real_to_phony;
  bool werror_real_to_phony;
  bool warn_phony_looks_real;
  bool werror_phony_looks_real;
  bool werror_writable;
  bool warn_real_no_cmds_or_deps;
  bool werror_real_no_cmds_or_deps;
  bool warn_real_no_cmds;
  bool werror_real_no_cmds;
  const char* default_pool;
  const char* goma_dir;
  const char* ignore_dirty_pattern;
  const char* no_ignore_dirty_pattern;
  const char* ignore_optional_include_pattern;
  const char* makefile;
  const char* ninja_dir;
  const char* ninja_suffix;
  const char* working_dir;  // -C <dir>
  int num_cpus;
  int num_jobs;
  int remote_num_jobs;
  std::vector<const char*> subkati_args;
  std::vector<Symbol> targets;
  std::vector<std::string_view> cl_vars;
  std::vector<std::string> writable;
  std::vector<Pattern> traced_variables_pattern;

  void Parse(int argc, char** argv);
};

extern Flags g_flags;

#endif  // FLAGS_H_
