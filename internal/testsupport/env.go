package testsupport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

func LoadDotEnv(t testing.TB) {
	t.Helper()

	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for {
		envPath := filepath.Join(currentDir, ".env")
		if _, err := os.Stat(envPath); err == nil {
			if err := godotenv.Load(envPath); err != nil {
				t.Fatalf("failed to load .env: %v", err)
			}
			trimEnvValues()
			return
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return
		}

		currentDir = parentDir
	}
}

func trimEnvValues() {
	for _, env := range os.Environ() {
		key, value, found := strings.Cut(env, "=")
		if !found {
			continue
		}

		os.Setenv(key, strings.TrimSpace(value))
	}
}
