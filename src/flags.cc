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

#include "flags.h"

#include <stdlib.h>
#include <unistd.h>

#include "log.h"
#include "strutil.h"

Flags g_flags;

static bool ParseCommandLineOptionWithArg(std::string_view option,
                                          char* argv[],
                                          int* index,
                                          const char** out_arg) {
  const char* arg = argv[*index];
  if (!HasPrefix(arg, option))
    return false;
  if (arg[option.size()] == '\0') {
    ++*index;
    *out_arg = argv[*index];
    return true;
  }
  if (arg[option.size()] == '=') {
    *out_arg = arg + option.size() + 1;
    return true;
  }
  // E.g, -j999
  if (option.size() == 2) {
    *out_arg = arg + option.size();
    return true;
  }
  return false;
}

void Flags::Parse(int argc, char** argv) {
  subkati_args.push_back(argv[0]);
  num_jobs = num_cpus = sysconf(_SC_NPROCESSORS_ONLN);
  const char* num_jobs_str;
  const char* writable_str;
  const char* variable_assignment_trace_filter;

  if (const char* makeflags = getenv("MAKEFLAGS")) {
    for (std::string_view tok : WordScanner(makeflags)) {
      if (!HasPrefix(tok, "-") && tok.find('=') != std::string::npos)
        cl_vars.push_back(tok);
    }
  }

  for (int i = 1; i < argc; i++) {
    const char* arg = argv[i];
    bool should_propagate = true;
    int pi = i;
    if (!strcmp(arg, "-f")) {
      makefile = argv[++i];
      should_propagate = false;
    } else if (!strcmp(arg, "-c")) {
      is_syntax_check_only = true;
    } else if (!strcmp(arg, "-i")) {
      is_dry_run = true;
    } else if (!strcmp(arg, "-s")) {
      is_silent_mode = true;
    } else if (!strcmp(arg, "-d")) {
      enable_debug = true;
    } else if (!strcmp(arg, "--kati_stats")) {
      enable_stat_logs = true;
    } else if (!strcmp(arg, "--warn")) {
      enable_kati_warnings = true;
    } else if (!strcmp(arg, "--ninja")) {
      generate_ninja = true;
    } else if (!strcmp(arg, "--empty_ninja_file")) {
      generate_empty_ninja = true;
    } else if (!strcmp(arg, "--gen_all_targets")) {
      gen_all_targets = true;
    } else if (!strcmp(arg, "--regen")) {
      // TODO: Make this default.
      regen = true;
    } else if (!strcmp(arg, "--regen_debug")) {
      regen_debug = true;
    } else if (!strcmp(arg, "--regen_ignoring_kati_binary")) {
      regen_ignoring_kati_binary = true;
    } else if (!strcmp(arg, "--dump_kati_stamp")) {
      dump_kati_stamp = true;
      regen_debug = true;
    } else if (!strcmp(arg, "--detect_android_echo")) {
      detect_android_echo = true;
    } else if (!strcmp(arg, "--detect_depfiles")) {
      detect_depfiles = true;
    } else if (!strcmp(arg, "--color_warnings")) {
      color_warnings = true;
    } else if (!strcmp(arg, "--no_builtin_rules")) {
      no_builtin_rules = true;
    } else if (!strcmp(arg, "--no_ninja_prelude")) {
      no_ninja_prelude = true;
    } else if (!strcmp(arg, "--use_ninja_phony_output")) {
      use_ninja_phony_output = true;
    } else if (!strcmp(arg, "--use_ninja_symlink_outputs")) {
      use_ninja_symlink_outputs = true;
    } else if (!strcmp(arg, "--use_ninja_validations")) {
      use_ninja_validations = true;
    } else if (!strcmp(arg, "--werror_find_emulator")) {
      werror_find_emulator = true;
    } else if (!strcmp(arg, "--werror_overriding_commands")) {
      werror_overriding_commands = true;
    } else if (!strcmp(arg, "--warn_implicit_rules")) {
      warn_implicit_rules = true;
    } else if (!strcmp(arg, "--werror_implicit_rules")) {
      werror_implicit_rules = true;
    } else if (!strcmp(arg, "--warn_suffix_rules")) {
      warn_suffix_rules = true;
    } else if (!strcmp(arg, "--werror_suffix_rules")) {
      werror_suffix_rules = true;
    } else if (!strcmp(arg, "--top_level_phony")) {
      top_level_phony = true;
    } else if (!strcmp(arg, "--warn_real_to_phony")) {
      warn_real_to_phony = true;
    } else if (!strcmp(arg, "--werror_real_to_phony")) {
      warn_real_to_phony = true;
      werror_real_to_phony = true;
    } else if (!strcmp(arg, "--warn_phony_looks_real")) {
      warn_phony_looks_real = true;
    } else if (!strcmp(arg, "--werror_phony_looks_real")) {
      warn_phony_looks_real = true;
      werror_phony_looks_real = true;
    } else if (!strcmp(arg, "--werror_writable")) {
      werror_writable = true;
    } else if (!strcmp(arg, "--warn_real_no_cmds_or_deps")) {
      warn_real_no_cmds_or_deps = true;
    } else if (!strcmp(arg, "--werror_real_no_cmds_or_deps")) {
      warn_real_no_cmds_or_deps = true;
      werror_real_no_cmds_or_deps = true;
    } else if (!strcmp(arg, "--warn_real_no_cmds")) {
      warn_real_no_cmds = true;
    } else if (!strcmp(arg, "--werror_real_no_cmds")) {
      warn_real_no_cmds = true;
      werror_real_no_cmds = true;
    } else if (ParseCommandLineOptionWithArg("-C", argv, &i, &working_dir)) {
    } else if (ParseCommandLineOptionWithArg("--dump_include_graph", argv, &i,
                                             &dump_include_graph)) {
    } else if (ParseCommandLineOptionWithArg("--dump_variable_assignment_trace",
                                             argv, &i,
                                             &dump_variable_assignment_trace)) {
    } else if (ParseCommandLineOptionWithArg(
                   "--variable_assignment_trace_filter", argv, &i,
                   &variable_assignment_trace_filter)) {
      for (std::string_view pat :
           WordScanner(variable_assignment_trace_filter)) {
        traced_variables_pattern.push_back(Pattern(pat));
      }
    } else if (ParseCommandLineOptionWithArg("-j", argv, &i, &num_jobs_str)) {
      num_jobs = strtol(num_jobs_str, NULL, 10);
      if (num_jobs <= 0) {
        ERROR("Invalid -j flag: %s", num_jobs_str);
      }
    } else if (ParseCommandLineOptionWithArg("--remote_num_jobs", argv, &i,
                                             &num_jobs_str)) {
      remote_num_jobs = strtol(num_jobs_str, NULL, 10);
      if (remote_num_jobs <= 0) {
        ERROR("Invalid -j flag: %s", num_jobs_str);
      }
    } else if (ParseCommandLineOptionWithArg("--ninja_suffix", argv, &i,
                                             &ninja_suffix)) {
    } else if (ParseCommandLineOptionWithArg("--ninja_dir", argv, &i,
                                             &ninja_dir)) {
    } else if (!strcmp(arg, "--use_find_emulator")) {
      use_find_emulator = true;
    } else if (ParseCommandLineOptionWithArg("--goma_dir", argv, &i,
                                             &goma_dir)) {
    } else if (ParseCommandLineOptionWithArg(
                   "--ignore_optional_include", argv, &i,
                   &ignore_optional_include_pattern)) {
    } else if (ParseCommandLineOptionWithArg("--ignore_dirty", argv, &i,
                                             &ignore_dirty_pattern)) {
    } else if (ParseCommandLineOptionWithArg("--no_ignore_dirty", argv, &i,
                                             &no_ignore_dirty_pattern)) {
    } else if (ParseCommandLineOptionWithArg("--writable", argv, &i,
                                             &writable_str)) {
      writable.push_back(writable_str);
    } else if (ParseCommandLineOptionWithArg("--default_pool", argv, &i,
                                             &default_pool)) {
    } else if (arg[0] == '-') {
      ERROR("Unknown flag: %s", arg);
    } else {
      if (strchr(arg, '=')) {
        cl_vars.push_back(arg);
      } else {
        should_propagate = false;
        targets.push_back(Intern(arg));
      }
    }

    if (should_propagate) {
      for (; pi <= i; pi++) {
        subkati_args.push_back(argv[pi]);
      }
    }
  }

  if (traced_variables_pattern.size() &&
      dump_variable_assignment_trace == nullptr) {
    ERROR(
        "--variable_assignment_trace_filter is valid only together with "
        "--dump_variable_assignment_trace");
  }
}
