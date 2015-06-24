// Copyright 2015 Google Inc. All rights reserved
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

// +build ignore

#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#include <queue>
#include <set>
#include <string>
#include <vector>

#define CHECK(expr) do {                                        \
    if (!(expr)) {                                              \
      fprintf(stderr, "%s:%d: " #expr, __FILE__, __LINE__);     \
      exit(1);                                                  \
    }                                                           \
  } while(0)
#define PCHECK(expr) do {                               \
    if ((expr) < 0) {                                   \
      fprintf(stderr, "%s:%d: ", __FILE__, __LINE__);   \
      perror("");                                       \
      exit(1);                                          \
    }                                                   \
  } while(0)

using namespace std;

class Para;

struct Task {
  Task() : echo(false), ignore_error(false), status(-1), signal(-1) {
    stdout_pipe[0] = -1;
    stdout_pipe[1] = -1;
    stderr_pipe[0] = -1;
    stderr_pipe[1] = -1;
  }
  ~Task() {
    if (stdout_pipe[0] >= 0)
      PCHECK(close(stdout_pipe[0]));
    if (stderr_pipe[0] >= 0)
      PCHECK(close(stderr_pipe[0]));
  }

  string output;
  string shell;
  string cmd;
  bool echo;
  bool ignore_error;

  pid_t pid;
  int stdout_pipe[2];
  int stderr_pipe[2];
  string stdout_buf;
  string stderr_buf;
  int status;
  int signal;
};

class TaskProvider {
 public:
  virtual ~TaskProvider() {}
  virtual int GetFD() = 0;
  virtual void PollFD(Para* para, int fd) = 0;
  virtual void OnStarted(Task* t) = 0;
  virtual void OnFinished(Task* t) = 0;
};

class Para {
 public:
  Para(TaskProvider* provider, int num_jobs);
  ~Para();

  void AddTask(Task* task);

  void Loop();

  void Done();

  static void WakeUp(int);

 private:
  void WaitChildren();
  void RunCommands();

  TaskProvider* provider_;
  size_t num_jobs_;
  int sig_pipe_[2];
  int provider_fd_;
  queue<Task*> tasks_;
  set<Task*> running_;
  bool done_;
};

class StdinTaskProvider : public TaskProvider {
 public:
  virtual ~StdinTaskProvider() {}
  virtual int GetFD() { return STDIN_FILENO; }
  virtual void PollFD(Para* para, int fd);
  virtual void OnStarted(Task* t);
  virtual void OnFinished(Task* t);

 private:
  string buf_;
};

class KatiTaskProvider : public TaskProvider {
 public:
  virtual ~KatiTaskProvider() {}
  virtual int GetFD() { return STDIN_FILENO; }
  virtual void PollFD(Para* para, int fd);
  virtual void OnStarted(Task* t);
  virtual void OnFinished(Task* t);

 private:
  string buf_;
};

Para* g_para_;

Para::Para(TaskProvider* provider, int num_jobs)
    : provider_(provider),
      num_jobs_(num_jobs),
      done_(false) {
  g_para_ = this;
  PCHECK(pipe(sig_pipe_));
  sigset_t sigmask;
  sigemptyset(&sigmask);
  sigaddset(&sigmask, SIGCHLD);
  PCHECK(sigprocmask(SIG_BLOCK, &sigmask, NULL));
  CHECK(signal(SIGCHLD, &Para::WakeUp) != SIG_ERR);
  provider_fd_ = provider_->GetFD();
}

Para::~Para() {
  PCHECK(close(sig_pipe_[0]));
  PCHECK(close(sig_pipe_[1]));
}

void Para::AddTask(Task* task) {
  task->pid = 0;
  tasks_.push(task);
}

static void SetFd(int fd, fd_set* fdset, int* nfds) {
  if (fd < 0)
    return;
  FD_SET(fd, fdset);
  *nfds = max(*nfds, fd);
}

static void readOutput(int* fd, string* buf) {
  char b[4096];
  ssize_t r = read(*fd, b, sizeof(b));
  if (r < 0 && errno != EINTR)
    return;
  PCHECK(r);
  if (r == 0) {
    PCHECK(close(*fd));
    *fd = -1;
    return;
  }

  size_t l = buf->size();
  buf->resize(l + r);
  memcpy(&((*buf)[l]), b, r);
}

void Para::Loop() {
  sigset_t sigmask;
  sigemptyset(&sigmask);

  while (!done_ || !tasks_.empty() || !running_.empty()) {
    int nfds = 0;
    fd_set rd;
    FD_ZERO(&rd);
    if (!done_)
      SetFd(provider_fd_, &rd, &nfds);
    SetFd(sig_pipe_[0], &rd, &nfds);
    for (Task* t : running_) {
      SetFd(t->stdout_pipe[0], &rd, &nfds);
      SetFd(t->stderr_pipe[0], &rd, &nfds);
    }
    int r = pselect(nfds, &rd, NULL, NULL, NULL, &sigmask);
    PCHECK(r && errno != EINTR);

    if (FD_ISSET(sig_pipe_[0], &rd)) {
      WaitChildren();
      RunCommands();
      continue;
    }
    if (FD_ISSET(provider_fd_, &rd)) {
      provider_->PollFD(this, provider_fd_);
      RunCommands();
      continue;
    }
    for (Task* t : running_) {
      if (t->stdout_pipe[0] >= 0 && FD_ISSET(t->stdout_pipe[0], &rd))
        readOutput(&t->stdout_pipe[0], &t->stdout_buf);
      if (t->stderr_pipe[0] >= 0 && FD_ISSET(t->stderr_pipe[0], &rd))
        readOutput(&t->stderr_pipe[0], &t->stderr_buf);
    }
  }
}

void Para::Done() {
  done_ = true;
}

void Para::WakeUp(int) {
  char c = 42;
  PCHECK(write(g_para_->sig_pipe_[1], &c, 1));
}

void Para::WaitChildren() {
  char c = 0;
  PCHECK(read(sig_pipe_[0], &c, 1));
  CHECK(c == 42);

  vector<Task*> finished;
  for (Task* task : running_) {
    int status;
    pid_t pid = waitpid(task->pid, &status, WNOHANG);
    if (WIFSIGNALED(status)) {
      task->signal = WTERMSIG(status);
    } else if (WIFEXITED(status)) {
      task->status = WEXITSTATUS(status);
    } else {
      PCHECK(false);
    }
    PCHECK(pid);
    if (pid == 0) {
      continue;
    }
    CHECK(pid == task->pid);

    while (task->stdout_pipe[0] >= 0)
      readOutput(&task->stdout_pipe[0], &task->stdout_buf);
    while (task->stderr_pipe[0] >= 0)
      readOutput(&task->stderr_pipe[0], &task->stderr_buf);

    // TODO: Handle error.
    finished.push_back(task);
  }

  for (Task* task : finished) {
    running_.erase(task);
    provider_->OnFinished(task);
    delete task;
  }
}

void Para::RunCommands() {
  while (!tasks_.empty() && (running_.size() < num_jobs_ || num_jobs_ == 0)) {
    Task* task = tasks_.front();
    tasks_.pop();
    provider_->OnStarted(task);

    PCHECK(pipe(task->stdout_pipe));
    PCHECK(pipe(task->stderr_pipe));
    task->pid = fork();
    if (task->pid == 0) {
      PCHECK(close(task->stdout_pipe[0]));
      PCHECK(close(task->stderr_pipe[0]));
      PCHECK(dup2(task->stdout_pipe[1], STDOUT_FILENO));
      PCHECK(dup2(task->stderr_pipe[1], STDERR_FILENO));
      PCHECK(close(task->stdout_pipe[1]));
      PCHECK(close(task->stderr_pipe[1]));

      const char* args[] = {
        task->shell.c_str(),
        "-c",
        task->cmd.c_str(),
        NULL
      };
      PCHECK(execvp(args[0], const_cast<char* const*>(args)));
      abort();
    }

    PCHECK(close(task->stdout_pipe[1]));
    PCHECK(close(task->stderr_pipe[1]));
    running_.insert(task);
  }
}

void StdinTaskProvider::PollFD(Para* para, int fd) {
  char buf[4096];
  ssize_t r = read(fd, buf, sizeof(buf));
  PCHECK(r);
  if (r == 0) {
    para->Done();
    return;
  }

  buf_.append(buf, r);
  for (;;) {
    size_t index = buf_.find('\n');
    if (index == string::npos) {
      break;
    }

    Task* task = new Task();
    task->shell = "/bin/sh";
    task->cmd = buf_.substr(0, index);
    if (task->cmd.empty())
      continue;
    para->AddTask(task);
    buf_ = buf_.substr(index + 1);
  }
}

void StdinTaskProvider::OnStarted(Task*) {
}

void StdinTaskProvider::OnFinished(Task* t) {
  fprintf(stdout, "%s", t->stdout_buf.c_str());
  fprintf(stderr, "%s", t->stderr_buf.c_str());
}

struct Runner {
  string output;
  string cmd;
  string shell;
  bool echo;
  bool ignore_error;
};

static void recvData(int fd, void* d, size_t sz) {
  size_t s = 0;
  while (s < sz) {
    ssize_t r = read(fd, reinterpret_cast<char*>(d) + s, sz - s);
    if (r < 0 && errno == EINTR)
      continue;
    PCHECK(r);
    if (r == 0) {
      exit(1);
    }
    s += r;
  }
}

static int recvInt(int fd) {
  int v;
  recvData(fd, &v, sizeof(v));
  return v;
}

static void recvString(int fd, string* s) {
  int l = recvInt(fd);
  s->resize(l);
  recvData(fd, &((*s)[0]), l);
}

static void recvTasks(int fd, vector<Task*>* tasks) {
  int l = recvInt(fd);
  for (int i = 0; i < l; i++) {
    Task* r = new Task();
    recvString(fd, &r->output);
    recvString(fd, &r->cmd);
    recvString(fd, &r->shell);
    r->echo = recvInt(fd);
    r->ignore_error = recvInt(fd);
    tasks->push_back(r);
  }
}

void KatiTaskProvider::PollFD(Para* para, int fd) {
  vector<Task*> tasks;
  recvTasks(fd, &tasks);
#if 0
  for (Task* t : tasks) {
    para->AddTask(t);
  }
#else
  Task* task = tasks[0];
  for (Task* t : tasks) {
    if (task == t)
      continue;
    task->cmd += " ; ";
    task->cmd += t->cmd;
  }
  para->AddTask(task);
#endif
}

static void sendData(int fd, const void* d, size_t sz) {
  size_t s = 0;
  while (s < sz) {
    ssize_t r = write(fd, reinterpret_cast<const char*>(d) + s, sz - s);
    if (r < 0 && errno == EINTR)
      continue;
    PCHECK(r);
    if (r == 0) {
      exit(1);
    }
    s += r;
  }
}

static void sendInt(int fd, int v) {
  sendData(fd, &v, sizeof(v));
}

static void sendString(int fd, const string& s) {
  sendInt(fd, s.size());
  sendData(fd, s.data(), s.size());
}

static void sendResult(int fd, Task* t) {
  sendString(fd, t->output);
  sendString(fd, t->stdout_buf);
  sendString(fd, t->stderr_buf);
  sendInt(fd, t->status);
  sendInt(fd, t->signal);
}

void KatiTaskProvider::OnStarted(Task* t) {
  sendResult(STDOUT_FILENO, t);
}

void KatiTaskProvider::OnFinished(Task* t) {
  sendResult(STDOUT_FILENO, t);
}

int GetNumCpus() {
  // TODO: Implement.
  return 4;
}

int main(int argc, char* argv[]) {
  int num_jobs = -1;
  bool from_kati = false;
  for (int i = 1; i < argc; i++) {
    char* arg = argv[i];
    if (!strncmp(arg, "-j", 2)) {
      num_jobs = atoi(arg + 2);
    } else if (!strcmp(arg, "--kati")) {
      from_kati = true;
    }
  }
  if (num_jobs < 0) {
    num_jobs = GetNumCpus();
  }

  TaskProvider* provider = NULL;
  if (from_kati) {
    provider = new KatiTaskProvider();
  } else {
    provider = new StdinTaskProvider();
  }
  Para para(provider, num_jobs);
  para.Loop();
}
