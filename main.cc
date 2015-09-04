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

#include <limits.h>
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include <time.h>
#include <unistd.h>

#include "ast.h"
#include "dep.h"
#include "eval.h"
#include "exec.h"
#include "file.h"
#include "file_cache.h"
#include "fileutil.h"
#include "find.h"
#include "flags.h"
#include "func.h"
#include "log.h"
#include "ninja.h"
#include "parser.h"
#include "stats.h"
#include "string_piece.h"
#include "stringprintf.h"
#include "strutil.h"
#include "symtab.h"
#include "timeutil.h"
#include "var.h"

static const char* g_makefile;
static bool g_is_syntax_check_only;
static bool g_generate_ninja;
static bool g_regen;
static bool g_regen_ignoring_kati_binary;
static bool g_dump_kati_stamp;
static const char* g_ninja_suffix;
static const char* g_ninja_dir;
static bool g_use_find_emulator;

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

static void ParseCommandLine(int argc, char* argv[],
                             vector<Symbol>* targets,
                             vector<StringPiece>* cl_vars) {
  g_num_jobs = sysconf(_SC_NPROCESSORS_ONLN);
  const char* num_jobs_str;

  for (int i = 1; i < argc; i++) {
    const char* arg = argv[i];
    if (!strcmp(arg, "-f")) {
      g_makefile = argv[++i];
    } else if (!strcmp(arg, "-c")) {
      g_is_syntax_check_only = true;
    } else if (!strcmp(arg, "-i")) {
      g_is_dry_run = true;
    } else if (!strcmp(arg, "--kati_stats")) {
      g_enable_stat_logs = true;
    } else if (!strcmp(arg, "--ninja")) {
      g_generate_ninja = true;
    } else if (!strcmp(arg, "--gen_all_phony_targets")) {
      // TODO: Remove this.
      g_gen_all_phony_targets = true;
    } else if (!strcmp(arg, "--regen")) {
      // TODO: Make this default.
      g_regen = true;
    } else if (!strcmp(arg, "--regen_ignoring_kati_binary")) {
      g_regen_ignoring_kati_binary = true;
    } else if (!strcmp(arg, "--dump_kati_stamp")) {
      g_dump_kati_stamp = true;
    } else if (!strcmp(arg, "--detect_android_echo")) {
      g_detect_android_echo = true;
    } else if (ParseCommandLineOptionWithArg(
        "-j", argv, &i, &num_jobs_str)) {
      g_num_jobs = strtol(num_jobs_str, NULL, 10);
      if (g_num_jobs <= 0) {
        ERROR("Invalid -j flag: %s", num_jobs_str);
      }
    } else if (ParseCommandLineOptionWithArg(
        "--remote_num_jobs", argv, &i, &num_jobs_str)) {
      g_remote_num_jobs = strtol(num_jobs_str, NULL, 10);
      if (g_remote_num_jobs <= 0) {
        ERROR("Invalid -j flag: %s", num_jobs_str);
      }
    } else if (ParseCommandLineOptionWithArg(
        "--ninja_suffix", argv, &i, &g_ninja_suffix)) {
    } else if (ParseCommandLineOptionWithArg(
        "--ninja_dir", argv, &i, &g_ninja_dir)) {
    } else if (!strcmp(arg, "--use_find_emulator")) {
      g_use_find_emulator = true;
    } else if (!strcmp(arg, "--gen_regen_rule")) {
      // TODO: Make this default once we have removed unnecessary
      // command line change from Android build.
      g_gen_regen_rule = true;
    } else if (ParseCommandLineOptionWithArg(
        "--goma_dir", argv, &i, &g_goma_dir)) {
    } else if (ParseCommandLineOptionWithArg(
        "--ignore_optional_include",
        argv, &i, &g_ignore_optional_include_pattern)) {
    } else if (ParseCommandLineOptionWithArg(
        "--ignore_dirty",
        argv, &i, &g_ignore_dirty_pattern)) {
    } else if (arg[0] == '-') {
      ERROR("Unknown flag: %s", arg);
    } else {
      if (strchr(arg, '=')) {
        cl_vars->push_back(arg);
      } else {
        targets->push_back(Intern(arg));
      }
    }
  }
}

static void Init() {
  InitSymtab();
  InitFuncTable();
  InitDepNodePool();
  InitParser();

  if (g_makefile == NULL) {
    if (Exists("GNUmakefile")) {
      g_makefile = "GNUmakefile";
#if !defined(__APPLE__)
    } else if (Exists("makefile")) {
      g_makefile = "makefile";
#endif
    } else if (Exists("Makefile")) {
      g_makefile = "Makefile";
    }
  }
}

static void Quit() {
  ReportAllStats();

  QuitParser();
  QuitDepNodePool();
  QuitFuncTable();
  QuitSymtab();
}

static void ReadBootstrapMakefile(const vector<Symbol>& targets,
                                  vector<AST*>* asts) {
  string bootstrap = (
      "CC?=cc\n"
#if defined(__APPLE__)
      "CXX?=c++\n"
#else
      "CXX?=g++\n"
#endif
      "AR?=ar\n"
      // Pretend to be GNU make 3.81, for compatibility.
      "MAKE_VERSION?=3.81\n"
      "KATI?=ckati\n"
      // Overwrite $SHELL environment variable.
      "SHELL=/bin/sh\n"
      // TODO: Add more builtin vars.

      // http://www.gnu.org/software/make/manual/make.html#Catalogue-of-Rules
      // The document above is actually not correct. See default.c:
      // http://git.savannah.gnu.org/cgit/make.git/tree/default.c?id=4.1
      ".c.o:\n"
      "\t$(CC) $(CFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<\n"
      ".cc.o:\n"
      "\t$(CXX) $(CXXFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<\n"
      // TODO: Add more builtin rules.
                      );
  bootstrap += StringPrintf("MAKE?=make -j%d\n",
                            g_num_jobs < 1 ? 1 : g_num_jobs / 2);
  bootstrap += StringPrintf("MAKECMDGOALS?=%s\n",
                            JoinSymbols(targets, " ").c_str());

  char cwd[PATH_MAX];
  if (!getcwd(cwd, PATH_MAX)) {
    fprintf(stderr, "getcwd failed\n");
    CHECK(false);
  }
  bootstrap += StringPrintf("CURDIR:=%s\n", cwd);
  Parse(Intern(bootstrap).str(), Loc("*bootstrap*", 0), asts);
}

static void SetVar(StringPiece l, VarOrigin origin, Vars* vars) {
  size_t found = l.find('=');
  CHECK(found != string::npos);
  Symbol lhs = Intern(l.substr(0, found));
  StringPiece rhs = l.substr(found + 1);
  vars->Assign(lhs,
               new RecursiveVar(NewLiteral(rhs.data()), origin, rhs.data()));
}

extern "C" char** environ;

static int Run(const vector<Symbol>& targets,
               const vector<StringPiece>& cl_vars,
               const string& orig_args) {
  double start_time = GetTime();

  if (g_generate_ninja && (g_regen || g_dump_kati_stamp)) {
    ScopedTimeReporter tr("regen check time");
    if (!NeedsRegen(g_ninja_suffix, g_ninja_dir,
                    g_regen_ignoring_kati_binary,
                    g_dump_kati_stamp,
                    start_time, orig_args)) {
      printf("No need to regenerate ninja file\n");
      return 0;
    }
    if (g_dump_kati_stamp) {
      printf("Need to regenerate ninja file\n");
      return 0;
    }
    ClearGlobCache();
  }

  MakefileCacheManager* cache_mgr = NewMakefileCacheManager();

  Vars* vars = new Vars();
  for (char** p = environ; *p; p++) {
    SetVar(*p, VarOrigin::ENVIRONMENT, vars);
  }
  Evaluator* ev = new Evaluator(vars);

  vector<AST*> bootstrap_asts;
  ReadBootstrapMakefile(targets, &bootstrap_asts);
  ev->set_is_bootstrap(true);
  for (AST* ast : bootstrap_asts) {
    LOG("%s", ast->DebugString().c_str());
    ast->Eval(ev);
  }
  ev->set_is_bootstrap(false);

  for (StringPiece l : cl_vars) {
    SetVar(l, VarOrigin::COMMAND_LINE, ev->mutable_vars());
  }

  vars->Assign(Intern("MAKEFILE_LIST"),
               new SimpleVar(StringPrintf(" %s", g_makefile),
                             VarOrigin::FILE));

  {
    ScopedTimeReporter tr("eval time");
    Makefile* mk = cache_mgr->ReadMakefile(g_makefile);
    for (AST* ast : mk->asts()) {
      LOG("%s", ast->DebugString().c_str());
      ast->Eval(ev);
    }
  }

  for (ParseErrorAST* err : GetParseErrors()) {
    WARN("%s:%d: warning for parse error in an unevaluated line: %s",
         LOCF(err->loc()), err->msg.c_str());
  }

  vector<DepNode*> nodes;
  {
    ScopedTimeReporter tr("make dep time");
    MakeDep(ev, ev->rules(), ev->rule_vars(), targets, &nodes);
  }

  if (g_is_syntax_check_only)
    return 0;

  if (g_generate_ninja) {
    ScopedTimeReporter tr("generate ninja time");
    GenerateNinja(g_ninja_suffix, g_ninja_dir, nodes, ev, !targets.empty(),
                  orig_args, start_time);
    return 0;
  }

  for (const auto& p : ev->exports()) {
    const Symbol name = p.first;
    if (p.second) {
      Var* v = ev->LookupVar(name);
      const string&& value = v->Eval(ev);
      LOG("setenv(%s, %s)", name.c_str(), value.c_str());
      setenv(name.c_str(), value.c_str(), 1);
    } else {
      LOG("unsetenv(%s)", name.c_str());
      unsetenv(name.c_str());
    }
  }

  {
    ScopedTimeReporter tr("exec time");
    Exec(nodes, ev);
  }

  for (AST* ast : bootstrap_asts)
    delete ast;
  delete ev;
  delete vars;
  delete cache_mgr;

  return 0;
}

int main(int argc, char* argv[]) {
  Init();
  string orig_args;
  for (int i = 0; i < argc; i++) {
    if (i)
      orig_args += ' ';
    orig_args += argv[i];
  }
  vector<Symbol> targets;
  vector<StringPiece> cl_vars;
  ParseCommandLine(argc, argv, &targets, &cl_vars);
  if (g_makefile == NULL)
    ERROR("*** No targets specified and no makefile found.");
  // This depends on command line flags.
  if (g_use_find_emulator)
    InitFindEmulator();
  int r = Run(targets, cl_vars, orig_args);
  Quit();
  return r;
}
