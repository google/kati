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

#include "thread_pool.h"

#include <condition_variable>
#include <mutex>
#include <stack>
#include <thread>
#include <vector>

#include "affinity.h"

class ThreadPoolImpl : public ThreadPool {
 public:
  explicit ThreadPoolImpl(int num_threads)
      : is_waiting_(false) {
    SetAffinityForMultiThread();
    threads_.reserve(num_threads);
    for (int i = 0; i < num_threads; i++) {
      threads_.push_back(thread([this]() { Loop(); }));
    }
  }

  virtual ~ThreadPoolImpl() override {
  }

  virtual void Submit(function<void(void)> task) override {
    unique_lock<mutex> lock(mu_);
    tasks_.push(task);
    cond_.notify_one();
  }

  virtual void Wait() override {
    {
      unique_lock<mutex> lock(mu_);
      is_waiting_ = true;
      cond_.notify_all();
    }

    for (thread& th : threads_) {
      th.join();
    }

    SetAffinityForSingleThread();
  }

 private:
  void Loop() {
    while (true) {
      function<void(void)> task;
      {
        unique_lock<mutex> lock(mu_);
        if (tasks_.empty()) {
          if (is_waiting_)
            return;
          cond_.wait(lock);
        }

        if (tasks_.empty())
          continue;

        task = tasks_.top();
        tasks_.pop();
      }
      task();
    }
  }

  vector<thread> threads_;
  mutex mu_;
  condition_variable cond_;
  stack<function<void(void)>> tasks_;
  bool is_waiting_;
};

ThreadPool* NewThreadPool(int num_threads) {
  return new ThreadPoolImpl(num_threads);
}
