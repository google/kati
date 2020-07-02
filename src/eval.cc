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

#include "eval.h"

#include <errno.h>
#include <pthread.h>
#include <stdio.h>
#include <string.h>

#include "expr.h"
#include "file.h"
#include "file_cache.h"
#include "fileutil.h"
#include "parser.h"
#include "rule.h"
#include "stats.h"
#include "stmt.h"
#include "strutil.h"
#include "symtab.h"
#include "var.h"


IncludeTreeNode::IncludeTreeNode(const string& file):
  included_file_(file),
  include_location_() {
}

IncludeTreeNode::~IncludeTreeNode() {
}

IncludeTreeNode::IncludeTreeNode(const IncludeStmt& stmt, const std::string& file) :
  included_file_(file),
  include_location_(stmt.loc()) {
}

void IncludeTreeNode::Add(std::unique_ptr<IncludeTreeNode> child) {
  children_.push_back(std::move(child));
}


IncludeGraphNode::IncludeGraphNode(const IncludeTreeNode& tree_node) :
    filename_(tree_node.IncludedFile()) {
}

IncludeGraphNode::~IncludeGraphNode() {
}


IncludeGraph::IncludeGraph() {
}

IncludeGraph::~IncludeGraph() {
}

void IncludeGraph::DumpJSON(FILE* output) {
  fprintf(output,"{\n");
  fprintf(output, "  \"nodes\": [");
  bool first_node = true;

  for (const auto& node : nodes_) {
    if (first_node) {
      first_node = false;
      fprintf(output, "\n");
    } else {
      fprintf(output, ",\n");
    }

    fprintf(output, "    {\n");
    // TODO(lberki): Quote all these strings properly
    fprintf(output, "      \"file\": \"%s\",\n", node.first.c_str());
    fprintf(output, "      \"includes\": [");
    bool first_include = true;
    for (const auto& include : node.second->includes_) {
      if (first_include) {
        first_include = false;
        fprintf(output, "\n");
      } else {
        fprintf(output, ",\n");
      }

      fprintf(output, "        \"%s\"", include.c_str());
    }
    fprintf(output, "\n      ]\n");
    fprintf(output, "    }");
  }

  fprintf(output, "\n");
  fprintf(output, "  ]\n");
  fprintf(output, "}\n");
}

void IncludeGraph::MergeTreeNode(const IncludeTreeNode &tree_node) {
  std::unique_ptr<IncludeGraphNode>& graph_node = nodes_[tree_node.IncludedFile()];
  if (graph_node.get() == nullptr) {
    graph_node.reset(new IncludeGraphNode(tree_node));
  }

  for (const auto& child : tree_node.Children()) {
    graph_node->includes_.insert(child->IncludedFile());
    MergeTreeNode(*child);
  }
}

Evaluator::Evaluator()
    : last_rule_(NULL),
      current_scope_(NULL),
      avoid_io_(false),
      eval_depth_(0),
      posix_sym_(Intern(".POSIX")),
      is_posix_(false),
      export_error_(false) {
#if defined(__APPLE__)
  stack_size_ = pthread_get_stacksize_np(pthread_self());
  stack_addr_ = (char*)pthread_get_stackaddr_np(pthread_self()) - stack_size_;
#else
  pthread_attr_t attr;
  CHECK(pthread_getattr_np(pthread_self(), &attr) == 0);
  CHECK(pthread_attr_getstack(&attr, &stack_addr_, &stack_size_) == 0);
  CHECK(pthread_attr_destroy(&attr) == 0);
#endif

  lowest_stack_ = (char*)stack_addr_ + stack_size_;
  LOG_STAT("Stack size: %zd bytes", stack_size_);
}

Evaluator::~Evaluator() {
  // delete vars_;
  // for (auto p : rule_vars) {
  //   delete p.second;
  // }
}

void Evaluator::in_command_line() {
  is_commandline_ = true;
  include_tree_.release();
  include_tree_stack_.clear();
}

void Evaluator::in_toplevel_makefile(const string& makefile) {
  is_commandline_ = false;
  include_tree_.reset(new IncludeTreeNode(makefile));
  include_tree_stack_.clear();
  include_tree_stack_.push_back(include_tree_.get());
}

Var* Evaluator::EvalRHS(Symbol lhs,
                        Value* rhs_v,
                        StringPiece orig_rhs,
                        AssignOp op,
                        bool is_override,
                        bool* needs_assign) {
  VarOrigin origin;
  if (is_bootstrap_) {
    origin = VarOrigin::DEFAULT;
  } else if (is_commandline_) {
    origin = VarOrigin::COMMAND_LINE;
  } else if (is_override) {
    origin = VarOrigin::OVERRIDE;
  } else {
    origin = VarOrigin::FILE;
  }

  Var* result = NULL;
  Var* prev = NULL;
  *needs_assign = true;

  switch (op) {
    case AssignOp::COLON_EQ: {
      prev = PeekVarInCurrentScope(lhs);
      result = new SimpleVar(origin, this, rhs_v);
      break;
    }
    case AssignOp::EQ:
      prev = PeekVarInCurrentScope(lhs);
      result = new RecursiveVar(rhs_v, origin, orig_rhs);
      break;
    case AssignOp::PLUS_EQ: {
      prev = LookupVarInCurrentScope(lhs);
      if (!prev->IsDefined()) {
        result = new RecursiveVar(rhs_v, origin, orig_rhs);
      } else if (prev->ReadOnly()) {
        Error(StringPrintf("*** cannot assign to readonly variable: %s",
                           lhs.c_str()));
      } else {
        result = prev;
        result->AppendVar(this, rhs_v);
        *needs_assign = false;
      }
      break;
    }
    case AssignOp::QUESTION_EQ: {
      prev = LookupVarInCurrentScope(lhs);
      if (!prev->IsDefined()) {
        result = new RecursiveVar(rhs_v, origin, orig_rhs);
      } else {
        result = prev;
        *needs_assign = false;
      }
      break;
    }
  }

  if (prev != NULL) {
    prev->Used(this, lhs);
    if (prev->Deprecated() && *needs_assign) {
      result->SetDeprecated(prev->DeprecatedMessage());
    }
  }

  LOG("Assign: %s=%s", lhs.c_str(), result->DebugString().c_str());
  return result;
}

void Evaluator::EvalAssign(const AssignStmt* stmt) {
  loc_ = stmt->loc();
  last_rule_ = NULL;
  Symbol lhs = stmt->GetLhsSymbol(this);
  if (lhs.empty())
    Error("*** empty variable name.");

  if (lhs == kKatiReadonlySym) {
    string rhs;
    stmt->rhs->Eval(this, &rhs);
    for (auto const& name : WordScanner(rhs)) {
      Var* var = Intern(name).GetGlobalVar();
      if (!var->IsDefined()) {
        Error(
            StringPrintf("*** unknown variable: %s", name.as_string().c_str()));
      }
      var->SetReadOnly();
    }
    return;
  }

  bool needs_assign;
  Var* var =
      EvalRHS(lhs, stmt->rhs, stmt->orig_rhs, stmt->op,
              stmt->directive == AssignDirective::OVERRIDE, &needs_assign);
  if (needs_assign) {
    bool readonly;
    lhs.SetGlobalVar(var, stmt->directive == AssignDirective::OVERRIDE,
                     &readonly);
    if (readonly) {
      Error(StringPrintf("*** cannot assign to readonly variable: %s",
                         lhs.c_str()));
    }
  }

  if (stmt->is_final) {
    var->SetReadOnly();
  }
}

// With rule broken into
//   <before_term> <term> <after_term>
// parses <before_term> into Symbol instances until encountering ':'
// Returns the remainder of <before_term>.
static StringPiece ParseRuleTargets(const Loc& loc,
                                    const StringPiece& before_term,
                                    vector<Symbol>* targets,
                                    bool* is_pattern_rule) {
  size_t pos = before_term.find(':');
  if (pos == string::npos) {
    ERROR_LOC(loc, "*** missing separator.");
  }
  StringPiece targets_string = before_term.substr(0, pos);
  size_t pattern_rule_count = 0;
  for (auto const& word : WordScanner(targets_string)) {
    StringPiece target = TrimLeadingCurdir(word);
    targets->push_back(Intern(target));
    if (Rule::IsPatternRule(target)) {
      ++pattern_rule_count;
    }
  }
  // Check consistency: either all outputs are patterns or none.
  if (pattern_rule_count && (pattern_rule_count != targets->size())) {
    ERROR_LOC(loc, "*** mixed implicit and normal rules: deprecated syntax");
  }
  *is_pattern_rule = pattern_rule_count;
  return before_term.substr(pos + 1);
}

void Evaluator::MarkVarsReadonly(Value* vars_list) {
  string vars_list_string;
  vars_list->Eval(this, &vars_list_string);
  for (auto const& name : WordScanner(vars_list_string)) {
    Var* var = current_scope_->Lookup(Intern(name));
    if (!var->IsDefined()) {
      Error(StringPrintf("*** unknown variable: %s", name.as_string().c_str()));
    }
    var->SetReadOnly();
  }
}

void Evaluator::EvalRuleSpecificAssign(const vector<Symbol>& targets,
                                       const RuleStmt* stmt,
                                       const StringPiece& after_targets,
                                       size_t separator_pos) {
  StringPiece var_name;
  StringPiece rhs_string;
  AssignOp assign_op;
  ParseAssignStatement(after_targets, separator_pos, &var_name, &rhs_string,
                       &assign_op);
  Symbol var_sym = Intern(var_name);
  bool is_final = (stmt->sep == RuleStmt::SEP_FINALEQ);
  for (Symbol target : targets) {
    auto p = rule_vars_.emplace(target, nullptr);
    if (p.second) {
      p.first->second = new Vars;
    }

    Value* rhs;
    if (rhs_string.empty()) {
      rhs = stmt->rhs;
    } else if (stmt->rhs) {
      StringPiece sep(stmt->sep == RuleStmt::SEP_SEMICOLON ? " ; " : " = ");
      rhs = Value::NewExpr(Value::NewLiteral(rhs_string),
                           Value::NewLiteral(sep), stmt->rhs);
    } else {
      rhs = Value::NewLiteral(rhs_string);
    }

    current_scope_ = p.first->second;
    if (var_sym == kKatiReadonlySym) {
      MarkVarsReadonly(rhs);
    } else {
      bool needs_assign;
      Var* rhs_var = EvalRHS(var_sym, rhs, StringPiece("*TODO*"), assign_op,
                             false, &needs_assign);
      if (needs_assign) {
        bool readonly;
        rhs_var->SetAssignOp(assign_op);
        current_scope_->Assign(var_sym, rhs_var, &readonly);
        if (readonly) {
          Error(StringPrintf("*** cannot assign to readonly variable: %s",
                             var_name));
        }
      }
      if (is_final) {
        rhs_var->SetReadOnly();
      }
    }
    current_scope_ = NULL;
  }
}

void Evaluator::EvalRule(const RuleStmt* stmt) {
  loc_ = stmt->loc();
  last_rule_ = NULL;

  const string&& before_term = stmt->lhs->Eval(this);
  // See semicolon.mk.
  if (before_term.find_first_not_of(" \t;") == string::npos) {
    if (stmt->sep == RuleStmt::SEP_SEMICOLON)
      Error("*** missing rule before commands.");
    return;
  }

  vector<Symbol> targets;
  bool is_pattern_rule;
  StringPiece after_targets =
      ParseRuleTargets(loc_, before_term, &targets, &is_pattern_rule);
  bool is_double_colon = (after_targets[0] == ':');
  if (is_double_colon) {
    after_targets = after_targets.substr(1);
  }

  // Figure out if this is a rule-specific variable assignment.
  // It is an assignment when either after_targets contains an assignment token
  // or separator is an assignment token, but only if there is no ';' before the
  // first assignment token.
  size_t separator_pos = after_targets.find_first_of("=;");
  char separator = '\0';
  if (separator_pos != string::npos) {
    separator = after_targets[separator_pos];
  } else if (separator_pos == string::npos &&
             (stmt->sep == RuleStmt::SEP_EQ ||
              stmt->sep == RuleStmt::SEP_FINALEQ)) {
    separator_pos = after_targets.size();
    separator = '=';
  }

  // If variable name is not empty, we have rule- or target-specific
  // variable assignment.
  if (separator == '=' && separator_pos) {
    EvalRuleSpecificAssign(targets, stmt, after_targets, separator_pos);
    return;
  }

  if (!separator_pos) {
    // We used to make this a warning and otherwise accept it, but Make 4.1
    // calls this out as an error, so let's follow.
    Error("*** empty variable name.");
  }

  Rule* rule = new Rule();
  rule->loc = loc_;
  rule->is_double_colon = is_double_colon;
  if (is_pattern_rule) {
    rule->output_patterns.swap(targets);
  } else {
    rule->outputs.swap(targets);
  }
  rule->ParsePrerequisites(after_targets, separator_pos, stmt);

  if (stmt->sep == RuleStmt::SEP_SEMICOLON) {
    rule->cmds.push_back(stmt->rhs);
  }

  for (Symbol o : rule->outputs) {
    if (o == posix_sym_)
      is_posix_ = true;
  }

  LOG("Rule: %s", rule->DebugString().c_str());
  rules_.push_back(rule);
  last_rule_ = rule;
}

void Evaluator::EvalCommand(const CommandStmt* stmt) {
  loc_ = stmt->loc();

  if (!last_rule_) {
    vector<Stmt*> stmts;
    ParseNotAfterRule(stmt->orig, stmt->loc(), &stmts);
    for (Stmt* a : stmts)
      a->Eval(this);
    return;
  }

  last_rule_->cmds.push_back(stmt->expr);
  if (last_rule_->cmd_lineno == 0)
    last_rule_->cmd_lineno = stmt->loc().lineno;
  LOG("Command: %s", Value::DebugString(stmt->expr).c_str());
}

void Evaluator::EvalIf(const IfStmt* stmt) {
  loc_ = stmt->loc();

  bool is_true;
  switch (stmt->op) {
    case CondOp::IFDEF:
    case CondOp::IFNDEF: {
      string var_name;
      stmt->lhs->Eval(this, &var_name);
      Symbol lhs = Intern(TrimRightSpace(var_name));
      if (lhs.str().find_first_of(" \t") != string::npos)
        Error("*** invalid syntax in conditional.");
      Var* v = LookupVarInCurrentScope(lhs);
      v->Used(this, lhs);
      is_true = (v->String().empty() == (stmt->op == CondOp::IFNDEF));
      break;
    }
    case CondOp::IFEQ:
    case CondOp::IFNEQ: {
      const string&& lhs = stmt->lhs->Eval(this);
      const string&& rhs = stmt->rhs->Eval(this);
      is_true = ((lhs == rhs) == (stmt->op == CondOp::IFEQ));
      break;
    }
    default:
      CHECK(false);
      abort();
  }

  const vector<Stmt*>* stmts;
  if (is_true) {
    stmts = &stmt->true_stmts;
  } else {
    stmts = &stmt->false_stmts;
  }
  for (Stmt* a : *stmts) {
    LOG("%s", a->DebugString().c_str());
    a->Eval(this);
  }
}

void Evaluator::DoInclude(const string& fname) {
  CheckStack();
  COLLECT_STATS_WITH_SLOW_REPORT("included makefiles", fname.c_str());

  Makefile* mk = MakefileCacheManager::Get()->ReadMakefile(fname);
  if (!mk->Exists()) {
    Error(StringPrintf("%s does not exist", fname.c_str()));
  }

  Var* var_list = LookupVar(Intern("MAKEFILE_LIST"));
  var_list->AppendVar(
      this, Value::NewLiteral(Intern(TrimLeadingCurdir(fname)).str()));
  for (Stmt* stmt : mk->stmts()) {
    LOG("%s", stmt->DebugString().c_str());
    stmt->Eval(this);
  }

  for (auto& mk : profiled_files_) {
    stats.MarkInteresting(mk);
  }
  profiled_files_.clear();
}

void Evaluator::EvalInclude(const IncludeStmt* stmt) {
  loc_ = stmt->loc();
  last_rule_ = NULL;

  const string&& pats = stmt->expr->Eval(this);
  for (StringPiece pat : WordScanner(pats)) {
    ScopedTerminator st(pat);
    vector<string>* files;
    Glob(pat.data(), &files);

    if (stmt->should_exist) {
      if (files->empty()) {
        // TODO: Kati does not support building a missing include file.
        Error(StringPrintf("%s: %s", pat.data(), strerror(errno)));
      }
    }

    include_stack_.push_back(stmt->loc());

    for (const string& fname : *files) {
      if (!stmt->should_exist && g_flags.ignore_optional_include_pattern &&
          Pattern(g_flags.ignore_optional_include_pattern).Match(fname)) {
        continue;
      }

      IncludeTreeNode* include_node = new IncludeTreeNode(*stmt, fname);
      include_tree_stack_.back()->Add(std::unique_ptr<IncludeTreeNode>(include_node));
      include_tree_stack_.push_back(include_node);

      DoInclude(fname);

      include_tree_stack_.pop_back();
    }
    include_stack_.pop_back();
  }
}

void Evaluator::EvalExport(const ExportStmt* stmt) {
  loc_ = stmt->loc();
  last_rule_ = NULL;

  const string&& exports = stmt->expr->Eval(this);
  for (StringPiece tok : WordScanner(exports)) {
    size_t equal_index = tok.find('=');
    StringPiece lhs;
    if (equal_index == string::npos) {
      lhs = tok;
    } else if (equal_index == 0 ||
               (equal_index == 1 &&
                (tok[0] == ':' || tok[0] == '?' || tok[0] == '+'))) {
      // Do not export tokens after an assignment.
      break;
    } else {
      StringPiece rhs;
      AssignOp op;
      ParseAssignStatement(tok, equal_index, &lhs, &rhs, &op);
    }
    Symbol sym = Intern(lhs);
    exports_[sym] = stmt->is_export;

    if (export_message_) {
      const char* prefix = "";
      if (!stmt->is_export) {
        prefix = "un";
      }

      if (export_error_) {
        Error(StringPrintf("*** %s: %sexport is obsolete%s.", sym.c_str(),
                           prefix, export_message_->c_str()));
      } else {
        WARN_LOC(loc(), "%s: %sexport has been deprecated%s.", sym.c_str(),
                 prefix, export_message_->c_str());
      }
    }
  }
}

Var* Evaluator::LookupVarGlobal(Symbol name) {
  Var* v = name.GetGlobalVar();
  if (v->IsDefined())
    return v;
  used_undefined_vars_.insert(name);
  return v;
}

Var* Evaluator::LookupVar(Symbol name) {
  if (current_scope_) {
    Var* v = current_scope_->Lookup(name);
    if (v->IsDefined())
      return v;
  }
  return LookupVarGlobal(name);
}

Var* Evaluator::PeekVar(Symbol name) {
  if (current_scope_) {
    Var* v = current_scope_->Peek(name);
    if (v->IsDefined())
      return v;
  }
  return name.PeekGlobalVar();
}

Var* Evaluator::LookupVarInCurrentScope(Symbol name) {
  if (current_scope_) {
    return current_scope_->Lookup(name);
  }
  return LookupVarGlobal(name);
}

Var* Evaluator::PeekVarInCurrentScope(Symbol name) {
  if (current_scope_) {
    return current_scope_->Peek(name);
  }
  return name.PeekGlobalVar();
}

string Evaluator::EvalVar(Symbol name) {
  return LookupVar(name)->Eval(this);
}

string Evaluator::GetShell() {
  return EvalVar(kShellSym);
}

string Evaluator::GetShellFlag() {
  // TODO: Handle $(.SHELLFLAGS)
  return is_posix_ ? "-ec" : "-c";
}

string Evaluator::GetShellAndFlag() {
  string shell = GetShell();
  shell += ' ';
  shell += GetShellFlag();
  return shell;
}

void Evaluator::Error(const string& msg) {
  for (auto& inc : include_stack_) {
    fprintf(stderr, "In file included from %s:%d:\n", LOCF(inc));
  }
  ERROR_LOC(loc_, "%s", msg.c_str());
}

void Evaluator::DumpStackStats() const {
  LOG_STAT("Max stack use: %zd bytes at %s:%d",
           ((char*)stack_addr_ - (char*)lowest_stack_) + stack_size_,
           LOCF(lowest_loc_));
}

void Evaluator::DumpIncludeJSON(const string& filename) const {
  IncludeGraph graph;
  graph.MergeTreeNode(*include_tree_);
  FILE* jsonfile;
  if (filename == "-") {
    jsonfile = stdout;
  } else {
    jsonfile = fopen(filename.c_str(), "w");
    if (jsonfile == NULL) {
      fprintf(stderr, "cannot open JSON dump file: %s\n", strerror(errno));
      return;
    }
  }

  graph.DumpJSON(jsonfile);
  fclose(jsonfile);
}

SymbolSet Evaluator::used_undefined_vars_;
