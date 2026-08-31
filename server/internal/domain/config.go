package domain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

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
	AuthConfig       *AuthConfig
}

func (l *Loader) LoadAllConfig() *Config {
	return &Config{
		MongoConfig:      l.LoadMongoConfig(),
		EventStoreConfig: l.LoadEventStoreConfig(),
		AuthConfig:       l.LoadAuthConfig(),
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

type AuthConfig struct {
	JWTSecret string
	JWTExpiry time.Duration
}

func (l *Loader) LoadAuthConfig() *AuthConfig {
	l.logger().Info().Msg("Load auth config")
	return &AuthConfig{
		JWTSecret: l.LoadEnv("JWT_SECRET"),
		JWTExpiry: l.LoadEnvDuration("JWT_EXPIRY", 24*time.Hour),
	}
}

func (l *Loader) LoadEnvDuration(key string, fallback time.Duration) time.Duration {
	val, exists := os.LookupEnv(key)
	if !exists || val == "" {
		return fallback
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		l.logger().Warn().Err(err).Str("key", key).Msg("invalid duration, using fallback")
		return fallback
	}
	return d
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
