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

static bool ParseCommandLineOptionWithArg(StringPiece option,
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

  if (const char* makeflags = getenv("MAKEFLAGS")) {
    for (StringPiece tok : WordScanner(makeflags)) {
      if (!HasPrefix(tok, "-") && tok.find('=') != string::npos)
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
    } else if (!strcmp(arg, "--gen_all_targets")) {
      gen_all_targets = true;
    } else if (!strcmp(arg, "--regen")) {
      // TODO: Make this default.
      regen = true;
    } else if (!strcmp(arg, "--regen_ignoring_kati_binary")) {
      regen_ignoring_kati_binary = true;
    } else if (!strcmp(arg, "--dump_kati_stamp")) {
      dump_kati_stamp = true;
    } else if (!strcmp(arg, "--detect_android_echo")) {
      detect_android_echo = true;
    } else if (!strcmp(arg, "--detect_depfiles")) {
      detect_depfiles = true;
    } else if (ParseCommandLineOptionWithArg(
        "-j", argv, &i, &num_jobs_str)) {
      num_jobs = strtol(num_jobs_str, NULL, 10);
      if (num_jobs <= 0) {
        ERROR("Invalid -j flag: %s", num_jobs_str);
      }
    } else if (ParseCommandLineOptionWithArg(
        "--remote_num_jobs", argv, &i, &num_jobs_str)) {
      remote_num_jobs = strtol(num_jobs_str, NULL, 10);
      if (remote_num_jobs <= 0) {
        ERROR("Invalid -j flag: %s", num_jobs_str);
      }
    } else if (ParseCommandLineOptionWithArg(
        "--ninja_suffix", argv, &i, &ninja_suffix)) {
    } else if (ParseCommandLineOptionWithArg(
        "--ninja_dir", argv, &i, &ninja_dir)) {
    } else if (!strcmp(arg, "--use_find_emulator")) {
      use_find_emulator = true;
    } else if (ParseCommandLineOptionWithArg(
        "--goma_dir", argv, &i, &goma_dir)) {
    } else if (ParseCommandLineOptionWithArg(
        "--ignore_optional_include",
        argv, &i, &ignore_optional_include_pattern)) {
    } else if (ParseCommandLineOptionWithArg(
        "--ignore_dirty",
        argv, &i, &ignore_dirty_pattern)) {
    } else if (ParseCommandLineOptionWithArg(
        "--no_ignore_dirty",
        argv, &i, &no_ignore_dirty_pattern)) {
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
}
