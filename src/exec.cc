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
#include <sys/wait.h>

#include <memory>
#include <string_view>
#include <unordered_map>
#include <utility>
#include <vector>

#include "command.h"
#include "dep.h"
#include "eval.h"
#include "expr.h"
#include "fileutil.h"
#include "flags.h"
#include "log.h"
#include "strutil.h"
#include "symtab.h"
#include "var.h"

namespace {

const double kNotExist = -2.0;
const double kProcessing = -1.0;

class Executor {
 public:
  explicit Executor(Evaluator* ev) : ce_(ev), num_commands_(0) {
    shell_ = ev->GetShell();
    shellflag_ = ev->GetShellFlag();
  }

  double ExecNode(const DepNode& n, const char* needed_by) {
    auto found = done_.find(n.output);
    if (found != done_.end()) {
      if (found->second == kProcessing) {
        WARN("Circular %s <- %s dependency dropped.",
             needed_by ? needed_by : "(null)", n.output.c_str());
      }
      return found->second;
    }
    ScopedFrame frame(
        ce_.evaluator()->Enter(FrameType::EXEC, n.output.c_str(), n.loc));

    done_[n.output] = kProcessing;
    double output_ts = GetTimestamp(n.output.c_str());

    LOG("ExecNode: %s for %s", n.output.c_str(),
        needed_by ? needed_by : "(null)");

    if (!n.has_rule && output_ts == kNotExist && !n.is_phony) {
      if (needed_by) {
        ERROR("*** No rule to make target '%s', needed by '%s'.",
              n.output.c_str(), needed_by);
      } else {
        ERROR("*** No rule to make target '%s'.", n.output.c_str());
      }
    }

    double latest = kProcessing;
    for (auto const& d : n.order_onlys) {
      if (Exists(d.second->output.str())) {
        continue;
      }
      double ts = ExecNode(*d.second, n.output.c_str());
      if (latest < ts)
        latest = ts;
    }

    for (auto const& d : n.deps) {
      double ts = ExecNode(*d.second, n.output.c_str());
      if (latest < ts)
        latest = ts;
    }

    if (output_ts >= latest && !n.is_phony) {
      done_[n.output] = output_ts;
      return output_ts;
    }

    auto commands = ce_.Eval(n);
    for (const Command& command : commands) {
      num_commands_ += 1;
      if (command.echo) {
        printf("%s\n", command.cmd.c_str());
        fflush(stdout);
      }
      if (!g_flags.is_dry_run) {
        std::string out;
        int result = RunCommand(shell_, shellflag_, command.cmd.c_str(),
                                RedirectStderr::STDOUT, &out);
        printf("%s", out.c_str());
        if (result != 0) {
          if (command.ignore_error) {
            fprintf(stderr, "[%s] Error %d (ignored)\n", command.output.c_str(),
                    WEXITSTATUS(result));
          } else {
            fprintf(stderr, "*** [%s] Error %d\n", command.output.c_str(),
                    WEXITSTATUS(result));
            exit(1);
          }
        }
      }
    }

    done_[n.output] = output_ts;
    return output_ts;
  }

  uint64_t Count() { return num_commands_; }

 private:
  CommandEvaluator ce_;
  std::unordered_map<Symbol, double> done_;
  std::string shell_;
  std::string shellflag_;
  uint64_t num_commands_;
};

}  // namespace

void Exec(const std::vector<NamedDepNode>& roots, Evaluator* ev) {
  Executor executor(ev);
  for (auto const& root : roots) {
    executor.ExecNode(*root.second, nullptr);
  }
  if (executor.Count() == 0) {
    for (auto const& root : roots) {
      printf("kati: Nothing to be done for `%s'.\n", root.first.c_str());
    }
  }
}
