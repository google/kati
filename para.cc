#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
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
  string shell;
  string cmd;
  pid_t pid;
};

class TaskProvider {
 public:
  virtual ~TaskProvider() {}
  virtual int GetFD() = 0;
  virtual void PollFD(Para* para) = 0;
};

class Para {
 public:
  Para(TaskProvider* provider, int num_jobs);

  void AddTask(const Task& task);

  void Loop();

  void Done();

  static void WakeUp(int);

 private:
  void WaitChildren();
  void RunCommands();

  TaskProvider* provider_;
  int num_jobs_;
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
  virtual void PollFD(Para* para);

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
  PCHECK(signal(SIGCHLD, &Para::WakeUp));
  provider_fd_ = provider_->GetFD();
}

void Para::AddTask(const Task& task) {
  Task* t = new Task(task);
  t->pid = 0;
  tasks_.push(t);
}

static void SetFd(int fd, fd_set* fdset, int* nfds) {
  FD_SET(fd, fdset);
  *nfds = max(*nfds, fd);
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
    int r = pselect(nfds, &rd, NULL, NULL, NULL, &sigmask);
    PCHECK(r && errno != EINTR);

    if (FD_ISSET(provider_fd_, &rd)) {
      provider_->PollFD(this);
    }
    if (FD_ISSET(sig_pipe_[0], &rd)) {
      WaitChildren();
    }

    RunCommands();
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
  vector<Task*> finished;
  for (Task* task : running_) {
    int status;
    pid_t pid = waitpid(task->pid, &status, WNOHANG);
    PCHECK(pid);
    if (pid == 0) {
      continue;
    }
    CHECK(pid == task->pid);
    // TODO: Handle error.
    finished.push_back(task);
  }

  for (Task* task : finished) {
    running_.erase(task);
    delete task;
  }
}

void Para::RunCommands() {
  while (!tasks_.empty() && running_.size() < num_jobs_) {
    Task* task = tasks_.front();
    tasks_.pop();
    task->pid = fork();
    if (task->pid == 0) {
      const char* args[] = {
        task->shell.c_str(),
        "-c",
        task->cmd.c_str(),
        NULL
      };
      PCHECK(execvp(args[0], const_cast<char* const*>(args)));
      abort();
    }
    running_.insert(task);
  }
}

void StdinTaskProvider::PollFD(Para* para) {
  const int BUF_SIZE = 4096;
  char buf[BUF_SIZE];
  ssize_t r = read(STDIN_FILENO, buf, BUF_SIZE);
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

    Task task;
    task.shell = "/bin/sh";
    task.cmd = buf_.substr(0, index);
    if (task.cmd.empty())
      continue;
    para->AddTask(task);
    buf_ = buf_.substr(index + 1);
  }
}

int main(int argc, char* argv[]) {
  TaskProvider* provider = NULL;
  provider = new StdinTaskProvider();
  Para para(provider, 4);
  para.Loop();
}
