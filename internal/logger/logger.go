package logger

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Log       *zap.Logger
	logFile   *os.File
	stopFlush chan struct{}
)

func Init(path string) error {
	// Открываем файл с очисткой старых данных во избежание захламления диска
	file, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY,
		0666,
	)
	if err != nil {
		return err
	}
	logFile = file

	// Настройка читаемого формата времени (ISO8601) вместо сырых наносекунд
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	encoder := zapcore.NewJSONEncoder(encoderConfig)

	// ИСПРАВЛЕНО: Асинхронный буферизированный писатель.
	// Защищает многопоточный чекер от падения скорости при записи логов на медленных HDD/SSD.
	ws := &zapcore.BufferedWriteSyncer{
		WS:            zapcore.AddSync(file),
		Size:          4096,            // Буфер в 4 КБ
		FlushInterval: 1 * time.Second, // Гарантированный сброс на диск каждую секунду
	}

	core := zapcore.NewCore(
		encoder,
		ws,
		zap.InfoLevel,
	)

	Log = zap.New(core, zap.AddCaller())

	// Запускаем фоновый таймер сброса буфера в ОС
	stopFlush = make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = ws.Sync()
			case <-stopFlush:
				_ = ws.Sync()
				return
			}
		}
	}()

	return nil
}

// Close гарантированно сбрасывает остатки логов из памяти на диск и закрывает дескриптор файла.
// Вызывать через defer logger.Close() в функции main() сразу после logger.Init().
func Close() {
	if stopFlush != nil {
		close(stopFlush)
	}
	if Log != nil {
		_ = Log.Sync()
	}
	if logFile != nil {
		_ = logFile.Close()
	}
}
