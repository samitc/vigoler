package vigoler

import (
	"context"
	"strconv"
	"strings"
	"sync"
)

type FFmpegWrapper struct {
	app externalApp
}
type DownloadCallback func(url string, setting DownloadSettings, output string)
type DownloadSettings struct {
	MaxSizeInKb         int
	SizeSplitThreshold  int
	MaxTimeInSec        int
	TimeSplitThreshold  int
	CallbackBeforeSplit DownloadCallback
}
type FFmpegState func(sizeInKb, timeInSeconds int)

func CreateFfmpegWrapper() FFmpegWrapper {
	return FFmpegWrapper{app: externalApp{"ffmpeg"}}
}
func beforeStartWork(line string) bool {
	return strings.HasPrefix(line, "Press [q] to stop, [?] for help")
}
func timeStringToInt(s string) int {
	return int((s[0]-'0')*10 + s[1] - '0')
}
func processData(line string) (time, size int) {
	splits := strings.Split(line, "=")
	sizeStr := splits[4]
	numberEnd := strings.Index(sizeStr, "k")
	numberStart := strings.LastIndex(sizeStr[:numberEnd], " ") + 1
	size, _ = strconv.Atoi(sizeStr[numberStart:numberEnd])
	timeStr := splits[5]
	time = timeStringToInt(timeStr[6:8]) + 60*(timeStringToInt(timeStr[3:5])+60*timeStringToInt(timeStr[:2]))
	return
}
func (ff *FFmpegWrapper) runFFmpeg(statsCallback FFmpegState, args ...string) (*Async, error) {
	// ffmpeg command template: ffmpeg -v warning -stats [args]
	finalArgs := make([]string, 0, 3+len(args))
	finalArgs = append(finalArgs, "-v", "warning", "-stats")
	finalArgs = append(finalArgs, args...)
	wa, _, oChan, err := ff.app.runCommand(context.Background(), true, true, false, finalArgs...)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, wa)
	go func() {
		defer wg.Done()
		warn := ""
		fullS := ""
		for s := range oChan {
			fullS, s = extractLineFromString(fullS + s)
			if s != "" {
				if strings.HasPrefix(s, "frame=") {
					if statsCallback != nil {
						timeInSec, sizeInKb := processData(s)
						statsCallback(sizeInKb, timeInSec)
					}
				} else {
					warn += s
				}
			}
		}
		async.SetResult(nil, nil, warn)
	}()
	return &async, nil
}
func (ff *FFmpegWrapper) Merge(output string, input ...string) (*Async, error) {
	// [-i {input}] -c copy output
	finalArgs := make([]string, 0, len(input)*2+3)
	for _, i := range input {
		finalArgs = append(finalArgs, "-i", i)
	}
	finalArgs = append(finalArgs, "-c", "copy", output)
	return ff.runFFmpeg(nil, finalArgs...)
}
func (ff *FFmpegWrapper) Download(url string, setting DownloadSettings, output string) (*Async, error) {
	const kbToByte = 1024
	// Remove suffix new line
	url = url[:len(url)-1]
	var statsCallback FFmpegState
	args := []string{"-i", url, "-c", "copy"}
	if setting.CallbackBeforeSplit != nil && (setting.SizeSplitThreshold > 0 || setting.TimeSplitThreshold > 0) {
		if setting.SizeSplitThreshold <= 0 {
			setting.SizeSplitThreshold = setting.MaxSizeInKb
		}
		if setting.TimeSplitThreshold <= 0 {
			setting.TimeSplitThreshold = setting.MaxTimeInSec
		}
		statsCallback = func(sizeInKb, timeInSec int) {
			if timeInSec > setting.TimeSplitThreshold || sizeInKb > setting.SizeSplitThreshold {
				go setting.CallbackBeforeSplit(url, setting, output)
			}
		}
	}
	if setting.MaxTimeInSec > 0 {
		args = append(args, "-t", strconv.Itoa(setting.MaxTimeInSec))
	}
	if setting.MaxSizeInKb > 0 {
		args = append(args, "-fs", strconv.Itoa(setting.MaxSizeInKb*kbToByte))
	}
	args = append(args, output)
	return ff.runFFmpeg(statsCallback, args...)
}
