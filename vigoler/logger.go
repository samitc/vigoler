package vigoler

import "go.uber.org/zap"

type Logger struct {
	Logger *zap.Logger
}

func (l *Logger) startDownloadLive(url VideoUrl, output string) {
	l.Logger.Info("Start downloading live", zap.Any("url", url), zap.String("output", output))
}
func (l *Logger) liveRecreated(url VideoUrl, output string) {
	l.Logger.Info("Recreate live to download", zap.Any("url", url), zap.String("output", output))
}
func (l *Logger) finishDownloadLive(url VideoUrl, output string) {
	l.Logger.Info("Finish downloading live", zap.Any("url", url), zap.String("output", output))
}
