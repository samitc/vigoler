package main

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type logger struct {
	logger *zap.Logger
}

func (l *logger) withVideo(vid *video) *zap.Logger {
	return l.logger.With(zap.Any("video", vid))
}
func (l *logger) logError(message string, err error) {
	l.logger.Error(message, zap.Error(err))
}
func (l *logger) deleteVideo(vid *video) {
	l.logger.Info("Delete video", zap.Any("video", vid))
}
func (l *logger) deleteVideoError(vid *video, warn string, err error) {
	l.logger.Error("Error on delete video", zap.Any("video", vid), zap.String("warn", warn), zap.Error(err))
}
func (l *logger) stopVideoError(vid *video, err error) {
	l.logger.Error("Error on stop video to delete", zap.Any("video", vid), zap.Error(err))
}
func (l *logger) deleteVideoFileError(vid *video, err error) {
	l.logger.Error("Error on delete video file", zap.Any("video", vid), zap.Error(err))
}
func (l *logger) logVideoFinish(vid *video, warn string, err error) {
	const message = "Video finish to download"
	video := zap.Any("video", vid)
	warnF := zap.String("warn", warn)
	if err == nil {
		l.logger.Info(message, video, warnF)
	} else {
		l.logger.Error(message, video, warnF, zap.Error(err))
	}
}
func (l *logger) newVideo(vid *video) {
	l.logger.Info("New video created", zap.Any("video", vid))
}
func (l *logger) videoAsyncError(vid *video, err error, warn string) {
	l.logger.Error("Error while downloading video", zap.Any("video", vid), zap.String("warn", warn), zap.Error(err))
}
func (l *logger) errorOpenVideoOutputFile(vid *video, filePath string, err error) {
	l.logger.Error("Error opening video output file", zap.Any("video", vid), zap.String("output", filePath), zap.Error(err))
}
func (l *logger) warnInVideoCreate(url string, warn string) {
	l.logger.Warn("Warning while creating videos", zap.String("url", url), zap.String("warn", warn))
}
func (l *logger) errorInVideoCreate(url string, err error) {
	l.logger.Error("Error while creating videos", zap.String("url", url), zap.Error(err))
}
func (l *logger) downloadVideoError(vid *video, downloadType string, err error) {
	l.logger.Error("Error while downloading video", zap.Any("video", vid), zap.String("type", downloadType), zap.Error(err))
}
func (l *logger) startDownloadVideo(vid *video) {
	l.logger.Info("Start downloading video", zap.Any("video", vid))
}
func (v video) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("ID", v.ID)
	enc.AddString("name", v.Name)
	enc.AddBool("is_live", v.IsLive)
	if v.Ids != nil {
		err := enc.AddArray("ids", stringArray(v.Ids))
		if err != nil {
			return err
		}
	}
	if v.parentID != "" {
		enc.AddString("parent_id", v.parentID)
	}
	if v.fileName != "" {
		enc.AddString("output", v.fileName)
	}
	if v.ext != "" {
		enc.AddString("extension", v.ext)
	}
	enc.AddTime("update_time", v.updateTime)
	return enc.AddObject("video_url", &v.videoURL)
}

type stringArray []string

func (ss stringArray) MarshalLogArray(arr zapcore.ArrayEncoder) error {
	for i := range ss {
		arr.AppendString(ss[i])
	}
	return nil
}
