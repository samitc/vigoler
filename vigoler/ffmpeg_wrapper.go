package vigoler

import (
	"context"
)

type FFmpegWrapper struct {
	app externalApp
}

func CreateFfmpegWrapper() FFmpegWrapper {
	return FFmpegWrapper{app: externalApp{"ffmpeg"}}
}
func (ff *FFmpegWrapper) Merge(output string, input ...string) (*Async, error) {
	finalArgs := make([]string, 0, len(input)*2+3)
	for _, i := range input {
		finalArgs = append(finalArgs, "-i", i)
	}
	finalArgs = append(finalArgs, "-c", "copy", output)
	wait, err := ff.app.runCommandWait(context.Background(), finalArgs...)
	if err != nil {
		return nil, err
	}
	async := createAsyncWaitable(wait)
	return &async, nil
}
