package main

import (
	"io"
	"strings"
)

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
			androidFindCache.init(nil)
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
			androidFindCache.init(nil)
			return &funcShellAndroidFindJavaInDir{
				funcShell: sh,
				chdir:     v[0],
				roots:     v[1],
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
			androidFindCache.init(nil)
			return &funcShellAndroidFindJavaResourceFileGroup{
				funcShell: sh,
				dir:       Expr(v),
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

type funcShellAndroidFindJavaInDir struct {
	*funcShell
	chdir Value
	roots Value
}

func (f *funcShellAndroidFindJavaInDir) Eval(w io.Writer, ev *Evaluator) {
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
	Logf("shellAndroidFindJavaInDir %s,%s => %s,%s", f.chdir.String(), f.roots.String(), chdir, roots)
	if strings.Contains(chdir, "..") || hasDotDot {
		Logf("shellAndroidFindJavaInDir contains ..: call original shell")
		f.funcShell.Eval(w, ev)
		return
	}
	if !androidFindCache.ready() {
		Logf("shellAndroidFindJavaInDir androidFindCache is not ready: call original shell")
		f.funcShell.Eval(w, ev)
		return
	}
	buf := newBuf()
	sw := ssvWriter{w: buf}
	for _, root := range roots {
		if !androidFindCache.findJavaInDir(&sw, chdir, root) {
			freeBuf(buf)
			Logf("shellAndroidFindJavaInDir androidFindCache couldn't handle: call original shell")
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
