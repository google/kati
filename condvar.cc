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

#include "condvar.h"

#include "log.h"

condition_variable::condition_variable() {
  if (pthread_cond_init(&cond_, NULL) != 0)
    PERROR("pthread_cond_init");
}

condition_variable::~condition_variable() {
  if (pthread_cond_destroy(&cond_) != 0)
    PERROR("pthread_cond_destroy");
}

void condition_variable::wait(const unique_lock<mutex>& mu) {
  if (pthread_cond_wait(&cond_, &mu.mutex()->mu_) != 0)
    PERROR("pthread_cond_wait");
}

void condition_variable::notify_one() {
  if (pthread_cond_signal(&cond_) != 0)
    PERROR("pthread_cond_signal");
}

void condition_variable::notify_all() {
  if (pthread_cond_broadcast(&cond_) != 0)
    PERROR("pthread_cond_broadcast");
}
