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

#include "exec.h"

#include <stdio.h>
#include <stdlib.h>

#include <memory>
#include <unordered_map>
#include <utility>
#include <vector>

#include "command.h"
#include "dep.h"
#include "eval.h"
#include "fileutil.h"
#include "flags.h"
#include "log.h"
#include "string_piece.h"
#include "strutil.h"
#include "symtab.h"
#include "value.h"
#include "var.h"

namespace {

class Executor {
 public:
  explicit Executor(Evaluator* ev)
      : ce_(ev) {
  }

  void ExecNode(DepNode* n, DepNode* needed_by) {
    if (done_[n->output])
      return;
    done_[n->output] = true;

    LOG("ExecNode: %s for %s",
        n->output.c_str(),
        needed_by ? needed_by->output.c_str() : "(null)");

    for (DepNode* d : n->deps) {
      if (d->is_order_only && Exists(d->output.str())) {
        continue;
      }
      // TODO: Check the timestamp.
      if (Exists(d->output.str())) {
        continue;
      }
      ExecNode(d, n);
    }

    vector<Command*> commands;
    ce_.Eval(n, &commands);
    for (Command* command : commands) {
      if (command->echo) {
        printf("%s\n", command->cmd->c_str());
        fflush(stdout);
      }
      if (!g_is_dry_run) {
        int result = system(command->cmd->c_str());
        if (result != 0) {
          if (command->ignore_error) {
            fprintf(stderr, "[%s] Error %d (ignored)\n",
                    command->output.c_str(), WEXITSTATUS(result));
          } else {
            fprintf(stderr, "*** [%s] Error %d\n",
                    command->output.c_str(), WEXITSTATUS(result));
            exit(1);
          }
        }
      }
      delete command;
    }
  }

 private:
  CommandEvaluator ce_;
  unordered_map<Symbol, bool> done_;
};

}  // namespace

void Exec(const vector<DepNode*>& roots, Evaluator* ev) {
  unique_ptr<Executor> executor(new Executor(ev));
  for (DepNode* root : roots) {
    executor->ExecNode(root, NULL);
  }
}
