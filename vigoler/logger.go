package vigoler

import "go.uber.org/zap"

type Logger struct {
	Logger *zap.Logger
}

func (l *Logger) startDownloadLive(url VideoUrl, output string) {
	l.Logger.Info("Start downloading live", zap.Any("video_url", url), zap.String("output", output))
}
func (l *Logger) liveRecreated(url VideoUrl, output string) {
	l.Logger.Info("Recreate live to download", zap.Any("video_url", url), zap.String("output", output))
}
func (l *Logger) liveRecreatedError(url VideoUrl, err error) {
	l.Logger.Error("Recreate live to download", zap.Any("video_url", url), zap.Error(err))
}
func (l *Logger) finishDownloadLive(url VideoUrl, output string, warn string, err error) {
	l.Logger.Info("Finish downloading live", zap.Any("video_url", url), zap.String("output", output), zap.String("warn", warn), zap.Error(err))
}
func (l *Logger) finishDownloadLiveError(url VideoUrl, output string, warn string, err error) {
	l.Logger.Error("Finish downloading live", zap.Any("video_url", url), zap.String("output", output), zap.String("warn", warn), zap.Error(err))
}

func (l *Logger) liveDownloadError(url VideoUrl, output string, err error) {
	l.Logger.Error("Start downloading live", zap.Any("video_url", url), zap.String("output", output), zap.Error(err))
}
