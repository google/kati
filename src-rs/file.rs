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

use std::{ffi::OsStr, io::Read, os::unix::ffi::OsStrExt, sync::Arc, time::SystemTime};

use anyhow::Result;
use bytes::Bytes;

use crate::{
    parser::parse_file,
    stmt::Stmt,
    symtab::{Symbol, intern},
};

pub struct Makefile {
    pub filename: Symbol,
    pub mtime: SystemTime,
    pub stmts: Vec<Stmt>,
}

impl Makefile {
    pub fn from_file(filename: &OsStr) -> Result<Option<Arc<Makefile>>> {
        let mut file = match std::fs::File::open(filename) {
            Ok(f) => f,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(None),
            Err(e) => return Err(e.into()),
        };
        let mtime = file.metadata()?.modified()?;
        let mut buf = Vec::new();
        file.read_to_end(&mut buf)?;
        let buf = Bytes::from(buf);

        let filename = intern(filename.as_bytes().to_vec());
        let stmts = parse_file(&buf, filename)?;

        Ok(Some(Arc::new(Makefile {
            filename,
            mtime,
            stmts,
        })))
    }
}
