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

use std::os::unix::ffi::OsStrExt;

use anyhow::Result;
use bytes::Bytes;
use kati::find::*;

fn main() -> Result<()> {
    env_logger::builder()
        .filter_level(log::LevelFilter::Trace)
        .parse_default_env()
        .init();
    let Some(cmd) = std::env::args_os().nth(1) else {
        log::error!("Usage: {} <command>", std::env::args().next().unwrap());
        std::process::exit(1);
    };

    let cmd = Bytes::from(cmd.as_bytes().to_vec());
    let Some(fc) = parse(&cmd)? else {
        log::error!("Unsupported command: {cmd:?}");
        std::process::exit(1);
    };

    let Some(output) = find(&cmd, &fc, &kati::loc::Loc::default())? else {
        log::error!("Unable to run command {cmd:?}");
        std::process::exit(1);
    };

    println!("{}", String::from_utf8_lossy(&output));
    Ok(())
}
