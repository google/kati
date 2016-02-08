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

#include "task_queue.h"

#include "log.h"

TaskQueueImpl::TaskQueueImpl()
    : should_finish_(false) {
}

TaskQueueImpl::~TaskQueueImpl() {
  CHECK(q_.empty());
  CHECK(should_finish_);
}

void TaskQueueImpl::Push(void* v) {
  unique_lock<mutex> lock(mu_);
  bool was_empty = q_.empty();
  q_.push(v);
  if (was_empty)
    cond_.notify_one();
}

void* TaskQueueImpl::Pop() {
  unique_lock<mutex> lock(mu_);
  while (true) {
    if (q_.empty()) {
      if (should_finish_)
        return nullptr;
      cond_.wait(lock);
      if (q_.empty())
        continue;
    }

    void* r = q_.front();
    q_.pop();
    return r;
  }
}

void TaskQueueImpl::Finish() {
  unique_lock<mutex> lock(mu_);
  should_finish_ = true;
  cond_.notify_one();
}
