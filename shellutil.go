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

package main

import (
	"io"
	"strings"
)

var androidDefaultLeafNames = []string{"CleanSpec.mk", "Android.mk"}

var shBuiltins = []struct {
	name    string
	pattern Expr
	compact func(*funcShell, []Value) Value
}{
	{
		name: "android:rot13",
		// in repo/android/build/core/definisions.mk
		// echo $(1) | tr 'a-zA-Z' 'n-za-mN-ZA-M'
		pattern: Expr{
			literal("echo "),
			matchVarref{},
			literal(" | tr 'a-zA-Z' 'n-za-mN-ZA-M'"),
		},
		compact: func(sh *funcShell, matches []Value) Value {
			return &funcShellAndroidRot13{
				funcShell: sh,
				v:         matches[0],
			}
		},
	},
	{
		name: "android:find-subdir-assets",
		// in repo/android/build/core/definitions.mk
		// if [ -d $1 ] ; then cd $1 ; find ./ -not -name '.*' -and -type f -and -not -type l ; fi
		pattern: Expr{
			literal("if [ -d "),
			matchVarref{},
			literal(" ] ; then cd "),
			matchVarref{},
			literal(" ; find ./ -not -name '.*' -and -type f -and -not -type l ; fi"),
		},
		compact: func(sh *funcShell, v []Value) Value {
			if v[0] != v[1] {
				return sh
			}
			androidFindCache.init(nil, androidDefaultLeafNames)
			return &funcShellAndroidFindFileInDir{
				funcShell: sh,
				dir:       v[0],
			}
		},
	},
	{
		name: "android:all-java-files-under",
		// in repo/android/build/core/definitions.mk
		// cd ${LOCAL_PATH} ; find -L $1 -name "*.java" -and -not -name ".*"
		pattern: Expr{
			literal("cd "),
			matchVarref{},
			literal(" ; find -L "),
			matchVarref{},
			literal(` -name "*.java" -and -not -name ".*"`),
		},
		compact: func(sh *funcShell, v []Value) Value {
			androidFindCache.init(nil, androidDefaultLeafNames)
			return &funcShellAndroidFindExtFilesUnder{
				funcShell: sh,
				chdir:     v[0],
				roots:     v[1],
				ext:       ".java",
			}
		},
	},
	{
		name: "android:all-proto-files-under",
		// in repo/android/build/core/definitions.mk
		// cd $(LOCAL_PATH) ; \
		// find -L $(1) -name "*.proto" -and -not -name ".*"
		pattern: Expr{
			literal("cd "),
			matchVarref{},
			literal(" ; find -L "),
			matchVarref{},
			literal(" -name \"*.proto\" -and -not -name \".*\""),
		},
		compact: func(sh *funcShell, v []Value) Value {
			androidFindCache.init(nil, androidDefaultLeafNames)
			return &funcShellAndroidFindExtFilesUnder{
				funcShell: sh,
				chdir:     v[0],
				roots:     v[1],
				ext:       ".proto",
			}
		},
	},
	{
		name: "android:java_resource_file_groups",
		// in repo/android/build/core/base_rules.mk
		// cd ${TOP_DIR}${LOCAL_PATH}/${dir} && find . -type d -a \
		// -name ".svn" -prune -o -type f -a \! -name "*.java" \
		// -a \! -name "package.html" -a \! -name "overview.html" \
		// -a \! -name ".*.swp" -a \! -name ".DS_Store" \
		// -a \! -name "*~" -print )
		pattern: Expr{
			literal("cd "),
			matchVarref{},
			matchVarref{},
			literal("/"),
			matchVarref{},
			literal(` && find . -type d -a -name ".svn" -prune -o -type f -a \! -name "*.java" -a \! -name "package.html" -a \! -name "overview.html" -a \! -name ".*.swp" -a \! -name ".DS_Store" -a \! -name "*~" -print `),
		},
		compact: func(sh *funcShell, v []Value) Value {
			androidFindCache.init(nil, androidDefaultLeafNames)
			return &funcShellAndroidFindJavaResourceFileGroup{
				funcShell: sh,
				dir:       Expr(v),
			}
		},
	},
	{
		name: "android:subdir_cleanspecs",
		// in repo/android/build/core/cleanspec.mk
		// build/tools/findleaves.py --prune=$(OUT_DIR) --prune=.repo --prune=.git . CleanSpec.mk)
		pattern: Expr{
			literal("build/tools/findleaves.py --prune="),
			matchVarref{},
			literal(" --prune=.repo --prune=.git . CleanSpec.mk"),
		},
		compact: func(sh *funcShell, v []Value) Value {
			if !contains(androidDefaultLeafNames, "CleanSpec.mk") {
				return sh
			}
			androidFindCache.init(nil, androidDefaultLeafNames)
			return &funcShellAndroidFindleaves{
				funcShell: sh,
				prunes: []Value{
					v[0],
					literal(".repo"),
					literal(".git"),
				},
				dirlist:  literal("."),
				name:     literal("CleanSpec.mk"),
				mindepth: -1,
			}
		},
	},
	{
		name: "android:subdir_makefiles",
		// in repo/android/build/core/main.mk
		// build/tools/findleaves.py --prune=$(OUT_DIR) --prune=.repo --prune=.git $(subdirs) Android.mk
		pattern: Expr{
			literal("build/tools/findleaves.py --prune="),
			matchVarref{},
			literal(" --prune=.repo --prune=.git "),
			matchVarref{},
			literal(" Android.mk"),
		},
		compact: func(sh *funcShell, v []Value) Value {
			if !contains(androidDefaultLeafNames, "Android.mk") {
				return sh
			}
			androidFindCache.init(nil, androidDefaultLeafNames)
			return &funcShellAndroidFindleaves{
				funcShell: sh,
				prunes: []Value{
					v[0],
					literal(".repo"),
					literal(".git"),
				},
				dirlist:  v[1],
				name:     literal("Android.mk"),
				mindepth: -1,
			}
		},
	},
	{
		name: "android:first-makefiles-under",
		// in repo/android/build/core/definisions.mk
		// build/tools/findleaves.py --prune=$(OUT_DIR) --prune=.repo --prune=.git \
		// --mindepth=2 $(1) Android.mk
		pattern: Expr{
			literal("build/tools/findleaves.py --prune="),
			matchVarref{},
			literal(" --prune=.repo --prune=.git --mindepth=2 "),
			matchVarref{},
			literal(" Android.mk"),
		},
		compact: func(sh *funcShell, v []Value) Value {
			if !contains(androidDefaultLeafNames, "Android.mk") {
				return sh
			}
			androidFindCache.init(nil, androidDefaultLeafNames)
			return &funcShellAndroidFindleaves{
				funcShell: sh,
				prunes: []Value{
					v[0],
					literal(".repo"),
					literal(".git"),
				},
				dirlist:  v[1],
				name:     literal("Android.mk"),
				mindepth: 2,
			}
		},
	},
}

type funcShellAndroidRot13 struct {
	*funcShell
	v Value
}

func rot13(buf []byte) {
	for i, b := range buf {
		// tr 'a-zA-Z' 'n-za-mN-ZA-M'
		if b >= 'a' && b <= 'z' {
			b += 'n' - 'a'
			if b > 'z' {
				b -= 'z' - 'a' + 1
			}
		} else if b >= 'A' && b <= 'Z' {
			b += 'N' - 'A'
			if b > 'Z' {
				b -= 'Z' - 'A' + 1
			}
		}
		buf[i] = b
	}
}

func (f *funcShellAndroidRot13) Eval(w io.Writer, ev *Evaluator) {
	abuf := newBuf()
	fargs := ev.args(abuf, f.v)
	rot13(fargs[0])
	w.Write(fargs[0])
	freeBuf(abuf)
}

type funcShellAndroidFindFileInDir struct {
	*funcShell
	dir Value
}

func (f *funcShellAndroidFindFileInDir) Eval(w io.Writer, ev *Evaluator) {
	abuf := newBuf()
	fargs := ev.args(abuf, f.dir)
	dir := string(trimSpaceBytes(fargs[0]))
	freeBuf(abuf)
	Logf("shellAndroidFindFileInDir %s => %s", f.dir.String(), dir)
	if strings.Contains(dir, "..") {
		Logf("shellAndroidFindFileInDir contains ..: call original shell")
		f.funcShell.Eval(w, ev)
		return
	}
	if !androidFindCache.ready() {
		Logf("shellAndroidFindFileInDir androidFindCache is not ready: call original shell")
		f.funcShell.Eval(w, ev)
		return
	}
	sw := ssvWriter{w: w}
	androidFindCache.findInDir(&sw, dir)
}

type funcShellAndroidFindExtFilesUnder struct {
	*funcShell
	chdir Value
	roots Value
	ext   string
}

func (f *funcShellAndroidFindExtFilesUnder) Eval(w io.Writer, ev *Evaluator) {
	abuf := newBuf()
	fargs := ev.args(abuf, f.chdir, f.roots)
	chdir := string(trimSpaceBytes(fargs[0]))
	var roots []string
	hasDotDot := false
	ws := newWordScanner(fargs[1])
	for ws.Scan() {
		root := string(ws.Bytes())
		if strings.Contains(root, "..") {
			hasDotDot = true
		}
		roots = append(roots, string(ws.Bytes()))
	}
	freeBuf(abuf)
	Logf("shellAndroidFindExtFilesUnder %s,%s => %s,%s", f.chdir.String(), f.roots.String(), chdir, roots)
	if strings.Contains(chdir, "..") || hasDotDot {
		Logf("shellAndroidFindExtFilesUnder contains ..: call original shell")
		f.funcShell.Eval(w, ev)
		return
	}
	if !androidFindCache.ready() {
		Logf("shellAndroidFindExtFilesUnder androidFindCache is not ready: call original shell")
		f.funcShell.Eval(w, ev)
		return
	}
	buf := newBuf()
	sw := ssvWriter{w: buf}
	for _, root := range roots {
		if !androidFindCache.findExtFilesUnder(&sw, chdir, root, f.ext) {
			freeBuf(buf)
			Logf("shellAndroidFindExtFilesUnder androidFindCache couldn't handle: call original shell")
			f.funcShell.Eval(w, ev)
			return
		}
	}
	w.Write(buf.Bytes())
	freeBuf(buf)
}

type funcShellAndroidFindJavaResourceFileGroup struct {
	*funcShell
	dir Value
}

func (f *funcShellAndroidFindJavaResourceFileGroup) Eval(w io.Writer, ev *Evaluator) {
	abuf := newBuf()
	fargs := ev.args(abuf, f.dir)
	dir := string(trimSpaceBytes(fargs[0]))
	freeBuf(abuf)
	Logf("shellAndroidFindJavaResourceFileGroup %s => %s", f.dir.String(), dir)
	if strings.Contains(dir, "..") {
		Logf("shellAndroidFindJavaResourceFileGroup contains ..: call original shell")
		f.funcShell.Eval(w, ev)
		return
	}
	if !androidFindCache.ready() {
		Logf("shellAndroidFindJavaResourceFileGroup androidFindCache is not ready: call original shell")
		f.funcShell.Eval(w, ev)
		return
	}
	sw := ssvWriter{w: w}
	androidFindCache.findJavaResourceFileGroup(&sw, dir)
}

type funcShellAndroidFindleaves struct {
	*funcShell
	prunes   []Value
	dirlist  Value
	name     Value
	mindepth int
}

func (f *funcShellAndroidFindleaves) Eval(w io.Writer, ev *Evaluator) {
	if !androidFindCache.leavesReady() {
		Logf("shellAndroidFindleaves androidFindCache is not ready: call original shell")
		f.funcShell.Eval(w, ev)
		return
	}
	abuf := newBuf()
	var params []Value
	params = append(params, f.name)
	params = append(params, f.dirlist)
	params = append(params, f.prunes...)
	fargs := ev.args(abuf, params...)
	name := string(trimSpaceBytes(fargs[0]))
	var dirs []string
	ws := newWordScanner(fargs[1])
	for ws.Scan() {
		dir := string(ws.Bytes())
		if strings.Contains(dir, "..") {
			Logf("shellAndroidFindleaves contains .. in %s: call original shell", dir)
			f.funcShell.Eval(w, ev)
			return
		}
		dirs = append(dirs, dir)
	}
	var prunes []string
	for _, arg := range fargs[2:] {
		prunes = append(prunes, string(trimSpaceBytes(arg)))
	}
	freeBuf(abuf)

	sw := ssvWriter{w: w}
	for _, dir := range dirs {
		androidFindCache.findleaves(&sw, dir, name, prunes, f.mindepth)
	}
}
