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

#include <vector>

#include "loc.h"
#include "stmt.h"
#include "string_piece.h"

using namespace std;

class Makefile;

void Parse(Makefile* mk);
void Parse(StringPiece buf, const Loc& loc, vector<Stmt*>* out_asts);
void ParseNotAfterRule(StringPiece buf, const Loc& loc,
                       vector<Stmt*>* out_asts);

void ParseAssignStatement(StringPiece line, size_t sep,
                          StringPiece* lhs, StringPiece* rhs, AssignOp* op);

void InitParser();
void QuitParser();

const vector<ParseErrorStmt*>& GetParseErrors();

#endif  // PARSER_H_
