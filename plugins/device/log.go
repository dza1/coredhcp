package device

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
)

func InitLogger(loggingConfig LoggingConfig) error {
	lumberjackLogger := &lumberjack.Logger{
		Filename:   loggingConfig.Filename,
		MaxSize:    loggingConfig.MaxSize,
		MaxBackups: loggingConfig.MaxBackups,
		MaxAge:     loggingConfig.MaxAge,
		Compress:   loggingConfig.Compress,
	}

	logWriter := io.MultiWriter(os.Stdout, lumberjackLogger)
	log.SetOutput(logWriter)

	level, err := log.ParseLevel(loggingConfig.LogLevel)
	if err != nil {
		return fmt.Errorf("parse log level: %v", err)
	}
	log.SetLevel(level)
	return nil
}
