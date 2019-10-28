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

#include "version.h"
#include "log.h"
#include "strutil.h"

#include <functional>
#include <algorithm>
#include <iostream>
#include <sstream>
#include "CLI/CLI.hpp"

Flags g_flags;

// One needs CLI11 wirh some methods made virtual. https://github.com/CLIUtils/CLI11/pull/340
class MyApp: public CLI::App{
  using CLI::App::App;

  virtual bool _parse_positional(std::vector<std::string> &args, bool haltOnSubcommand) override{
    positionalArgImmediateCallback(args.back());
    auto res = CLI::App::_parse_positional(args, haltOnSubcommand);
    return res;
  }
  virtual bool _parse_single(std::vector<std::string> &args, bool &positional_only) override {
    perArgCallbackPre();
    std::string first, second; // the array is inverted, last string is first arg (from remaining), pre-last is the second one
    auto previousSize = args.size();
    if(previousSize >= 1){
      first = args[args.size()-1];
      if(previousSize >= 2)
        second = args[args.size()-2];
    }

    auto res = CLI::App::_parse_single(args, positional_only);
    auto newSize = args.size();
    auto sizeDiff = previousSize - newSize;
    if(res){
      if(sizeDiff >= 1){
        perArgCallbackPost(first);
        if(sizeDiff >= 2){
          if(sizeDiff == 2)
            perArgCallbackPost(second);
          else
            throw std::invalid_argument("_parse_single has consumed more than 2 raw argc arguments. You need to fix this function to process the situation correctly.");
        }
      }
    }
    return res;
  }

  public:
    std::function<void(void)> perArgCallbackPre;
    std::function<void(std::string)> perArgCallbackPost;
    std::function<void(std::string)> positionalArgImmediateCallback;
};

void Flags::Parse(int argc, char** argv) {
  subkati_args.push_back(argv[0]);
  num_jobs = num_cpus = sysconf(_SC_NPROCESSORS_ONLN);

  if (const char* makeflags = getenv("MAKEFLAGS")) {
    for (StringPiece tok : WordScanner(makeflags)) {
      if (!HasPrefix(tok, "-") && tok.find('=') != string::npos)
        cl_vars.emplace_back(std::string(tok.data(), tok.size()));
    }
  }
  MyApp app;
  auto appName = "ckati";
  {
    std::stringstream appMsg;
    appMsg << appName << " - An experimental GNU make clone. Version https://github.com/google/kati/commit/" << kGitVersion << " .";
    app.description(appMsg.str());
    app.name(appName);
  }

  app.fallthrough();
  
  auto makeGroup = app.add_option_group("make", "options mimicking ones of Make or just relevant to using kati as make");
  auto ninjaGroup = app.add_option_group("ninja", "options for using with ninja");
  auto katiGroup = app.add_option_group("kati", "settings of kati itself");
  auto featuresGroup = app.add_option_group("features", "additional features");
  auto debugGroup = app.add_option_group("debug", "flags useful for debugging");
  auto warningsControl = app.add_option_group("warnings", "enable warnings");
  
  ninjaGroup->add_option(app.add_flag("--no_ninja_prelude", no_ninja_prelude, "Disables outputing some lines into `build.ninja`. These lines are about ninja binary, \"pool local_pool\", count of jobs and _kati_always_build_ rule."));
  ninjaGroup->add_option(app.add_flag("--ninja", generate_ninja, "Transform a makefile into `build.ninja`."));
  ninjaGroup->add_option(app.add_flag("--empty_ninja_file", generate_empty_ninja, "outputs something additional into `build.ninja`"));
  ninjaGroup->add_option(app.add_option("--ninja_suffix", ninja_suffix, "Something added into end of files names. Both input and output filenames are affected."));
  ninjaGroup->add_option(app.add_option("--ninja_dir", ninja_dir, "directory of ninja binary"));
  ninjaGroup->add_option(app.add_option("--remote_num_jobs", remote_num_jobs, "count of jobs for ninja")->check(CLI::PositiveNumber));
  ninjaGroup->add_option(app.add_flag("--detect_android_echo", detect_android_echo, "disable .phony for Android"));
  
  makeGroup->add_option(app.add_option("-f,--file", makefile, "file"));
  makeGroup->add_option(app.add_flag("-s,--silent", is_silent_mode, "silent mode. currently does nothing. It controls global_echo variable but it doesn't control anything."));
  makeGroup->add_option(app.add_option("-j,--jobs", num_jobs, "num of processes")->check(CLI::PositiveNumber));
  makeGroup->add_option(app.add_flag("-i", is_dry_run, "dry run, don't run any commands from makefiles"));
  makeGroup->add_option(app.add_flag("--no_builtin_rules", no_builtin_rules, "Do not insert `.c.o`, `.cc.o` and some GNU Make advertising"));
  makeGroup->add_option(app.add_flag("-c", is_syntax_check_only, "only check makefile syntax, don't do anything else"));
  
  bool requestPrintingKatiVersion = false;
  katiGroup->add_option(app.add_flag("--version", requestPrintingKatiVersion, "print version of this program"));
  katiGroup->add_option(app.add_flag("--regen_ignoring_kati_binary", regen_ignoring_kati_binary));
  katiGroup->add_option(app.add_flag("--color_warnings", color_warnings, "show warnings using ECMA control codes"));
  katiGroup->add_option(app.add_flag("--gen_all_targets", gen_all_targets));
  katiGroup->add_option(app.add_flag("--regen", regen, "Check if regeneration is needed"));
  katiGroup->add_option(app.add_flag("--top_level_phony", top_level_phony));
  katiGroup->add_option(app.add_option("--ignore_optional_include", ignore_optional_include_pattern, "don't include a file if it matches this pattern"));
  katiGroup->add_option(app.add_option("--ignore_dirty", ignore_dirty_pattern, "files matching this pattern are not regenerated"));
  katiGroup->add_option(app.add_option("--no_ignore_dirty", no_ignore_dirty_pattern, "files matching this pattern are regenerated even if ignored by ignore_dirty"));
  katiGroup->add_option(app.add_option("--writable", writable, "List of some prefixes of file names."));
  
  featuresGroup->add_option(app.add_option("--goma_dir", goma_dir, "Directory of gomacc binary. See https://chromium.googlesource.com/infra/goma/client/+/master/README.md for more info."));
  featuresGroup->add_option(app.add_flag("--use_find_emulator", use_find_emulator, "emulate `find` command"));
  featuresGroup->add_option(app.add_flag("--detect_depfiles", detect_depfiles, "detects dependent files by looking for filenames looking like artifacts in commands"));
  
  debugGroup->add_option(app.add_flag("-d,--debug", enable_debug, "print debug messages"));
  debugGroup->add_option(app.add_flag("--kati_stats", enable_stat_logs, "Enable stat logs"));
  auto regenDebugOption = app.add_flag("--regen_debug", regen_debug, "print info about dirtiness status of files and about the decisions amde");
  debugGroup->add_option(regenDebugOption);
  debugGroup->add_option(app.add_flag_callback("--dump_kati_stamp", [&](){regen_debug=dump_kati_stamp=true;})); // ToDo: ->sets(regenDebugOption)
  
  warningsControl->add_option(app.add_flag("--warn", enable_kati_warnings, "Enable warnings"));
  warningsControl->add_option(app.add_flag("--werror_find_emulator", werror_find_emulator));
  warningsControl->add_option(app.add_flag("--werror_overriding_commands", werror_overriding_commands));
  warningsControl->add_option(app.add_flag("--warn_implicit_rules", warn_implicit_rules));
  warningsControl->add_option(app.add_flag("--werror_implicit_rules", werror_implicit_rules));
  warningsControl->add_option(app.add_flag("--warn_suffix_rules", warn_suffix_rules));
  warningsControl->add_option(app.add_flag("--werror_suffix_rules", werror_suffix_rules));
  warningsControl->add_option(app.add_flag("--warn_real_to_phony", warn_real_to_phony));
  warningsControl->add_option(app.add_flag("--werror_real_to_phony", werror_real_to_phony));
  auto warn_phony_looks_realPref = app.add_flag("--warn_phony_looks_real", warn_phony_looks_real);
  warningsControl->add_option(warn_phony_looks_realPref);
  warningsControl->add_option(app.add_flag_callback("--werror_phony_looks_real", [&](){warn_phony_looks_real=werror_phony_looks_real=true;})); // ToDo: ->sets(warn_phony_looks_realPref)
  warningsControl->add_option(app.add_flag("--werror_writable", werror_writable));

  bool should_propagate;
  app.perArgCallbackPre = [&]() -> void {
    should_propagate = true;
  };

  app.positionalArgImmediateCallback = [&](std::string arg) -> void {
    if (arg.find("=") != std::string::npos) {
      cl_vars.emplace_back(arg); // ToDo: improve CLI11 to use `std::span`s. emplace_back doesn't work here
    } else {
      should_propagate = false;
      targets_strings.emplace_back(arg); // arg may change address, arg contents has no static address (since std::string are on heap, previously pointers to argv were used which don't change addresses during whole program execution) too, causing UB. So we do 2 stages: populate the storage vector and then populate the stuff referring the data in it.
    }
  };

  app.perArgCallbackPost = [&](std::string arg) -> void {
    if(arg == "-f") { // cannot do it in callbacks, callbacks are executed when everything is parsed
      should_propagate = false;
    }
    if (should_propagate) {
      subkati_args.emplace_back(arg);
    }
  };
  
  app.add_option_function<std::string>("OTHER OPTS", [&](const std::string &arg){
    if (arg.data()[0] == '-') {
      ERROR("Unknown flag: %s", arg.data());
    }
  }, "restOpts");

  app.allow_extras();
  try {
    app.parse(argc, argv);
  } catch (const CLI::ParseError &e) {
    std::exit(app.exit(e));
  }

  if(requestPrintingKatiVersion){
    std::cout << kGitVersion << std::endl;
    std::exit(0);
  }

  targets_strings.shrink_to_fit();
  for(auto &s: targets_strings){
    targets.emplace_back(Intern(s));
  }
}
