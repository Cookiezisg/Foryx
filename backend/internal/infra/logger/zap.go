// Package logger provides the project-wide zap logger factory.
//
// Package logger 提供项目级 zap logger 工厂。
package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New builds a zap logger; dev=true → colored console, else JSON; extras tee alongside.
//
// New 构造 zap logger；dev=true 用彩色控制台，否则 JSON；extras 与主 core 并行输出。
func New(dev bool, extras ...zapcore.Core) (*zap.Logger, error) {
	var cfg zap.Config
	if dev {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "time"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	log, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %w", err)
	}
	if len(extras) > 0 {
		allCores := append([]zapcore.Core{log.Core()}, extras...)
		log = log.WithOptions(zap.WrapCore(func(_ zapcore.Core) zapcore.Core {
			return zapcore.NewTee(allCores...)
		}))
	}
	return log, nil
}
