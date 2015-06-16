#include "exec.h"

#include <stdio.h>
#include <stdlib.h>

#include <memory>
#include <unordered_map>

#include "dep.h"
#include "eval.h"
#include "log.h"
#include "string_piece.h"
#include "value.h"

namespace {

struct Runner {
  Runner()
      : echo(true), ignore_error(false) {
  }
  StringPiece output;
  shared_ptr<string> cmd;
  bool echo;
  bool ignore_error;
  //StringPiece shell;
};

}  // namespace

class Executor {
 public:
  explicit Executor(const Vars* vars)
      : vars_(vars) {
  }

  void ExecNode(DepNode* n, DepNode* needed_by) {
    if (done_[n->output])
      return;

    LOG("ExecNode: %s for %s",
        n->output.as_string().c_str(),
        needed_by ? needed_by->output.as_string().c_str() : "(null)");

    for (DepNode* d : n->deps) {
#if 0
      if (d.is_order_only && exists(d->output)) {
      }
#endif
      ExecNode(d, n);
    }

    vector<Runner*> runners;
    CreateRunners(n, &runners);
    for (Runner* runner : runners) {
      if (runner->echo) {
        printf("%s\n", runner->cmd->c_str());
        fflush(stdout);
      }
      system(runner->cmd->c_str());
      delete runner;
    }
  }

  void CreateRunners(DepNode* n, vector<Runner*>* runners) {
    unique_ptr<Evaluator> ev(new Evaluator(vars_));
    for (Value* v : n->cmds) {
      shared_ptr<string> cmd = v->Eval(ev.get());
      while (true) {
        size_t index = cmd->find('\n');
        if (index == string::npos)
          break;

        Runner* runner = new Runner;
        runner->output = n->output;
        runner->cmd = make_shared<string>(cmd->substr(0, index));
        runners->push_back(runner);
        cmd = make_shared<string>(cmd->substr(index + 1));
      }
      Runner* runner = new Runner;
      runner->output = n->output;
      runner->cmd = cmd;
      runners->push_back(runner);
      continue;
    }
  }

 private:
  const Vars* vars_;
  unordered_map<StringPiece, bool> done_;

};

void Exec(const vector<DepNode*>& roots, const Vars* vars) {
  unique_ptr<Executor> executor(new Executor(vars));
  for (DepNode* root : roots) {
    executor->ExecNode(root, NULL);
  }
}
