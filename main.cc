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
#include <unistd.h>

#include "ast.h"
#include "dep.h"
#include "eval.h"
#include "exec.h"
#include "file.h"
#include "file_cache.h"
#include "fileutil.h"
#include "flags.h"
#include "func.h"
#include "log.h"
#include "parser.h"
#include "string_piece.h"
#include "stringprintf.h"
#include "strutil.h"
#include "time.h"
#include "var.h"

static const char* g_makefile;
static bool g_is_syntax_check_only;

static void ParseCommandLine(int argc, char* argv[],
                             vector<StringPiece>* targets,
                             vector<StringPiece>* cl_vars) {
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
    } else if (Exists("makefile")) {
      g_makefile = "makefile";
    } else if (Exists("Makefile")) {
      g_makefile = "Makefile";
    } else {
      ERROR("*** No targets specified and no makefile found.");
    }
  }
}

static void Quit() {
  QuitParser();
  QuitDepNodePool();
  QuitFuncTable();
  QuitSymtab();
}

static void ReadBootstrapMakefile(const vector<StringPiece>& targets,
                                  vector<AST*>* asts) {
  string bootstrap = (
      "CC:=cc\n"
      "CXX:=g++\n"
      "AR:=ar\n"
      "MAKE:=kati\n"
      // Pretend to be GNU make 3.81, for compatibility.
      "MAKE_VERSION:=3.81\n"
      "SHELL:=/bin/sh\n"
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
  bootstrap += StringPrintf("MAKECMDGOALS:=%s\n",
                            JoinStrings(targets, " ").c_str());

  char cwd[PATH_MAX];
  if (!getcwd(cwd, PATH_MAX)) {
    fprintf(stderr, "getcwd failed\n");
    CHECK(false);
  }
  bootstrap += StringPrintf("CURDIR:=%s\n", cwd);
  Parse(Intern(bootstrap), Loc("*bootstrap*", 0), asts);
}

static void SetVar(StringPiece l, VarOrigin origin, Vars* vars) {
  size_t found = l.find('=');
  CHECK(found != string::npos);
  StringPiece lhs = Intern(l.substr(0, found));
  StringPiece rhs = l.substr(found + 1);
  vars->Assign(lhs,
               new RecursiveVar(NewLiteral(rhs.data()), origin, rhs.data()));
}

extern "C" char** environ;
static void FillDefaultVars(const vector<StringPiece>& cl_vars, Vars* vars) {
  for (char** p = environ; *p; p++) {
    SetVar(*p, VarOrigin::ENVIRONMENT, vars);
  }
  for (StringPiece l : cl_vars) {
    SetVar(l, VarOrigin::COMMAND_LINE, vars);
  }
}

static int Run(const vector<StringPiece>& targets,
               const vector<StringPiece>& cl_vars) {
  MakefileCacheManager* cache_mgr = NewMakefileCacheManager();

  Vars* vars = new Vars();
  FillDefaultVars(cl_vars, vars);
  Evaluator* ev = new Evaluator(vars);

  vector<AST*> bootstrap_asts;
  ReadBootstrapMakefile(targets, &bootstrap_asts);
  ev->set_is_bootstrap(true);
  for (AST* ast : bootstrap_asts) {
    LOG("%s", ast->DebugString().c_str());
    ast->Eval(ev);
  }
  ev->set_is_bootstrap(false);

  vars->Assign("MAKEFILE_LIST",
               new SimpleVar(make_shared<string>(
                   StringPrintf(" %s", g_makefile)), VarOrigin::FILE));

  {
    ScopedTimeReporter tr("eval time");
    Makefile* mk = cache_mgr->ReadMakefile(g_makefile);
    for (AST* ast : mk->asts()) {
      LOG("%s", ast->DebugString().c_str());
      ast->Eval(ev);
    }
  }

  vector<DepNode*> nodes;
  {
    ScopedTimeReporter tr("make dep time");
    MakeDep(ev, ev->rules(), ev->rule_vars(), targets, &nodes);
  }

  for (const auto& p : ev->exports()) {
    const string& name = p.first.as_string();
    if (p.second) {
      Var* v = ev->LookupVar(name);
      shared_ptr<string> value = v->Eval(ev);
      LOG("setenv(%s, %s)", name.c_str(), value->c_str());
      setenv(name.c_str(), value->c_str(), 1);
    } else {
      LOG("unsetenv(%s)", name.c_str());
      unsetenv(name.c_str());
    }
  }

  if (g_is_syntax_check_only)
    return 0;

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
  vector<StringPiece> targets;
  vector<StringPiece> cl_vars;
  ParseCommandLine(argc, argv, &targets, &cl_vars);
  int r = Run(targets, cl_vars);
  Quit();
  return r;
}
