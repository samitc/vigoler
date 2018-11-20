package vigoler

import (
	"bufio"
	"context"
	"io"
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

func CreateFfmpegWrapper() FFmpegWrapper {
	return FFmpegWrapper{app: externalApp{"ffmpeg"}}
}
func beforeStartWork(line string) bool {
	return line[:len(line)-2] == "Press [q] to stop, [?] for help"
}
func (ff *FFmpegWrapper) Merge(output string, input ...string) (*Async, error) {
	finalArgs := make([]string, 0, len(input)*2+3)
	for _, i := range input {
		finalArgs = append(finalArgs, "-i", i)
	}
	finalArgs = append(finalArgs, "-c", "copy", output)
	wa, cOutput, err := ff.app.runCommandChan(context.Background(), finalArgs...)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, wa)
	go func(async *Async, fileOutput string, output *<-chan string) {
		warn := ""
		defer async.wg.Done()
		for s := range *output {
			if strings.Contains(s, "[") {
				if !beforeStartWork(s) && strings.Index(s, "Stream #0") == -1 {
					warn += s
				}
			}
		}
		if warn != "" {
			warn = "WARNING WHEN MERGE " + fileOutput + ":" + warn
		}
		async.SetResult(nil, nil, warn)
	}(&async, output, &cOutput)
	return &async, nil
}
func (ff *FFmpegWrapper) Download(url string, setting DownloadSettings, output string) (*Async, error) {
	const KB_TO_BYTE = 1024
	url = url[:len(url)-1]
	needCommandReader := setting.CallbackBeforeSplit != nil && (setting.SizeSplitThreshold > 0 || setting.TimeSplitThreshold > 0)
	if setting.SizeSplitThreshold <= 0 {
		setting.SizeSplitThreshold = setting.MaxSizeInKb
	}
	if setting.TimeSplitThreshold <= 0 {
		setting.TimeSplitThreshold = setting.MaxTimeInSec
	}
	args := []string{"-i", url, "-c", "copy"}
	if setting.MaxTimeInSec > 0 {
		args = append(args, "-t", strconv.Itoa(setting.MaxTimeInSec))
	}
	if setting.MaxSizeInKb > 0 {
		args = append(args, "-fs", strconv.Itoa(setting.MaxSizeInKb*KB_TO_BYTE))
	}
	args = append(args, output)
	waitAble, reader, _, err := ff.app.runCommand(context.Background(), false, !needCommandReader, args...)
	if err != nil {
		return nil, err
	}
	var async Async
	if !needCommandReader {
		async = createAsyncWaitAble(waitAble)
	} else {
		var wg sync.WaitGroup
		wg.Add(1)
		async = CreateAsyncWaitGroup(&wg, waitAble)
		go func(async *Async, reader io.ReadCloser, setting *DownloadSettings) {
			defer async.wg.Done()
			defer reader.Close()
			timeStringToInt := func(s string) int {
				return int((s[0]-'0')*10 + s[1] - '0')
			}
			processData := func(line string) (time, size int) {
				if strings.Index(line, "frame") != -1 {
					splits := strings.Split(line, "=")
					sizeStr := splits[4]
					numberEnd := strings.Index(sizeStr, "k")
					numberStart := strings.LastIndex(sizeStr[:numberEnd], " ") + 1
					size, _ = strconv.Atoi(sizeStr[numberStart:numberEnd])
					timeStr := splits[5]
					time = timeStringToInt(timeStr[6:8]) + 60*(timeStringToInt(timeStr[3:5])+60*timeStringToInt(timeStr[:2]))
				}
				return
			}
			rd := bufio.NewReader(reader)
			var delim byte = '\n'
			line, err := rd.ReadString(delim)
			isFinish := false
			for ; err == nil; line, err = rd.ReadString(delim) {
				if !isFinish {
					if delim == '\n' {
						if beforeStartWork(line) {
							delim = '\r'
						}
					} else {
						time, size := processData(line)
						if time > setting.TimeSplitThreshold || size > setting.SizeSplitThreshold {
							go setting.CallbackBeforeSplit(url, *setting, output)
							isFinish = true
						}
					}
				}
			}
			if err != io.EOF {
				panic(err)
			}
		}(&async, reader, &setting)
	}
	return &async, nil
}
