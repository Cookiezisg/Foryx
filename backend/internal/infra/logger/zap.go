// Package logger provides the project-wide zap logger factory.
//
// Package logger 提供项目级 zap logger 工厂。
package logger

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// New builds the project zap logger: a console core (dev=true → colored console, else
// JSON to stderr) TEE'd with a rotating JSON file at <logDir>/forgify.log — the desktop
// app's support story: "send me the log file" must always be answerable, a windowed
// app's stdout goes nowhere. Empty logDir keeps console-only (tests).
//
// New 构造项目 zap logger：控制台 core（dev=true 彩色控制台，否则 stderr JSON）TEE 上
// <logDir>/forgify.log 的轮转 JSON 文件——桌面 app 的报障故事：「把日志文件发我」必须永远
// 可答，窗口化 app 的 stdout 没人看得见。logDir 为空则只留控制台（测试）。
func New(dev bool, logDir string) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if dev {
		level = zapcore.DebugLevel
	}

	var consoleEnc zapcore.Encoder
	if dev {
		ec := zap.NewDevelopmentEncoderConfig()
		ec.EncodeLevel = zapcore.CapitalColorLevelEncoder
		consoleEnc = zapcore.NewConsoleEncoder(ec)
	} else {
		ec := zap.NewProductionEncoderConfig()
		ec.TimeKey = "time"
		ec.EncodeTime = zapcore.ISO8601TimeEncoder
		consoleEnc = zapcore.NewJSONEncoder(ec)
	}
	cores := []zapcore.Core{
		zapcore.NewCore(consoleEnc, zapcore.Lock(os.Stderr), level),
	}

	if logDir != "" {
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return nil, fmt.Errorf("logger: mkdir log dir: %w", err)
		}
		fileEC := zap.NewProductionEncoderConfig()
		fileEC.TimeKey = "time"
		fileEC.EncodeTime = zapcore.ISO8601TimeEncoder
		fileSink := zapcore.AddSync(&lumberjack.Logger{
			Filename:   filepath.Join(logDir, "forgify.log"),
			MaxSize:    10, // MB per file before rotation. 单文件轮转阈值（MB）。
			MaxBackups: 3,
			MaxAge:     28, // days. 天。
			Compress:   true,
		})
		cores = append(cores, zapcore.NewCore(zapcore.NewJSONEncoder(fileEC), fileSink, level))
	}

	return zap.New(zapcore.NewTee(cores...)), nil
}
