package logger

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

func NewLogger(level zerolog.Level) *zerolog.Logger {
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		NoColor:    false,
		TimeFormat: "2006-01-02 15:04:05",
		FormatCaller: func(i any) string {
			path := fmt.Sprintf("%s", i)
			parts := strings.Split(path, "/")
			if len(parts) > 2 {
				path = strings.Join(parts[len(parts)-3:], "/")
			}
			return fmt.Sprintf("[%s]", path)
		},
		FormatMessage: func(i any) string {
			return fmt.Sprintf("{%-20s}", i)
		},
	}

	logger := zerolog.New(consoleWriter).Level(level).With().Caller().Timestamp().Logger()
	return &logger
}
