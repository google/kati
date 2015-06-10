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
	mu       sync.Mutex
	ok       bool
	files    []fileInfo
	scanTime time.Duration
}

var (
	androidFindCache androidFindCacheT
)

func (c *androidFindCacheT) ready() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ok
}

func (c *androidFindCacheT) init() {
	c.once.Do(func() {
		c.mu.Lock()
		go c.start()
	})
}

func (c *androidFindCacheT) start() {
	defer c.mu.Unlock()
	t := time.Now()
	defer func() {
		c.scanTime = time.Since(t)
		Logf("android find cache scan: %v", c.scanTime)
	}()

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		c.files = append(c.files, fileInfo{
			path: strings.TrimPrefix(path, "./"),
			mode: info.Mode(),
		})
		return nil
	})
	if err != nil {
		Logf("error in adnroid find cache: %v", err)
		c.ok = false
		return
	}
	sort.Sort(fileInfoByName(c.files))
	for i, fi := range c.files {
		Logf("android find cache: %d: %s %v", i, fi.path, fi.mode)
	}
	c.ok = true
}

type fileInfoByName []fileInfo

func (f fileInfoByName) Len() int      { return len(f) }
func (f fileInfoByName) Swap(i, j int) { f[i], f[j] = f[j], f[i] }
func (f fileInfoByName) Less(i, j int) bool {
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
