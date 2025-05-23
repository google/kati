/*
Copyright 2025 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

use std::{collections::HashMap, ffi::OsStr, os::unix::ffi::OsStrExt, sync::Arc, time::SystemTime};

use anyhow::Result;
use bytes::Bytes;
use parking_lot::Mutex;

use crate::{
    command::CommandEvaluator,
    dep::{DepNode, NamedDepNode},
    error,
    eval::{Evaluator, FrameType},
    fileutil::{RedirectStderr, get_timestamp, run_command},
    flags::FLAGS,
    log,
    symtab::Symbol,
    warn,
};

#[derive(Clone, Copy, PartialEq, Eq)]
pub enum ExecStatus {
    Processing,
    Timestamp(Option<SystemTime>),
}

impl PartialOrd for ExecStatus {
    fn partial_cmp(&self, other: &Self) -> Option<std::cmp::Ordering> {
        match (self, other) {
            (ExecStatus::Processing, ExecStatus::Processing) => Some(std::cmp::Ordering::Equal),
            (ExecStatus::Processing, ExecStatus::Timestamp(Some(_))) => {
                Some(std::cmp::Ordering::Less)
            }
            (ExecStatus::Timestamp(None), ExecStatus::Timestamp(None)) => {
                Some(std::cmp::Ordering::Equal)
            }
            (ExecStatus::Timestamp(None), _) => Some(std::cmp::Ordering::Less),
            (_, ExecStatus::Timestamp(None)) => Some(std::cmp::Ordering::Greater),
            (ExecStatus::Timestamp(Some(a)), ExecStatus::Timestamp(Some(b))) => Some(a.cmp(b)),
            (ExecStatus::Timestamp(Some(_)), _) => Some(std::cmp::Ordering::Greater),
        }
    }
}

struct Executor<'a> {
    ce: CommandEvaluator<'a>,
    done: HashMap<Symbol, ExecStatus>,
    shell: Bytes,
    shellflag: &'static [u8],
    num_commands: u64,
}

impl<'a> Executor<'a> {
    fn new(ev: &'a mut Evaluator) -> Result<Self> {
        let shell = ev.get_shell()?;
        let shellflag = ev.get_shell_flag();
        Ok(Executor {
            ce: CommandEvaluator::new(ev)?,
            done: HashMap::new(),
            shell,
            shellflag,
            num_commands: 0,
        })
    }

    fn exec_node(
        &mut self,
        n: &Arc<Mutex<DepNode>>,
        needed_by: Option<&[u8]>,
    ) -> Result<ExecStatus> {
        let output = n.lock().output;
        let output_str = output.as_bytes();
        if let Some(found) = self.done.get(&output) {
            if found == &ExecStatus::Processing {
                warn!(
                    "Circular {} <- {} dependency dropped.",
                    String::from_utf8_lossy(needed_by.unwrap_or(b"(null)")),
                    output
                )
            }
            return Ok(*found);
        }
        let loc = n.lock().loc.clone();
        let _frame = self
            .ce
            .ev
            .enter(FrameType::Exec, output_str.clone(), loc.unwrap_or_default());

        self.done.insert(output, ExecStatus::Processing);
        let output_timestamp = get_timestamp(&output_str)?;
        let output_ts = ExecStatus::Timestamp(output_timestamp);

        log!(
            "ExecNode: {output} for {}",
            String::from_utf8_lossy(needed_by.unwrap_or(b"(null)"))
        );

        if !n.lock().has_rule && output_timestamp.is_none() && !n.lock().is_phony {
            if let Some(needed_by) = needed_by {
                error!(
                    "*** No rule to make target '{output}', needed by '{}'.",
                    String::from_utf8_lossy(needed_by)
                );
            } else {
                error!("*** No rule to make target '{output}'");
            }
        }

        let mut latest = ExecStatus::Processing;
        let order_onlys = n.lock().order_onlys.clone();
        for (_, d) in order_onlys {
            let dep_out = d.lock().output.as_bytes();
            if std::fs::exists(OsStr::from_bytes(&dep_out))? {
                continue;
            }
            let ts = self.exec_node(&d, Some(&output_str))?;
            if latest < ts {
                latest = ts;
            }
        }

        let deps = n.lock().deps.clone();
        for (_, d) in deps {
            let ts = self.exec_node(&d, Some(&output_str))?;
            if latest < ts {
                latest = ts;
            }
        }

        if output_ts >= latest && !n.lock().is_phony {
            self.done.insert(output, output_ts);
            return Ok(output_ts);
        }

        let commands = self.ce.eval(n)?;
        for command in commands {
            self.num_commands += 1;
            if command.echo {
                println!("{}", String::from_utf8_lossy(&command.cmd));
            }
            if !FLAGS.is_dry_run {
                let (status, output) = run_command(
                    &self.shell,
                    self.shellflag,
                    &command.cmd,
                    RedirectStderr::Stdout,
                )?;
                print!("{}", String::from_utf8_lossy(&output));
                if !status.success() {
                    if command.ignore_error {
                        eprintln!(
                            "[{}] Error {} (ignored)",
                            command.output,
                            status.code().unwrap_or(1)
                        )
                    } else {
                        error!(
                            "*** [{}] Error {}",
                            command.output,
                            status.code().unwrap_or(1)
                        );
                    }
                }
            }
        }

        self.done.insert(output, output_ts);
        Ok(output_ts)
    }
}

pub fn exec(roots: Vec<NamedDepNode>, ev: &mut Evaluator) -> Result<()> {
    let mut executor = Executor::new(ev)?;
    for (_sym, root) in &roots {
        executor.exec_node(root, None)?;
    }
    if executor.num_commands == 0 {
        for (sym, _) in roots {
            println!("kati: Nothing to be done for `{sym}'.")
        }
    }
    Ok(())
}
