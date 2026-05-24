package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

func AppCallLocal(action, pattern, param string) {
	switch action {
	case "kill":
		// call kill
		kill(pattern)
	case "update":
		// call update lib ver
		update(pattern, param)
	case "sleep":
		// turn of certain env or envs
		sleep(pattern, param)
	case "awake":
		// turn on certain env or envs
		awake(pattern, param)
	default:
		fmt.Println("I don't know what you mean. There is no such subcommand for app")
		os.Exit(2)
	}
}

func kill(pattern string) {
	files, err := filepath.Glob(pattern)
	Check(err)

	for _, file := range files {
		os.RemoveAll(file)
	}
}

func update(pattern string, version string) {
	if version == "" {
		fmt.Println("You passed empty version, which is illegal.")
		os.Exit(2)
	}
	files, err := filepath.Glob(pattern + "/Chart.yaml")
	Check(err)

	for _, chartPath := range files {

		data, err := os.ReadFile(chartPath)
		Check(err)

		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			panic(err)
		}

		deps := doc["dependencies"].([]any)
		deps[0].(map[string]any)["version"] = version

		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))
		Check(enc.Encode(doc))
		enc.Close()
		Check(os.WriteFile(chartPath, buf.Bytes(), 0644))
	}
}

func awake(pattern, env string) {
	if env == "" {
		env = "*"
	}
	files, err := filepath.Glob(pattern + "/_values-" + env + ".yaml")
	Check(err)

	for _, file := range files {
		filename := filepath.Base(file)
		filename = strings.TrimPrefix(filename, "_")
		newPath := filepath.Join(filepath.Dir(file), filename)
		os.Rename(file, newPath)
	}
}

func sleep(pattern, env string) {
	if env == "" {
		env = "*"
	}
	files, err := filepath.Glob(pattern + "/values-" + env + ".yaml")
	Check(err)

	for _, file := range files {
		filename := filepath.Base(file)
		newPath := filepath.Join(filepath.Dir(file), "_"+filename)
		os.Rename(file, newPath)
	}
}
