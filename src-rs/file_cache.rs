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

use std::{
    collections::{HashMap, HashSet},
    ffi::{OsStr, OsString},
    sync::{Arc, LazyLock},
};

use anyhow::Result;
use parking_lot::Mutex;

use crate::file::Makefile;

static CACHE: LazyLock<Mutex<MakefileCacheManager>> = LazyLock::new(|| {
    Mutex::new(MakefileCacheManager {
        cache: HashMap::new(),
        extra_file_deps: HashSet::new(),
    })
});

struct MakefileCacheManager {
    cache: HashMap<OsString, Option<Arc<Makefile>>>,
    extra_file_deps: HashSet<OsString>,
}

impl MakefileCacheManager {
    fn get_makefile(&mut self, filename: &OsStr) -> Result<Option<Arc<Makefile>>> {
        if let Some(mk) = self.cache.get(filename) {
            return Ok(mk.clone());
        }
        let filename = filename.to_os_string();
        let mk = Makefile::from_file(&filename)?;
        self.cache.insert(filename, mk.clone());
        Ok(mk)
    }
}

pub fn get_makefile(filename: &OsStr) -> Result<Option<Arc<Makefile>>> {
    CACHE.lock().get_makefile(filename)
}

pub fn add_extra_file_dep(filename: OsString) {
    CACHE.lock().extra_file_deps.insert(filename);
}

pub fn get_all_filenames() -> HashSet<OsString> {
    let manager = CACHE.lock();
    let mut ret = HashSet::new();
    for p in manager.cache.keys() {
        ret.insert(p.clone());
    }
    for f in &manager.extra_file_deps {
        ret.insert(f.clone());
    }
    ret
}
