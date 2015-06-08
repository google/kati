package main

import (
	"os/exec"
	"path/filepath"
	"strings"
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
