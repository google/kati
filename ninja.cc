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

#include "ninja.h"

#include <stdio.h>

#include <memory>
#include <string>
#include <unordered_set>

#include "command.h"
#include "dep.h"
#include "eval.h"
#include "log.h"
#include "string_piece.h"
#include "stringprintf.h"
#include "strutil.h"
#include "var.h"

class NinjaGenerator {
 public:
  explicit NinjaGenerator(Evaluator* ev)
      : ce_(ev), ev_(ev), fp_(NULL), rule_id_(0) {
    ev_->set_avoid_io(true);
  }

  ~NinjaGenerator() {
    ev_->set_avoid_io(false);
  }

  void Generate(const vector<DepNode*>& nodes) {
    GenerateShell();
    GenerateNinja(nodes);
  }

 private:
  string GenRuleName() {
    return StringPrintf("rule%d", rule_id_++);
  }

  StringPiece TranslateCommand(const char* in) {
    const size_t orig_size = cmd_buf_.size();
    bool prev_backslash = false;
    char quote = 0;
    bool done = false;
    for (; *in && !done; in++) {
      switch (*in) {
        case '#':
          if (quote == 0 && !prev_backslash) {
            done = true;
            break;
          }

        case '\'':
        case '"':
        case '`':
          if (quote) {
            if (quote == *in)
              quote = 0;
          } else if (!prev_backslash) {
            quote = *in;
          }
          cmd_buf_ += *in;
          break;

        case '$':
          cmd_buf_ += "$$";
          break;

        case '\t':
          cmd_buf_ += ' ';
          break;

        case '\n':
          if (prev_backslash) {
            cmd_buf_[cmd_buf_.size()-1] = ' ';
          } else {
            cmd_buf_ += ' ';
          }
          break;

        case '\\':
          prev_backslash = !prev_backslash;
          cmd_buf_ += '\\';
          break;

        default:
          cmd_buf_ += *in;
          prev_backslash = false;
      }
    }

    while (true) {
      char c = cmd_buf_[cmd_buf_.size()-1];
      if (!isspace(c) && c != ';')
        break;
      cmd_buf_.resize(cmd_buf_.size() - 1);
    }

    return StringPiece(cmd_buf_.data() + orig_size,
                       cmd_buf_.size() - orig_size);
  }

  void GenShellScript(const vector<Command*>& commands) {
    //bool use_gomacc = false;
    bool should_ignore_error = false;
    cmd_buf_.clear();
    for (const Command* c : commands) {
      if (!cmd_buf_.empty()) {
        if (should_ignore_error) {
          cmd_buf_ += " ; ";
        } else {
          cmd_buf_ += " && ";
        }
      }
      should_ignore_error = c->ignore_error;

      const char* in = c->cmd->c_str();
      while (isspace(*in))
        in++;

      bool needs_subshell = commands.size() > 1;
      if (*in == '(') {
        needs_subshell = false;
      }

      if (needs_subshell)
        cmd_buf_ += '(';

      StringPiece translated = TranslateCommand(in);
      if (translated.empty()) {
        cmd_buf_ += "true";
      } else {
        // TODO: flip use_gomacc
      }

      if (c == commands.back() && c->ignore_error) {
        cmd_buf_ += " ; true";
      }

      if (needs_subshell)
        cmd_buf_ += ')';
    }
  }

  void EmitNode(DepNode* node) {
    auto p = done_.insert(node->output);
    if (!p.second)
      return;

    if (node->cmds.empty() && node->deps.empty() && !node->is_phony)
      return;

    vector<Command*> commands;
    ce_.Eval(node, &commands);

    string rule_name = "phony";
    if (!commands.empty()) {
      rule_name = GenRuleName();
      fprintf(fp_, "rule %s\n", rule_name.c_str());
      fprintf(fp_, " description = build $out\n");

      GenShellScript(commands);
      // TODO: depfile

      // It seems Linux is OK with ~130kB.
      // TODO: Find this number automatically.
      if (cmd_buf_.size() > 100 * 1000) {
        fprintf(fp_, " rspfile = $out.rsp\n");
        fprintf(fp_, " rspfile_content = %s\n", cmd_buf_.c_str());
        fprintf(fp_, " command = sh $out.rsp\n");
      } else {
        fprintf(fp_, " command = %s\n", cmd_buf_.c_str());
      }
    }

    EmitBuild(node, rule_name);
    // TODO: goma

    for (DepNode* d : node->deps) {
      EmitNode(d);
    }
  }

  void EmitBuild(DepNode* node, const string& rule_name) {
    fprintf(fp_, "build %.*s: %s", SPF(node->output), rule_name.c_str());
    vector<StringPiece> order_onlys;
    for (DepNode* d : node->deps) {
      if (d->is_order_only) {
        order_onlys.push_back(d->output);
      } else {
        fprintf(fp_, " %.*s", SPF(d->output));
      }
    }
    if (!order_onlys.empty()) {
      fprintf(fp_, " ||");
      for (StringPiece oo : order_onlys) {
        fprintf(fp_, " %.*s", SPF(oo));
      }
    }
    fprintf(fp_, "\n");
  }

  void GenerateNinja(const vector<DepNode*>& nodes) {
    fp_ = fopen("build.ninja", "wb");
    if (fp_ == NULL)
      PERROR("fopen(build.ninja) failed");

    fprintf(fp_, "# Generated by kati\n");
    fprintf(fp_, "\n");

    for (DepNode* node : nodes) {
      EmitNode(node);
    }

    fclose(fp_);
  }

  void GenerateShell() {
#if 0
    Var* v = ev->LookupVar("SHELL");
    shell_ = v->Eval(ev);
    if (shell_->empty())
      shell_ = make_shared<string>("/bin/sh");
#endif
  }

  CommandEvaluator ce_;
  Evaluator* ev_;
  FILE* fp_;
  unordered_set<StringPiece> done_;
  int rule_id_;
  string cmd_buf_;
};

void GenerateNinja(const vector<DepNode*>& nodes, Evaluator* ev) {
  NinjaGenerator ng(ev);
  ng.Generate(nodes);
}
