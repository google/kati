// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// A simple cross platform thread local storage implementation.
//
// This is a drop-in replacement of __thread keyword. If your compiler
// toolchain supports __thread keyword, the user of this code should
// be as fast as the code which uses __thread. Chrome's
// base::ThreadLocalPointer and base::ThreadLocalStorage cannot be as
// fast as __thread.
// TODO(crbug.com/249345): If pthread_getspecific is slow for our use,
// expose bionic's internal TLS and stop using pthread_getspecific
// based implementation.
//
// Usage:
//
// Before (linux):
//
// __thread Foo* foo;
// foo = new Foo();
// foo->func();
//
//
// After:
//
// DEFINE_THREAD_LOCAL(Foo*, foo);
// foo.Ref() = new Foo();
// foo.Ref()->func();
//
// Thread local PODs are zero-initialized.
// Thread local non-PODs are initialized with the default constructor.

#ifndef THREAD_LOCAL_H_
#define THREAD_LOCAL_H_

#include <errno.h>
#include <pthread.h>

#include "log.h"

#ifdef __linux__

#define DEFINE_THREAD_LOCAL(Type, name) thread_local Type name
#define TLS_REF(x) x

#else

// Thread local storage implementation which uses pthread.
// Note that DEFINE_THREAD_LOCAL creates a global variable just like
// thread local storage based on __thread keyword. So we should not use
// constructor in ThreadLocal class to avoid static initializator.
template <typename Type>
void ThreadLocalDestructor(void* ptr) {
  delete reinterpret_cast<Type>(ptr);
}

template<typename Type, pthread_key_t* key>
void ThreadLocalInit() {
  if (pthread_key_create(key, ThreadLocalDestructor<Type>))
    ERROR("Failed to create a pthread key for TLS errno=%d", errno);
}

template<typename Type, pthread_key_t* key, pthread_once_t* once>
class ThreadLocal {
 public:
  Type& Ref() {
    return *GetPointer();
  }
  Type Get() {
    return Ref();
  }
  void Set(const Type& value) {
    Ref() = value;
  }
  Type* GetPointer() {
    pthread_once(once, ThreadLocalInit<Type*, key>);
    Type* value = reinterpret_cast<Type*>(pthread_getspecific(*key));
    if (value) return value;
    // new Type() for PODs means zero initialization.
    value = new Type();
    int error = pthread_setspecific(*key, value);
    if (error != 0)
      ERROR("Failed to set a TLS: error=%d", error);
    return value;
  }
};

// We need a namespace for name##_key and name##_once since template parameters
// do not accept unnamed values such as static global variables.
#define DEFINE_THREAD_LOCAL(Type, name)                 \
  namespace {                                           \
  pthread_once_t name##_once = PTHREAD_ONCE_INIT;       \
  pthread_key_t name##_key;                             \
  }                                                     \
  ThreadLocal<Type, &name##_key, &name##_once> name;

#define TLS_REF(x) x.Ref()

#endif

#endif  // THREAD_LOCAL_H_
