package vigoler

import (
	"context"
	"fmt"
	"sync"
)

type FFmpegWrapper struct {
	app externalApp
}
type DownloadSettings struct {
	MaxSizeInKb  int
	MaxTimeInSec int
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
func (ff *FFmpegWrapper) Download(url string, setting DownloadSettings, output string) (*Async, error) {
	url = url[:len(url)-1]
	outChan, err := ff.app.runCommandChan(context.Background(), "-i", url, "-c", "copy", output)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := createAsyncWaitGroup(&wg)
	go func(async *Async, output *<-chan string) {
		defer async.wg.Done()
		for s := range outChan {
			fmt.Println(s)
		}
	}(&async, &outChan)
	return &async, nil
}
