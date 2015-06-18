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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var wildcardCache = make(map[string][]string)

func wildcard(sw *ssvWriter, pat string) {
	if useWildcardCache {
		// TODO(ukai): make sure it didn't chdir?
		if files, ok := wildcardCache[pat]; ok {
			for _, file := range files {
				sw.WriteString(file)
			}
			return
		}
	}
	if strings.Contains(pat, "..") {
		// For some reason, go's Glob normalizes
		// foo/../bar to bar. We ask shell to expand
		// a glob to avoid this.
		cmdline := []string{"/bin/sh", "-c", "/bin/ls -d " + pat}
		cmd := exec.Cmd{
			Path: cmdline[0],
			Args: cmdline,
		}
		// Ignore errors.
		out, _ := cmd.Output()
		if len(trimSpaceBytes(out)) > 0 {
			out = formatCommandOutput(out)
			sw.Write(out)
		}
		if useWildcardCache {
			ws := newWordScanner(out)
			var files []string
			for ws.Scan() {
				files = append(files, string(ws.Bytes()))
			}
			wildcardCache[pat] = files
		}
		return
	}
	files, err := filepath.Glob(pat)
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		sw.WriteString(file)
	}
	if useWildcardCache {
		wildcardCache[pat] = files
	}
}

type fileInfo struct {
	path string
	mode os.FileMode
}

type androidFindCacheT struct {
	once     sync.Once
	filesch  chan []fileInfo
	leavesch chan []fileInfo
	files    []fileInfo
	leaves   []fileInfo
	scanTime time.Duration
}

var (
	androidFindCache androidFindCacheT
)

func (c *androidFindCacheT) ready() bool {
	if c.files != nil {
		return true
	}
	select {
	case c.files = <-c.filesch:
	}
	return c.files != nil
}

func (c *androidFindCacheT) leavesReady() bool {
	if c.leaves != nil {
		return true
	}
	select {
	case c.leaves = <-c.leavesch:
	}
	return c.leaves != nil
}

func (c *androidFindCacheT) init(prunes, leaves []string) {
	c.once.Do(func() {
		c.filesch = make(chan []fileInfo, 1)
		c.leavesch = make(chan []fileInfo, 1)
		go c.start(prunes, leaves)
	})
}

func (c *androidFindCacheT) start(prunes, leafNames []string) {
	Logf("find cache init: prunes=%q leafNames=%q", prunes, leafNames)
	te := traceEvent.begin("findcache", literal("init"), traceEventFindCache)
	defer func() {
		traceEvent.end(te)
		c.scanTime = time.Since(te.t)
		LogStats("android find cache scan: %v", c.scanTime)
	}()

	dirs := make(chan string, 32)
	filech := make(chan fileInfo, 1000)
	leafch := make(chan fileInfo, 1000)
	var wg sync.WaitGroup
	numWorker := runtime.NumCPU() - 1
	wg.Add(numWorker)
	for i := 0; i < numWorker; i++ {
		go func() {
			defer wg.Done()
			for dir := range dirs {
				err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
					if info.IsDir() {
						for _, prune := range prunes {
							if info.Name() == prune {
								Logf("find cache prune: %s", path)
								return filepath.SkipDir
							}
						}
					}
					filech <- fileInfo{
						path: path,
						mode: info.Mode(),
					}
					for _, leaf := range leafNames {
						if info.Name() == leaf {
							Logf("find cache leaf: %s", path)
							leafch <- fileInfo{
								path: path,
								mode: info.Mode(),
							}
							break
						}
					}
					return nil
				})
				if err != nil && err != filepath.SkipDir {
					Logf("error in adnroid find cache: %v", err)
					close(c.filesch)
					close(c.leavesch)
					return
				}
			}
		}()
	}

	go func() {
		dirs := make(map[string]bool)
		leavesTe := traceEvent.begin("findcache", literal("leaves"), traceEventFindCacheLeaves)
		var leaves []fileInfo
		nfiles := 0
		for leaf := range leafch {
			leaves = append(leaves, leaf)
			nfiles++
			for dir := filepath.Dir(leaf.path); dir != "."; dir = filepath.Dir(dir) {
				if dirs[dir] {
					break
				}
				leaves = append(leaves, fileInfo{
					path: dir,
					mode: leaf.mode | os.ModeDir,
				})
				dirs[dir] = true
			}
		}
		sort.Sort(fileInfoByLeaf(leaves))
		c.leavesch <- leaves
		traceEvent.end(leavesTe)
		LogStats("%d leaves %d dirs in find cache", nfiles, len(dirs))
		for i, leaf := range leaves {
			Logf("android findleaves cache: %d: %s %v", i, leaf.path, leaf.mode)
		}
	}()

	go func() {
		filesTe := traceEvent.begin("findcache", literal("files"), traceEventFindCacheFiles)
		var files []fileInfo
		for file := range filech {
			files = append(files, file)
		}
		sort.Sort(fileInfoByName(files))
		c.filesch <- files
		traceEvent.end(filesTe)
		LogStats("%d files in find cache", len(files))
		for i, fi := range files {
			Logf("android find cache: %d: %s %v", i, fi.path, fi.mode)
		}
	}()

	curdir, err := os.Open(".")
	if err != nil {
		Logf("open . failed: %v", err)
		close(c.filesch)
		close(c.leavesch)
		return
	}
	names, err := curdir.Readdirnames(-1)
	if err != nil {
		Logf("readdir . failed: %v", err)
		close(c.filesch)
		close(c.leavesch)
		return
	}
	curdir.Close()

	for _, name := range names {
		dirs <- name
	}
	close(dirs)
	wg.Wait()
	close(filech)
	close(leafch)
}

type fileInfoByName []fileInfo

func (f fileInfoByName) Len() int      { return len(f) }
func (f fileInfoByName) Swap(i, j int) { f[i], f[j] = f[j], f[i] }
func (f fileInfoByName) Less(i, j int) bool {
	return f[i].path < f[j].path
}

type fileInfoByLeaf []fileInfo

func (f fileInfoByLeaf) Len() int      { return len(f) }
func (f fileInfoByLeaf) Swap(i, j int) { f[i], f[j] = f[j], f[i] }
func (f fileInfoByLeaf) Less(i, j int) bool {
	di := strings.Count(f[i].path, "/")
	dj := strings.Count(f[j].path, "/")
	if di != dj {
		return di < dj
	}
	diri := filepath.Dir(f[i].path) + "/"
	dirj := filepath.Dir(f[j].path) + "/"
	if diri != dirj {
		return diri < dirj
	}
	mdi := f[i].mode & os.ModeDir
	mdj := f[j].mode & os.ModeDir
	if mdi != mdj {
		return mdi < mdj
	}
	return f[i].path < f[j].path
}

var errSkipDir = errors.New("skip dir")

func (c *androidFindCacheT) walk(dir string, walkFn func(int, fileInfo) error) error {
	i := sort.Search(len(c.files), func(i int) bool {
		return c.files[i].path >= dir
	})
	Logf("android find in dir cache: %s i=%d/%d", dir, i, len(c.files))
	start := i
	var skipdirs []string
Loop:
	for i := start; i < len(c.files); i++ {
		if c.files[i].path == dir {
			err := walkFn(i, c.files[i])
			if err != nil {
				return err
			}
			continue
		}
		if !strings.HasPrefix(c.files[i].path, dir) {
			Logf("android find in dir cache: %s end=%d/%d", dir, i, len(c.files))
			return nil
		}
		if !strings.HasPrefix(c.files[i].path, dir+"/") {
			continue
		}
		for _, skip := range skipdirs {
			if strings.HasPrefix(c.files[i].path, skip+"/") {
				continue Loop
			}
		}

		err := walkFn(i, c.files[i])
		if err == errSkipDir {
			Logf("android find in skip dir: %s", c.files[i].path)
			skipdirs = append(skipdirs, c.files[i].path)
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// pattern in repo/android/build/core/definitions.mk
// find-subdir-assets
// if [ -d $1 ] ; then cd $1 ; find ./ -not -name '.*' -and -type f -and -not -type l ; fi
func (c *androidFindCacheT) findInDir(sw *ssvWriter, dir string) {
	dir = filepath.Clean(dir)
	Logf("android find in dir cache: %s", dir)
	c.walk(dir, func(_ int, fi fileInfo) error {
		// -not -name '.*'
		if strings.HasPrefix(filepath.Base(fi.path), ".") {
			return nil
		}
		// -type f and -not -type l
		// regular type and not symlink
		if !fi.mode.IsRegular() {
			return nil
		}
		name := strings.TrimPrefix(fi.path, dir+"/")
		name = "./" + name
		sw.WriteString(name)
		Logf("android find in dir cache: %s=> %s", dir, name)
		return nil
	})
}

// pattern in repo/android/build/core/definitions.mk
// all-java-files-under
// cd ${LOCAL_PATH} ; find -L $1 -name "*.java" -and -not -name ".*"
// returns false if symlink is found.
func (c *androidFindCacheT) findJavaInDir(sw *ssvWriter, chdir string, root string) bool {
	chdir = filepath.Clean(chdir)
	dir := filepath.Join(chdir, root)
	Logf("android find java in dir cache: %s %s", chdir, root)
	// check symlinks
	var matches []int
	err := c.walk(dir, func(i int, fi fileInfo) error {
		if fi.mode&os.ModeSymlink == os.ModeSymlink {
			Logf("android find java in dir cache: detect symlink %s %v", c.files[i].path, c.files[i].mode)
			return fmt.Errorf("symlink %s", fi.path)
		}
		matches = append(matches, i)
		return nil
	})
	if err != nil {
		return false
	}
	// no symlinks
	for _, i := range matches {
		fi := c.files[i]
		base := filepath.Base(fi.path)
		// -name "*.java"
		if filepath.Ext(base) != ".java" {
			continue
		}
		// -not -name ".*"
		if strings.HasPrefix(base, ".") {
			continue
		}
		name := strings.TrimPrefix(fi.path, chdir+"/")
		sw.WriteString(name)
		Logf("android find java in dir cache: %s=> %s", dir, name)
	}
	return true
}

// pattern: in repo/android/build/core/base_rules.mk
// java_resource_file_groups+= ...
// cd ${TOP_DIR}${LOCAL_PATH}/${dir} && find . -type d -a -name ".svn" -prune \
// -o -type f -a \! -name "*.java" -a \! -name "package.html" -a \! \
// -name "overview.html" -a \! -name ".*.swp" -a \! -name ".DS_Store" \
// -a \! -name "*~" -print )
func (c *androidFindCacheT) findJavaResourceFileGroup(sw *ssvWriter, dir string) {
	Logf("android find java resource in dir cache: %s", dir)
	c.walk(filepath.Clean(dir), func(_ int, fi fileInfo) error {
		// -type d -a -name ".svn" -prune
		if fi.mode.IsDir() && filepath.Base(fi.path) == ".svn" {
			return errSkipDir
		}
		// -type f
		if !fi.mode.IsRegular() {
			return nil
		}
		// ! -name "*.java" -a ! -name "package.html" -a
		// ! -name "overview.html" -a ! -name ".*.swp" -a
		// ! -name ".DS_Store" -a ! -name "*~"
		base := filepath.Base(fi.path)
		if filepath.Ext(base) == ".java" ||
			base == "package.html" ||
			base == "overview.html" ||
			(strings.HasPrefix(base, ".") && strings.HasSuffix(base, ".swp")) ||
			base == ".DS_Store" ||
			strings.HasSuffix(base, "~") {
			return nil
		}
		name := strings.TrimPrefix(fi.path, dir+"/")
		name = "./" + name
		sw.WriteString(name)
		Logf("android find java resource in dir cache: %s=> %s", dir, name)
		return nil
	})
}

func (c *androidFindCacheT) findleaves(sw *ssvWriter, dir, name string, prunes []string, mindepth int) bool {
	var found []string
	var dirs []string
	topdepth := 1 + strings.Count(dir, "/")
	dirs = append(dirs, dir)
	for len(dirs) > 0 {
		dir = filepath.Clean(dirs[0]) + "/"
		dirs = dirs[1:]
		if dir == "./" {
			dir = ""
		}
		depth := strings.Count(dir, "/")
		// Logf("android findleaves dir=%q depth=%d dirs=%q", dir, depth, dirs)
		i := sort.Search(len(c.leaves), func(i int) bool {
			di := strings.Count(c.leaves[i].path, "/")
			if di != depth {
				return di >= depth
			}
			diri := filepath.Dir(c.leaves[i].path) + "/"
			if diri != dir {
				return diri >= dir
			}
			return c.leaves[i].path >= dir
		})
		Logf("android findleaves dir=%q i=%d/%d", dir, i, len(c.leaves))

	Scandir:
		for ; i < len(c.leaves); i++ {
			if dir == "" && strings.Contains(c.leaves[i].path, "/") {
				break
			}
			if !strings.HasPrefix(c.leaves[i].path, dir) {
				break
			}
			if mindepth < 0 || depth >= topdepth+mindepth {
				if !c.leaves[i].mode.IsDir() && filepath.Base(c.leaves[i].path) == name {
					n := "./" + c.leaves[i].path
					found = append(found, n)
					Logf("android findleaves name=%s=> %s (depth=%d topdepth=%d mindepth=%d)", name, n, depth, topdepth, mindepth)
					break Scandir
				}
			}
			if c.leaves[i].mode.IsDir() {
				dirs = append(dirs, c.leaves[i].path)
			}
		}
		// Logf("android findleaves next dirs=%q", dirs)
	}
	Logf("android findleave done")
	sort.Strings(found)
	for _, f := range found {
		sw.WriteString(f)
	}
	return true
}
