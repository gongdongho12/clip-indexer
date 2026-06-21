package media

import (
	"bufio"
	"os"
	"strings"
)

func loadEnvFiles(paths ...string) []string {
	var warnings []string
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, "could not read "+path+": "+err.Error())
			}
			continue
		}
		warnings = append(warnings, loadEnvFile(path, file)...)
		_ = file.Close()
	}
	return warnings
}

func loadEnvFile(path string, file *os.File) []string {
	var warnings []string
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			warnings = append(warnings, path+": invalid env line")
			continue
		}
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			warnings = append(warnings, path+": empty env key")
			continue
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, path+": "+err.Error())
	}
	_ = lineNumber
	return warnings
}
