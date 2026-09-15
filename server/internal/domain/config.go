package domain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	RustFSConfig     *RustFSConfig
	RedisConfig      *RedisConfig
	SMTPConfig       *SMTPConfig
}

func (l *Loader) LoadAllConfig() *Config {
	return &Config{
		MongoConfig:      l.LoadMongoConfig(),
		EventStoreConfig: l.LoadEventStoreConfig(),
		AuthConfig:       l.LoadAuthConfig(),
		RustFSConfig:     l.LoadRustFSConfig(),
		RedisConfig:      l.LoadRedisConfig(),
		SMTPConfig:       l.LoadSMTPConfig(),
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
		EventStoreConnString: l.LoadEnvOr("EVENT_STORE_CONN_STRING", "esdb://localhost:2113?tls=false"),
	}
}

// gom dữ liệu
type RustFSConfig struct {
	Endpoint  string // địa chỉ đang chạy
	AccessKey string // usename
	SecretKey string // pass
	Bucket    string // chứa file media
	Region    string
}

// đọc dữ liệu từ env file và trả về cấu hình RustFSConfig
func (l *Loader) LoadRustFSConfig() *RustFSConfig {
	l.logger().Info().Msg("Load rustfs config")
	return &RustFSConfig{
		Endpoint:  l.LoadEnvOr("RUSTFS_ENDPOINT", "http://localhost:9000"),
		AccessKey: l.LoadEnvOr("RUSTFS_ACCESS_KEY", "rustfsadmin"),
		SecretKey: l.LoadEnvOr("RUSTFS_SECRET_KEY", "rustfsadmin"),
		Bucket:    l.LoadEnvOr("RUSTFS_BUCKET", "media"),
		Region:    l.LoadEnvOr("RUSTFS_REGION", "us-east-1"),
	}
}

type RedisConfig struct {
	Addr     string
	Password string
}

func (l *Loader) LoadRedisConfig() *RedisConfig {
	l.logger().Info().Msg("Load redis config")
	return &RedisConfig{
		Addr:     l.LoadEnvOr("REDIS_ADDR", "localhost:6379"),
		Password: l.LoadEnvOr("REDIS_PASSWORD", ""),
	}
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

const defaultSMTPPort = 587

func (l *Loader) LoadSMTPConfig() *SMTPConfig {
	l.logger().Info().Msg("Load smtp config")
	return &SMTPConfig{
		Host:     l.LoadEnvOr("SMTP_HOST", ""),
		Port:     l.LoadEnvPort("SMTP_PORT", defaultSMTPPort),
		Username: l.LoadEnvOr("SMTP_USERNAME", ""),
		Password: l.LoadEnvOr("SMTP_PASSWORD", ""),
		From:     l.LoadEnvOr("SMTP_FROM", ""),
	}
}

// SenderAddress trả về địa chỉ gửi mail: ưu tiên From, fallback Username.
func (c *SMTPConfig) SenderAddress() string {
	if c.From != "" {
		return c.From
	}
	return c.Username
}

// Configured cho biết SMTP đã đủ cấu hình tối thiểu để gửi mail chưa.
func (c *SMTPConfig) Configured() bool {
	return c.Host != ""
}

func (l *Loader) LoadEnvPort(key string, fallback int) int {
	val, exists := os.LookupEnv(key)
	if !exists || val == "" {
		return fallback
	}
	port, err := strconv.Atoi(val)
	if err != nil || port <= 0 {
		l.logger().Warn().Err(err).Str("key", key).Msg("invalid port, using fallback")
		return fallback
	}
	return port
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

const minJWTSecretLength = 32

func (l *Loader) LoadAuthConfig() *AuthConfig {
	l.logger().Info().Msg("Load auth config")
	secret := l.LoadEnv("JWT_SECRET")
	if secret == "" {
		l.logger().Fatal().Msg("JWT_SECRET must not be empty")
	}
	if len(secret) < minJWTSecretLength {
		l.logger().Fatal().Int("minLength", minJWTSecretLength).Int("actualLength", len(secret)).
			Msg("JWT_SECRET must be at least 32 characters to resist brute-force attacks")
	}
	return &AuthConfig{
		JWTSecret: secret,
		JWTExpiry: l.LoadEnvDuration("JWT_EXPIRY", 24*time.Hour),
	}
}

func (l *Loader) LoadEnvDuration(key string, fallback time.Duration) time.Duration {
	val, exists := os.LookupEnv(key)
	if !exists || val == "" {
		return fallback
	}
	d, err := time.ParseDuration(val)
	if err != nil || d <= 0 {
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
