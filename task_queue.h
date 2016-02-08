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

#ifndef TASK_QUEUE_H_
#define TASK_QUEUE_H_

#include <queue>

#include "condvar.h"
#include "mutex.h"

using namespace std;

class TaskQueueImpl {
 private:
  template <class T> friend class TaskQueue;

  TaskQueueImpl();
  ~TaskQueueImpl();

  void Push(void* v);
  void* Pop();
  void Finish();

  queue<void*> q_;
  mutex mu_;
  condition_variable cond_;
  bool should_finish_;
};

template <class T>
class TaskQueue {
 public:
  ~TaskQueue() {}

  void Push(T* v) {
    impl_.Push((void*)v);
  }

  T* Pop() {
    return (T*)impl_.Pop();
  }

  void Finish() {
    impl_.Finish();
  }

 private:
  TaskQueueImpl impl_;
};

#endif  // TASK_QUEUE_H_
