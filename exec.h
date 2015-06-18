#ifndef EXEC_H_
#define EXEC_H_

#include <vector>

using namespace std;

class DepNode;
class Vars;

void Exec(const vector<DepNode*>& roots, Vars* vars);

#endif  // EXEC_H_
