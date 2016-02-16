// Copyright 2016 Google Inc. All rights reserved
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

#ifndef CONDVAR_H_
#define CONDVAR_H_

#include <pthread.h>

#include "mutex.h"

class condition_variable {
 public:
  condition_variable();
  ~condition_variable();

  void wait(const UniqueLock<Mutex>& mu);
  void notify_one();
  void notify_all();

 private:
  pthread_cond_t cond_;
};

#endif  // CONDVAR_H_
