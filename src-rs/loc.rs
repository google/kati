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

use std::{fmt::Display, sync::LazyLock};

use crate::symtab::{Symbol, intern};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Loc {
    pub filename: Symbol,
    pub line: i32,
}

impl Display for Loc {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}:{}", self.filename, self.line)
    }
}

static DEFAULT_FILENAME: LazyLock<Symbol> = LazyLock::new(|| intern("<unknown>"));

impl Default for Loc {
    fn default() -> Self {
        Loc {
            filename: *DEFAULT_FILENAME,
            line: 0,
        }
    }
}
