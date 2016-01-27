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

#ifndef MUTEX_H_
#define MUTEX_H_

#include <pthread.h>

class mutex {
 public:
  explicit mutex();
  ~mutex();

  void lock();
  void unlock();

 private:
  pthread_mutex_t mu_;

  friend class condition_variable;
};

template<class T> class unique_lock {
 public:
  explicit unique_lock(T& mu)
      : mu_(mu) {
    mu_.lock();
  }
  ~unique_lock() {
    mu_.unlock();
  }

  T* mutex() const { return &mu_; }

 private:
  T& mu_;
};

#endif  // MUTEX_H_
