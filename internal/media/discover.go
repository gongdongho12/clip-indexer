package media

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var videoExtensions = map[string]bool{
	".3gp":  true,
	".avi":  true,
	".insv": true,
	".lrv":  true,
	".m2ts": true,
	".m4v":  true,
	".mkv":  true,
	".mov":  true,
	".mp4":  true,
	".mpeg": true,
	".mpg":  true,
	".mts":  true,
	".webm": true,
}

func Discover(inputs []string, recursive bool, includeUnsupported bool) ([]string, []string, error) {
	var paths []string
	var warnings []string
	seen := map[string]bool{}

	addPath := func(path string, explicit bool) {
		ext := strings.ToLower(filepath.Ext(path))
		if !includeUnsupported && !videoExtensions[ext] {
			if explicit {
				warnings = append(warnings, fmt.Sprintf("skipped unsupported file extension: %s", path))
			}
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not resolve absolute path for %s: %v", path, err))
			return
		}
		if seen[abs] {
			return
		}
		seen[abs] = true
		paths = append(paths, abs)
	}

	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not stat %s: %v", input, err))
			continue
		}
		if !info.IsDir() {
			addPath(input, true)
			continue
		}

		if recursive {
			walkErr := filepath.WalkDir(input, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("could not read %s: %v", path, err))
					return nil
				}
				if entry.IsDir() {
					return nil
				}
				addPath(path, false)
				return nil
			})
			if walkErr != nil {
				return nil, warnings, walkErr
			}
			continue
		}

		entries, err := os.ReadDir(input)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not read directory %s: %v", input, err))
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			addPath(filepath.Join(input, entry.Name()), false)
		}
	}

	sort.Strings(paths)
	return paths, warnings, nil
}
