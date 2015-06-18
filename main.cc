#include <stdio.h>
#include <string.h>

#include "ast.h"
#include "dep.h"
#include "eval.h"
#include "exec.h"
#include "file.h"
#include "file_cache.h"
#include "fileutil.h"
#include "func.h"
#include "log.h"
#include "parser.h"
#include "string_piece.h"
#include "strutil.h"
#include "var.h"

static const char* g_makefile;

static void ParseCommandLine(int argc, char* argv[],
                             vector<StringPiece>* targets) {
  for (int i = 1; i < argc; i++) {
    const char* arg = argv[i];
    if (!strcmp(arg, "-f")) {
      g_makefile = argv[++i];
    } else if (arg[0] == '-') {
      ERROR("Unknown flag: %s", arg);
    } else {
      targets->push_back(Intern(arg));
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

static int Run(const vector<StringPiece>& targets) {
  MakefileCacheManager* cache_mgr = NewMakefileCacheManager();
  Makefile* mk = cache_mgr->ReadMakefile(g_makefile);

  // TODO: Fill env, etc.
  Vars* vars = new Vars();
  Evaluator* ev = new Evaluator(vars);
  for (AST* ast : mk->asts()) {
    LOG("%s", ast->DebugString().c_str());
    ast->Eval(ev);
  }

  EvalResult* er = ev->GetEvalResult();
  for (auto p : *er->vars) {
    vars->Assign(p.first, p.second);
  }
  er->vars->clear();

  vector<DepNode*> nodes;
  MakeDep(er->rules, *vars, er->rule_vars, targets, &nodes);

  Exec(nodes, vars);

  delete er;
  delete ev;
  delete vars;
  delete cache_mgr;

  return mk != 0;
}

int main(int argc, char* argv[]) {
  Init();
  vector<StringPiece> targets;
  ParseCommandLine(argc, argv, &targets);
  int r = Run(targets);
  Quit();
  return r;
}
