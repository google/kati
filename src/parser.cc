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

#include "parser.h"

#include <stack>
#include <string_view>
#include <unordered_map>

#include "expr.h"
#include "file.h"
#include "loc.h"
#include "log.h"
#include "stats.h"
#include "stmt.h"
#include "strutil.h"

enum struct ParserState {
  NOT_AFTER_RULE = 0,
  AFTER_RULE,
  MAYBE_AFTER_RULE,
};

class Parser {
  struct IfState {
    IfStmt* stmt;
    bool is_in_else;
    int num_nest;
  };

  typedef void (Parser::*DirectiveHandler)(std::string_view line,
                                           std::string_view directive);
  typedef std::unordered_map<std::string_view, DirectiveHandler> DirectiveMap;

 public:
  Parser(std::string_view buf, const char* filename, std::vector<Stmt*>* stmts)
      : buf_(buf),
        state_(ParserState::NOT_AFTER_RULE),
        stmts_(stmts),
        out_stmts_(stmts),
        num_define_nest_(0),
        num_if_nest_(0),
        loc_(filename, 0),
        fixed_lineno_(false) {}

  Parser(std::string_view buf, const Loc& loc, std::vector<Stmt*>* stmts)
      : buf_(buf),
        state_(ParserState::NOT_AFTER_RULE),
        stmts_(stmts),
        out_stmts_(stmts),
        num_if_nest_(0),
        loc_(loc),
        fixed_lineno_(true) {}

  void Parse() {
    l_ = 0;

    for (l_ = 0; l_ < buf_.size();) {
      size_t lf_cnt = 0;
      size_t e = FindEndOfLine(&lf_cnt);
      if (!fixed_lineno_)
        loc_.lineno++;
      std::string_view line(buf_.data() + l_, e - l_);
      if (!line.empty() && line.back() == '\r')
        line.remove_suffix(1);
      orig_line_with_directives_ = line;
      ParseLine(line);
      if (!fixed_lineno_)
        loc_.lineno += lf_cnt - 1;
      if (e == buf_.size())
        break;

      l_ = e + 1;
    }

    if (!if_stack_.empty())
      ERROR_LOC(Loc(loc_.filename, loc_.lineno + 1), "*** missing `endif'.");
    if (!define_name_.empty())
      ERROR_LOC(Loc(loc_.filename, define_start_line_),
                "*** missing `endef', unterminated `define'.");
  }

  void set_state(ParserState st) { state_ = st; }

  static std::vector<ParseErrorStmt*> parse_errors;

 private:
  void Error(const std::string& msg) {
    ParseErrorStmt* stmt = new ParseErrorStmt();
    stmt->set_loc(loc_);
    stmt->msg = msg;
    out_stmts_->push_back(stmt);
    parse_errors.push_back(stmt);
  }

  size_t FindEndOfLine(size_t* lf_cnt) {
    return ::FindEndOfLine(buf_, l_, lf_cnt);
  }

  Value* ParseExpr(Loc* loc,
                   std::string_view s,
                   ParseExprOpt opt = ParseExprOpt::NORMAL) {
    return ::ParseExpr(loc, s, opt);
  }

  void ParseLine(std::string_view line) {
    if (!define_name_.empty()) {
      ParseInsideDefine(line);
      return;
    }

    if (line.empty() || (line.size() == 1 && line[0] == '\r'))
      return;

    current_directive_ = AssignDirective::NONE;

    if (line[0] == '\t' && state_ != ParserState::NOT_AFTER_RULE) {
      CommandStmt* stmt = new CommandStmt();
      stmt->set_loc(loc_);
      Loc mutable_loc(loc_);
      stmt->expr =
          ParseExpr(&mutable_loc, line.substr(1), ParseExprOpt::COMMAND);
      stmt->orig = line;
      out_stmts_->push_back(stmt);
      return;
    }

    line = TrimLeftSpace(line);

    if (line[0] == '#')
      return;

    if (HandleDirective(line, make_directives_)) {
      return;
    }

    ParseRuleOrAssign(line);
  }

  void ParseRuleOrAssign(std::string_view line) {
    size_t sep = FindThreeOutsideParen(line, ':', '=', ';');
    if (sep == std::string::npos || line[sep] == ';') {
      ParseRule(line, std::string::npos);
    } else if (line[sep] == '=') {
      ParseAssign(line, sep);
    } else if (sep + 1 < line.size() && line[sep + 1] == '=') {
      ParseAssign(line, sep + 1);
    } else if (line[sep] == ':') {
      ParseRule(line, sep);
    } else {
      CHECK(false);
    }
  }

  void ParseRule(std::string_view line, size_t sep) {
    if (current_directive_ != AssignDirective::NONE) {
      if (IsInExport())
        return;
      if (sep != std::string::npos) {
        sep += orig_line_with_directives_.size() - line.size();
      }
      line = orig_line_with_directives_;
    }

    line = TrimLeftSpace(line);
    if (line.empty())
      return;

    if (orig_line_with_directives_[0] == '\t') {
      Error("*** commands commence before first target.");
      return;
    }

    const bool is_rule = sep != std::string::npos && line[sep] == ':';
    RuleStmt* rule_stmt = new RuleStmt();
    rule_stmt->set_loc(loc_);

    size_t found = FindTwoOutsideParen(line.substr(sep + 1), '=', ';');
    Loc mutable_loc(loc_);
    if (found != std::string::npos) {
      found += sep + 1;
      rule_stmt->lhs =
          ParseExpr(&mutable_loc, TrimSpace(line.substr(0, found)));
      if (line[found] == ';') {
        rule_stmt->sep = RuleStmt::SEP_SEMICOLON;
      } else if (line[found] == '=') {
        if (line.size() > (found + 2) && line[found + 1] == '$' &&
            line[found + 2] == '=') {
          rule_stmt->sep = RuleStmt::SEP_FINALEQ;
          found += 2;
        } else {
          rule_stmt->sep = RuleStmt::SEP_EQ;
        }
      }
      ParseExprOpt opt = rule_stmt->sep == RuleStmt::SEP_SEMICOLON
                             ? ParseExprOpt::COMMAND
                             : ParseExprOpt::NORMAL;
      rule_stmt->rhs =
          ParseExpr(&mutable_loc, TrimLeftSpace(line.substr(found + 1)), opt);
    } else {
      rule_stmt->lhs = ParseExpr(&mutable_loc, line);
      rule_stmt->sep = RuleStmt::SEP_NULL;
      rule_stmt->rhs = NULL;
    }
    out_stmts_->push_back(rule_stmt);
    state_ = is_rule ? ParserState::AFTER_RULE : ParserState::MAYBE_AFTER_RULE;
  }

  void ParseAssign(std::string_view line, size_t separator_pos) {
    if (separator_pos == 0) {
      Error("*** empty variable name ***");
      return;
    }
    std::string_view lhs;
    std::string_view rhs;
    AssignOp op;
    ParseAssignStatement(line, separator_pos, &lhs, &rhs, &op);

    // If rhs starts with '$=', this is 'final assignment',
    // e.g., a combination of the assignment and
    //  .KATI_READONLY := <lhs>
    // statement. Note that we assume that ParseAssignStatement
    // trimmed the left
    bool is_final = (rhs.size() >= 2 && rhs[0] == '$' && rhs[1] == '=');
    if (is_final) {
      rhs = TrimLeftSpace(rhs.substr(2));
    }

    AssignStmt* stmt = new AssignStmt();
    Loc mutable_loc(loc_);
    stmt->set_loc(loc_);
    stmt->lhs = ParseExpr(&mutable_loc, lhs);
    stmt->rhs = ParseExpr(&mutable_loc, rhs);
    stmt->orig_rhs = rhs;
    stmt->op = op;
    stmt->directive = current_directive_;
    stmt->is_final = is_final;
    out_stmts_->push_back(stmt);
    state_ = ParserState::NOT_AFTER_RULE;
  }

  void ParseInclude(std::string_view line, std::string_view directive) {
    IncludeStmt* stmt = new IncludeStmt();
    stmt->set_loc(loc_);
    Loc mutable_loc(loc_);
    stmt->expr = ParseExpr(&mutable_loc, line);
    stmt->should_exist = directive[0] == 'i';
    out_stmts_->push_back(stmt);
    state_ = ParserState::NOT_AFTER_RULE;
  }

  void ParseDefine(std::string_view line, std::string_view) {
    if (line.empty()) {
      Error("*** empty variable name.");
      return;
    }
    define_name_ = line;
    num_define_nest_ = 1;
    define_start_ = 0;
    define_start_line_ = loc_.lineno;
    state_ = ParserState::NOT_AFTER_RULE;
  }

  void ParseInsideDefine(std::string_view line) {
    line = TrimLeftSpace(line);
    std::string_view directive = GetDirective(line);
    if (directive == "define")
      num_define_nest_++;
    else if (directive == "endef")
      num_define_nest_--;
    if (num_define_nest_ > 0) {
      if (define_start_ == 0)
        define_start_ = l_;
      return;
    }

    std::string_view rest = TrimRightSpace(
        RemoveComment(TrimLeftSpace(line.substr(sizeof("endef") - 1))));
    if (!rest.empty()) {
      WARN_LOC(loc_, "extraneous text after `endef' directive");
    }

    AssignStmt* stmt = new AssignStmt();
    stmt->set_loc(Loc(loc_.filename, define_start_line_));
    Loc mutable_loc(stmt->loc());
    stmt->lhs = ParseExpr(&mutable_loc, define_name_);
    mutable_loc.lineno++;
    std::string_view rhs;
    if (define_start_)
      rhs = buf_.substr(define_start_, l_ - define_start_ - 1);
    stmt->rhs = ParseExpr(&mutable_loc, rhs, ParseExprOpt::DEFINE);
    stmt->orig_rhs = rhs;
    stmt->op = AssignOp::EQ;
    stmt->directive = current_directive_;
    out_stmts_->push_back(stmt);
    define_name_ = std::string_view();
  }

  void EnterIf(IfStmt* stmt) {
    IfState* st = new IfState();
    st->stmt = stmt;
    st->is_in_else = false;
    st->num_nest = num_if_nest_;
    if_stack_.push(st);
    out_stmts_ = &stmt->true_stmts;
  }

  void ParseIfdef(std::string_view line, std::string_view directive) {
    IfStmt* stmt = new IfStmt();
    stmt->set_loc(loc_);
    stmt->op = directive[2] == 'n' ? CondOp::IFNDEF : CondOp::IFDEF;
    Loc mutable_loc(loc_);
    stmt->lhs = ParseExpr(&mutable_loc, line);
    stmt->rhs = NULL;
    out_stmts_->push_back(stmt);
    EnterIf(stmt);
  }

  bool ParseIfEqCond(std::string_view s, IfStmt* stmt) {
    if (s.empty()) {
      return false;
    }

    Loc mutable_loc(loc_);
    if (s[0] == '(' && s[s.size() - 1] == ')') {
      s = s.substr(1, s.size() - 2);
      char terms[] = {',', '\0'};
      size_t n;
      stmt->lhs =
          ParseExprImpl(&mutable_loc, s, terms, ParseExprOpt::NORMAL, &n, true);
      if (s[n] != ',')
        return false;
      s = TrimLeftSpace(s.substr(n + 1));
      stmt->rhs =
          ParseExprImpl(&mutable_loc, s, NULL, ParseExprOpt::NORMAL, &n);
      s = TrimLeftSpace(s.substr(std::min(n, s.size())));
    } else {
      for (int i = 0; i < 2; i++) {
        if (s.empty())
          return false;
        char quote = s[0];
        if (quote != '\'' && quote != '"')
          return false;
        size_t end = s.find(quote, 1);
        if (end == std::string::npos)
          return false;
        Value* v =
            ParseExpr(&mutable_loc, s.substr(1, end - 1), ParseExprOpt::NORMAL);
        if (i == 0)
          stmt->lhs = v;
        else
          stmt->rhs = v;
        s = TrimLeftSpace(s.substr(end + 1));
      }
    }
    if (!s.empty()) {
      WARN_LOC(loc_, "extraneous text after `ifeq' directive");
      return true;
    }
    return true;
  }

  void ParseIfeq(std::string_view line, std::string_view directive) {
    IfStmt* stmt = new IfStmt();
    stmt->set_loc(loc_);
    stmt->op = directive[2] == 'n' ? CondOp::IFNEQ : CondOp::IFEQ;

    if (!ParseIfEqCond(line, stmt)) {
      Error("*** invalid syntax in conditional.");
      return;
    }

    out_stmts_->push_back(stmt);
    EnterIf(stmt);
  }

  void ParseElse(std::string_view line, std::string_view) {
    if (!CheckIfStack("else"))
      return;
    IfState* st = if_stack_.top();
    if (st->is_in_else) {
      Error("*** only one `else' per conditional.");
      return;
    }
    st->is_in_else = true;
    out_stmts_ = &st->stmt->false_stmts;

    std::string_view next_if = TrimLeftSpace(line);
    if (next_if.empty())
      return;

    num_if_nest_ = st->num_nest + 1;
    if (!HandleDirective(next_if, else_if_directives_)) {
      WARN_LOC(loc_, "extraneous text after `else' directive");
    }
    num_if_nest_ = 0;
  }

  void ParseEndif(std::string_view line, std::string_view) {
    if (!CheckIfStack("endif"))
      return;
    if (!line.empty()) {
      Error("extraneous text after `endif` directive");
      return;
    }
    IfState st = *if_stack_.top();
    for (int t = 0; t <= st.num_nest; t++) {
      delete if_stack_.top();
      if_stack_.pop();
      if (if_stack_.empty()) {
        out_stmts_ = stmts_;
      } else {
        IfState* st = if_stack_.top();
        if (st->is_in_else)
          out_stmts_ = &st->stmt->false_stmts;
        else
          out_stmts_ = &st->stmt->true_stmts;
      }
    }
  }

  bool IsInExport() const {
    return (static_cast<int>(current_directive_) &
            static_cast<int>(AssignDirective::EXPORT));
  }

  void CreateExport(std::string_view line, bool is_export) {
    ExportStmt* stmt = new ExportStmt;
    stmt->set_loc(loc_);
    Loc mutable_loc(loc_);
    stmt->expr = ParseExpr(&mutable_loc, line);
    stmt->is_export = is_export;
    out_stmts_->push_back(stmt);
  }

  void ParseOverride(std::string_view line, std::string_view) {
    current_directive_ = static_cast<AssignDirective>(
        (static_cast<int>(current_directive_) |
         static_cast<int>(AssignDirective::OVERRIDE)));
    if (HandleDirective(line, assign_directives_))
      return;
    if (IsInExport()) {
      CreateExport(line, true);
    }
    ParseRuleOrAssign(line);
  }

  void ParseExport(std::string_view line, std::string_view) {
    current_directive_ = static_cast<AssignDirective>(
        (static_cast<int>(current_directive_) |
         static_cast<int>(AssignDirective::EXPORT)));
    if (HandleDirective(line, assign_directives_))
      return;
    CreateExport(line, true);
    ParseRuleOrAssign(line);
  }

  void ParseUnexport(std::string_view line, std::string_view) {
    CreateExport(line, false);
  }
  bool CheckIfStack(const char* keyword) {
    if (if_stack_.empty()) {
      Error(StringPrintf("*** extraneous `%s'.", keyword));
      return false;
    }
    return true;
  }

  std::string_view RemoveComment(std::string_view line) {
    size_t i = FindOutsideParen(line, '#');
    if (i == std::string::npos)
      return line;
    return line.substr(0, i);
  }

  std::string_view GetDirective(std::string_view line) {
    if (line.size() < shortest_directive_len_)
      return std::string_view();
    std::string_view prefix = line.substr(0, longest_directive_len_ + 1);
    size_t space_index = prefix.find_first_of(" \t#");
    return prefix.substr(0, space_index);
  }

  bool HandleDirective(std::string_view line,
                       const DirectiveMap& directive_map) {
    std::string_view directive = GetDirective(line);
    auto found = directive_map.find(directive);
    if (found == directive_map.end())
      return false;

    std::string_view rest = TrimRightSpace(
        RemoveComment(TrimLeftSpace(line.substr(directive.size()))));
    (this->*found->second)(rest, directive);
    return true;
  }

  std::string_view buf_;
  size_t l_;
  ParserState state_;

  std::vector<Stmt*>* stmts_;
  std::vector<Stmt*>* out_stmts_;

  std::string_view define_name_;
  int num_define_nest_;
  size_t define_start_;
  int define_start_line_;

  std::string_view orig_line_with_directives_;
  AssignDirective current_directive_;

  int num_if_nest_;
  std::stack<IfState*> if_stack_;

  Loc loc_;
  bool fixed_lineno_;

  const static DirectiveMap make_directives_;
  const static DirectiveMap else_if_directives_;
  const static DirectiveMap assign_directives_;
  const static size_t shortest_directive_len_;
  const static size_t longest_directive_len_;
};

void Parse(Makefile* mk) {
  COLLECT_STATS("parse file time");
  Parser parser(std::string_view(mk->buf()), mk->filename().c_str(),
                mk->mutable_stmts());
  parser.Parse();
}

void Parse(std::string_view buf,
           const Loc& loc,
           std::vector<Stmt*>* out_stmts) {
  COLLECT_STATS("parse eval time");
  Parser parser(buf, loc, out_stmts);
  parser.Parse();
}

void ParseNotAfterRule(std::string_view buf,
                       const Loc& loc,
                       std::vector<Stmt*>* out_stmts) {
  Parser parser(buf, loc, out_stmts);
  parser.set_state(ParserState::NOT_AFTER_RULE);
  parser.Parse();
}

const Parser::DirectiveMap Parser::make_directives_ = {
    {"include", &Parser::ParseInclude},   {"-include", &Parser::ParseInclude},
    {"sinclude", &Parser::ParseInclude},  {"define", &Parser::ParseDefine},
    {"ifdef", &Parser::ParseIfdef},       {"ifndef", &Parser::ParseIfdef},
    {"ifeq", &Parser::ParseIfeq},         {"ifneq", &Parser::ParseIfeq},
    {"else", &Parser::ParseElse},         {"endif", &Parser::ParseEndif},
    {"override", &Parser::ParseOverride}, {"export", &Parser::ParseExport},
    {"unexport", &Parser::ParseUnexport}};

const Parser::DirectiveMap Parser::else_if_directives_ = {
    {"ifdef", &Parser::ParseIfdef},
    {"ifndef", &Parser::ParseIfdef},
    {"ifeq", &Parser::ParseIfeq},
    {"ifneq", &Parser::ParseIfeq},
};

const Parser::DirectiveMap Parser::assign_directives_ = {
    {"define", &Parser::ParseDefine},
    {"export", &Parser::ParseExport},
    {"override", &Parser::ParseOverride},
};

const size_t Parser::shortest_directive_len_ = []() {
  size_t result = 9999;
  for (auto p : Parser::make_directives_) {
    size_t len = p.first.size();
    result = std::min(len, result);
  }
  return result;
}();

const size_t Parser::longest_directive_len_ = []() {
  size_t result = 0;
  for (auto p : Parser::make_directives_) {
    size_t len = p.first.size();
    result = std::max(len, result);
  }
  return result;
}();

std::vector<ParseErrorStmt*> Parser::parse_errors;

void ParseAssignStatement(std::string_view line,
                          size_t sep,
                          std::string_view* lhs,
                          std::string_view* rhs,
                          AssignOp* op) {
  CHECK(sep != 0);
  *op = AssignOp::EQ;
  size_t lhs_end = sep;
  switch (line[sep - 1]) {
    case ':':
      lhs_end--;
      *op = AssignOp::COLON_EQ;
      break;
    case '+':
      lhs_end--;
      *op = AssignOp::PLUS_EQ;
      break;
    case '?':
      lhs_end--;
      *op = AssignOp::QUESTION_EQ;
      break;
  }
  *lhs = TrimSpace(line.substr(0, lhs_end));
  *rhs = TrimLeftSpace(line.substr(std::min(sep + 1, line.size())));
}

const std::vector<ParseErrorStmt*>& GetParseErrors() {
  return Parser::parse_errors;
}
