#ifndef VALUE_H_
#define VALUE_H_

#include <memory>
#include <string>
#include <vector>

#include "string_piece.h"

using namespace std;

class Evaluator;

class Evaluable {
 public:
  virtual void Eval(Evaluator* ev, string* s) const = 0;
  virtual shared_ptr<string> Eval(Evaluator*) const;

 protected:
  Evaluable();
  virtual ~Evaluable();
};

class Value : public Evaluable {
 public:
  virtual ~Value();

  virtual Value* Compact() { return this; }

  string DebugString() const;

 protected:
  Value();
  virtual string DebugString_() const = 0;
};

Value* ParseExpr(StringPiece s, bool is_command);

string JoinValues(const vector<Value*> vals, const char* sep);

#endif  // VALUE_H_
