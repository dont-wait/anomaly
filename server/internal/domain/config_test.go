package domain

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestGetEnvLoaderReturnsSingleton(t *testing.T) {
	if got := GetEnvLoader(); got != &loader {
		t.Fatalf("GetEnvLoader() = %p, want %p", got, &loader)
	}
}

func TestLoaderLoadLoadsDotEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/test\n")
	writeTestFile(t, filepath.Join(tmpDir, ".env"), "TEST_ENV_KEY=from-dotenv\n")

	restoreWD := chdirForTest(t, tmpDir)
	defer restoreWD()

	oldVal, hadVal := os.LookupEnv("TEST_ENV_KEY")
	if hadVal {
		if err := os.Unsetenv("TEST_ENV_KEY"); err != nil {
			t.Fatalf("unset env: %v", err)
		}
	}
	t.Cleanup(func() {
		if hadVal {
			_ = os.Setenv("TEST_ENV_KEY", oldVal)
			return
		}
		_ = os.Unsetenv("TEST_ENV_KEY")
	})

	log := testLogger()
	loader := &Loader{}

	got := loader.Load(log)

	if got != loader {
		t.Fatalf("Load() returned different loader: got %p want %p", got, loader)
	}
	if !loader.loaded {
		t.Fatal("Load() did not mark loader as loaded")
	}
	if loader.log != log {
		t.Fatal("Load() did not store logger")
	}
	if got := os.Getenv("TEST_ENV_KEY"); got != "from-dotenv" {
		t.Fatalf("TEST_ENV_KEY = %q, want %q", got, "from-dotenv")
	}
}

func TestLoaderLoadSkipsReloadWhenAlreadyLoaded(t *testing.T) {
	original := testLogger()
	replacement := testLogger()
	loader := &Loader{loaded: true, log: original}

	got := loader.Load(replacement)

	if got != loader {
		t.Fatalf("Load() returned different loader: got %p want %p", got, loader)
	}
	if loader.log != original {
		t.Fatal("Load() replaced logger on an already loaded loader")
	}
}

func TestLoaderLoggerReturnsStoredLogger(t *testing.T) {
	log := testLogger()
	loader := &Loader{log: log}

	if got := loader.logger(); got != log {
		t.Fatalf("logger() = %p, want %p", got, log)
	}
}

func TestLoaderLoggerReturnsNopWhenNil(t *testing.T) {
	loader := &Loader{}

	if got := loader.logger(); got == nil {
		t.Fatal("logger() returned nil")
	}
}

func TestLoaderLoadAllConfig(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://db.example:27017")
	t.Setenv("MONGO_DB", "banking")
	t.Setenv("JWT_SECRET", "test-secret-32-chars-minimum-len-xx")

	loader := &Loader{log: testLogger()}
	got := loader.LoadAllConfig()

	if got == nil || got.MongoConfig == nil {
		t.Fatal("LoadAllConfig() returned nil config")
	}
	if got.MongoConfig.MongoURI != "mongodb://db.example:27017" {
		t.Fatalf("MongoURI = %q, want %q", got.MongoConfig.MongoURI, "mongodb://db.example:27017")
	}
	if got.MongoConfig.MongoDBName != "banking" {
		t.Fatalf("MongoDBName = %q, want %q", got.MongoConfig.MongoDBName, "banking")
	}
	if got.AuthConfig == nil {
		t.Fatal("LoadAllConfig() returned nil AuthConfig")
	}
	if got.AuthConfig.JWTSecret != "test-secret-32-chars-minimum-len-xx" {
		t.Fatalf("JWTSecret = %q, want %q", got.AuthConfig.JWTSecret, "test-secret-32-chars-minimum-len-xx")
	}
}

func TestLoaderLoadMongoConfigUsesDefaults(t *testing.T) {
	t.Setenv("MONGO_URI", "")
	t.Setenv("MONGO_DB", "")

	loader := &Loader{log: testLogger()}
	got := loader.LoadMongoConfig()

	if got.MongoURI != "mongodb://localhost:27017" {
		t.Fatalf("MongoURI = %q, want %q", got.MongoURI, "mongodb://localhost:27017")
	}
	if got.MongoDBName != "anomaly" {
		t.Fatalf("MongoDBName = %q, want %q", got.MongoDBName, "anomaly")
	}
}

func TestLoaderLoadMongoConfigUsesEnvOverrides(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://db.internal:27018")
	t.Setenv("MONGO_DB", "payments")

	loader := &Loader{log: testLogger()}
	got := loader.LoadMongoConfig()

	if got.MongoURI != "mongodb://db.internal:27018" {
		t.Fatalf("MongoURI = %q, want %q", got.MongoURI, "mongodb://db.internal:27018")
	}
	if got.MongoDBName != "payments" {
		t.Fatalf("MongoDBName = %q, want %q", got.MongoDBName, "payments")
	}
}

func TestLoaderLoadEnvReturnsValue(t *testing.T) {
	t.Setenv("APP_ENV", "dev")

	loader := &Loader{log: testLogger()}
	if got := loader.LoadEnv("APP_ENV"); got != "dev" {
		t.Fatalf("LoadEnv() = %q, want %q", got, "dev")
	}
}

func TestLoaderLoadEnvOr(t *testing.T) {
	t.Run("returns env value when present", func(t *testing.T) {
		t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:1420")

		loader := &Loader{}
		if got := loader.LoadEnvOr("CORS_ALLOWED_ORIGINS", "fallback"); got != "http://localhost:1420" {
			t.Fatalf("LoadEnvOr() = %q, want %q", got, "http://localhost:1420")
		}
	})

	t.Run("returns fallback when missing", func(t *testing.T) {
		loader := &Loader{}
		if got := loader.LoadEnvOr("MISSING_ENV_KEY", "fallback"); got != "fallback" {
			t.Fatalf("LoadEnvOr() = %q, want %q", got, "fallback")
		}
	})

	t.Run("returns fallback when env is empty", func(t *testing.T) {
		t.Setenv("MONGO_DB", "")

		loader := &Loader{}
		if got := loader.LoadEnvOr("MONGO_DB", "anomaly"); got != "anomaly" {
			t.Fatalf("LoadEnvOr() = %q, want %q", got, "anomaly")
		}
	})
}

func TestFindProjectRoot(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/test\n")
	nestedDir := filepath.Join(tmpDir, "nested", "deeper")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested dir: %v", err)
	}

	restoreWD := chdirForTest(t, nestedDir)
	defer restoreWD()

	got, err := findProjectRoot()
	if err != nil {
		t.Fatalf("findProjectRoot() error = %v", err)
	}
	if got != tmpDir {
		t.Fatalf("findProjectRoot() = %q, want %q", got, tmpDir)
	}
}

func testLogger() *zerolog.Logger {
	logger := zerolog.New(io.Discard)
	return &logger
}

func chdirForTest(t *testing.T, dir string) func() {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
	return func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
