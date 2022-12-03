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

#ifndef PARSER_H_
#define PARSER_H_

#include <string_view>
#include <vector>

#include "loc.h"
#include "stmt.h"

class Makefile;

void Parse(Makefile* mk);
void Parse(std::string_view buf, const Loc& loc, std::vector<Stmt*>* out_asts);
void ParseNotAfterRule(std::string_view buf,
                       const Loc& loc,
                       std::vector<Stmt*>* out_asts);

void ParseAssignStatement(std::string_view line,
                          size_t sep,
                          std::string_view* lhs,
                          std::string_view* rhs,
                          AssignOp* op);

const std::vector<ParseErrorStmt*>& GetParseErrors();

#endif  // PARSER_H_
