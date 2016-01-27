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

#ifndef THREAD_POOL_H_
#define THREAD_POOL_H_

#include <functional>

using namespace std;

class ThreadPool {
 public:
  virtual ~ThreadPool() = default;

  virtual void Submit(function<void(void)> task) = 0;
  virtual void Wait() = 0;

 protected:
  ThreadPool() = default;
};

ThreadPool* NewThreadPool(int num_threads);

#endif  // THREAD_POOL_H_
