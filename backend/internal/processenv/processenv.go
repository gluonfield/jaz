package processenv

import (
	"os"
	"runtime"
	"sort"
)

func Base() map[string]string {
	env := map[string]string{}
	PreserveHost(env, baseKeys()...)
	return env
}

func PreserveHost(env map[string]string, keys ...string) {
	for _, key := range keys {
		if key == "" || env[key] != "" {
			continue
		}
		if value := os.Getenv(key); value != "" {
			env[key] = value
		}
	}
}

func List(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key, value := range env {
		if key != "" && value != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}

func baseKeys() []string {
	keys := []string{"PATH"}
	if runtime.GOOS != "windows" {
		return keys
	}
	return append(keys,
		"APPDATA",
		"ComSpec",
		"LOCALAPPDATA",
		"PATHEXT",
		"ProgramData",
		"ProgramFiles",
		"ProgramFiles(x86)",
		"SystemDrive",
		"SystemRoot",
		"TEMP",
		"TMP",
		"USERPROFILE",
		"WINDIR",
	)
}
