package logger

import (
    "log/slog"
    "os"
    "gopkg.in/natefinch/lumberjack.v2"
)

func Setup(ruta string, nivel int) {
    lvl := slog.LevelDebug
    switch nivel {
    case 1: lvl = slog.LevelInfo
    case 2: lvl = slog.LevelWarn
    case 3: lvl = slog.LevelError
    }

    rot := &lumberjack.Logger{
        Filename:   ruta,
        MaxSize:    10, // MB → igual que el README
        MaxBackups: 3,
        Compress:   true,
    }

    handler := slog.NewTextHandler(rot, &slog.HandlerOptions{Level: lvl})
    slog.SetDefault(slog.New(handler))

    // Opcional: también a stdout en modo consola
    if os.Getenv("RELAY_CONSOLE") == "1" {
        multi := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
        slog.SetDefault(slog.New(slog.NewMultiHandler(handler, multi)))
    }
}