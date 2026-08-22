package envloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSetsNewVariables(t *testing.T) {
	key := "ENVLOADER_TEST_NEW_VAR"
	os.Unsetenv(key)
	t.Cleanup(func() { os.Unsetenv(key) })

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(key+"=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := os.Getenv(key); got != "from-file" {
		t.Fatalf("want %q, got %q", "from-file", got)
	}
}

func TestLoadDoesNotOverrideExisting(t *testing.T) {
	key := "ENVLOADER_TEST_KEEP_VAR"
	t.Setenv(key, "already-set")

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(key+"=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := os.Getenv(key); got != "already-set" {
		t.Fatalf("existing env var was overridden: %q", got)
	}
}

func TestLoadSkipsCommentsAndBlanks(t *testing.T) {
	key := "ENVLOADER_TEST_SKIP_VAR"
	os.Unsetenv(key)
	t.Cleanup(func() { os.Unsetenv(key) })

	path := filepath.Join(t.TempDir(), ".env")
	content := "# comment line\n\n  \n" + key + " = value-with-spaces \n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := os.Getenv(key); got != "value-with-spaces" {
		t.Fatalf("want %q, got %q", "value-with-spaces", got)
	}
}

func TestLoadMissingFileIsIgnored(t *testing.T) {
	if err := Load(filepath.Join(t.TempDir(), "nope.env")); err != nil {
		t.Fatalf("missing env file should be ignored, got %v", err)
	}
}
