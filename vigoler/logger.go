package vigoler

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"reflect"
)

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
func (l *Logger) logLogError(url VideoUrl, msg string, logError LogError) {
	errorLog := logError.LogAttributes()
	logAttributes := make([]zap.Field, 0, len(errorLog)+2)
	logAttributes = append(logAttributes, zap.Any("video_url", url))
	for k, v := range errorLog {
		var field zap.Field
		vType := reflect.TypeOf(v)
		switch vType.Kind() {
		case reflect.Slice, reflect.Array:
			if vType.Elem().Implements(reflect.TypeOf((*zapcore.ObjectMarshaler)(nil)).Elem()) {
				val := reflect.ValueOf(v)
				field = zap.Array(k, formatObjects{value: val})
			} else {
				field = zap.Any(k, v)
			}
		default:
			field = zap.Any(k, v)
		}
		logAttributes = append(logAttributes, field)
	}
	logAttributes = append(logAttributes, zap.Error(logError))
	l.Logger.Error(msg, logAttributes...)
}
func (v VideoUrl) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("ID", v.ID)
	enc.AddString("name", v.Name)
	enc.AddBool("is_live", v.IsLive)
	enc.AddString("web_page_url", v.WebPageURL)
	return enc.AddArray("formats", formatArray(v.Formats))
}
func (f Format) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("id", f.formatID)
	return nil
}

type formatArray []Format

func (f formatArray) MarshalLogArray(enc zapcore.ArrayEncoder) (err error) {
	for _, format := range f {
		err = enc.AppendObject(&format)
		if err != nil {
			return
		}
	}
	return
}

type formatObjects struct {
	value reflect.Value
}

func (f formatObjects) MarshalLogArray(enc zapcore.ArrayEncoder) (err error) {
	for i := 0; i < f.value.Len(); i++ {
		err = enc.AppendObject(f.value.Index(i).Interface().(zapcore.ObjectMarshaler))
		if err != nil {
			return
		}
	}
	return
}
