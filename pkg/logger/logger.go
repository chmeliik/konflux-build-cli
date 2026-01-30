package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

var Logger = logrus.New()

func InitLogger(logLevel string) error {
	Logger.SetOutput(os.Stderr)
	Logger.SetFormatter(&logrus.TextFormatter{
		EnvironmentOverrideColors: true,
	})

	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		return err
	}
	Logger.SetLevel(level)

	return nil
}
