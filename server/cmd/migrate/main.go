package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mongodb"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/rs/zerolog"
)

func main() {
	if len(os.Args) != 2 {
		fatalf("usage: migrate <up|up-by-one|down|version>")
	}

	log := logger.NewLogger(zerolog.InfoLevel)
	loader := domain.GetEnvLoader().Load(log)
	databaseURL, err := mongoDatabaseURL(loader)
	if err != nil {
		fatalf("build MongoDB migration URL: %v", err)
	}

	runner, err := migrate.New("file://migrations", databaseURL)
	if err != nil {
		fatalf("create migration runner: %v", err)
	}
	defer func() {
		sourceErr, databaseErr := runner.Close()
		if sourceErr != nil || databaseErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "close migration runner: source=%v database=%v\n", sourceErr, databaseErr)
		}
	}()

	if err := run(runner, os.Args[1]); err != nil {
		fatalf("migration %s failed: %v", os.Args[1], err)
	}
}

func run(runner *migrate.Migrate, command string) error {
	var err error
	switch command {
	case "up":
		err = runner.Up()
	case "up-by-one":
		err = runner.Steps(1)
	case "down":
		err = runner.Steps(-1)
	case "version":
		version, dirty, versionErr := runner.Version()
		if versionErr != nil {
			return versionErr
		}
		fmt.Printf("version=%d dirty=%t\n", version, dirty)
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("no migrations to run")
		return nil
	}
	return err
}

func mongoDatabaseURL(loader *domain.Loader) (string, error) {
	config := loader.LoadMongoConfig()
	parsed, err := url.Parse(config.MongoURI)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "mongodb" && parsed.Scheme != "mongodb+srv" {
		return "", fmt.Errorf("unsupported MongoDB URI scheme %q", parsed.Scheme)
	}
	if strings.TrimSpace(config.MongoDBName) == "" {
		return "", errors.New("MongoDB database name is required")
	}
	parsed.Path = "/" + config.MongoDBName
	return parsed.String(), nil
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
