package domain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

type Loader struct {
	mu     sync.RWMutex
	loaded bool
	log    *zerolog.Logger
}

var loader Loader

func GetEnvLoader() *Loader {
	return &loader
}

func (l *Loader) Load(log *zerolog.Logger) *Loader {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.loaded {
		return l
	}

	l.log = log
	projectRoot, err := findProjectRoot()
	if err != nil {
		fatalf(log, "failed to get current working directory: %v", err)
	}

	envPath := filepath.Join(projectRoot, ".env")

	if err := godotenv.Load(envPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		fatalf(log, "load env file %q: %v", envPath, err)
	}

	l.loaded = true
	return l
}

func (l *Loader) logger() *zerolog.Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.log != nil {
		return l.log
	}
	nop := zerolog.Nop()
	return &nop
}

type Config struct {
	MongoConfig      *MongoConfig
	EventStoreConfig *EventStoreConfig
	RustFSConfig     *RustFSConfig
}

func (l *Loader) LoadAllConfig() *Config {
	return &Config{
		MongoConfig:      l.LoadMongoConfig(),
		EventStoreConfig: l.LoadEventStoreConfig(),
		RustFSConfig:     l.LoadRustFSConfig(),
	}
}

type MongoConfig struct {
	MongoURI    string
	MongoDBName string
}

func (l *Loader) LoadMongoConfig() *MongoConfig {
	l.logger().Info().Msg("Load mongo config")
	return &MongoConfig{
		MongoURI:    l.LoadEnvOr("MONGO_URI", "mongodb://localhost:27017"),
		MongoDBName: l.LoadEnvOr("MONGO_DB", "anomaly"),
	}
}

type EventStoreConfig struct {
	EventStoreConnString string
}

func (l *Loader) LoadEventStoreConfig() *EventStoreConfig {
	l.logger().Info().Msg("Load event store config")
	return &EventStoreConfig{
		EventStoreConnString: l.LoadEnvOr("EVENT_STORE_CONN_STRING", "kurrentdb://localhost:2113?tls=false"),
	}
}

// gom dữ liệu
type RustFSConfig struct {
	Endpoint  string // địa chỉ đang chạy
	AccessKey string // usename
	SecretKey string // pass
	Bucket    string // chứa file media
	UseSSL    bool   // true nếu endpoint là https, false nếu là http
}

// đọc dữ liệu từ env file và trả về cấu hình RustFSConfig
func (l *Loader) LoadRustFSConfig() *RustFSConfig {
	l.logger().Info().Msg("Load rustfs config")
	return &RustFSConfig{
		Endpoint:  l.LoadEnvOr("RUSTFS_ENDPOINT", "http://localhost:9000"),
		AccessKey: l.LoadEnvOr("RUSTFS_ACCESS_KEY", "rustfsadmin"),
		SecretKey: l.LoadEnvOr("RUSTFS_SECRET_KEY", "rustfsadmin"),
		Bucket:    l.LoadEnvOr("RUSTFS_BUCKET", "media"),
		UseSSL:    l.LoadEnvOr("RUSTFS_USE_SSL", "false") == "true", // này ko gọi từ .env thì dùng giá trị mặc đinhj false
	}
}

func (l *Loader) LoadEnv(key string) string {
	val, exists := os.LookupEnv(key)
	if !exists {
		l.logger().Fatal().Msgf("key %s do not exist in env file", key)
	}
	return val
}

func (l *Loader) LoadEnvOr(key, fallback string) string {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		return val
	}
	return fallback
}

func fatalf(log *zerolog.Logger, format string, args ...any) {
	if log != nil {
		log.Fatal().Msgf(format, args...)
		return
	}

	nop := zerolog.Nop()
	nop.Fatal().Msgf(format, args...)
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil {
			return dir, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat go.mod in %s: %w", dir, err)
		}
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			return "", fmt.Errorf("could not find project root")
		}
		dir = parentDir
	}
}
