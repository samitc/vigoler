package vigoler

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ServerStopSendDataError = errors.New("server stop send data")

type FFmpegWrapper struct {
	ffmpeg                        externalApp
	ffprobe                       externalApp
	maxSecondsWithoutOutputToStop int
	ignoreHttpReuseErros          bool
}
type DownloadCallback func(url string, setting DownloadSettings, output string)
type DownloadSettings struct {
	MaxSizeInKb         int
	SizeSplitThreshold  int
	MaxTimeInSec        int
	TimeSplitThreshold  int
	CallbackBeforeSplit DownloadCallback
	returnWaitError     bool
}
type FFmpegState func(sizeInKb, timeInSeconds int)
type ffmpegWaitAble struct {
	*commandWaitAble
}
type UnsupportedSeekError struct {
}

func (*UnsupportedSeekError) Error() string {
	return "Unsupported seek"
}

type WaitError struct {
	err error
}

func (w *WaitError) Error() string {
	return w.err.Error()
}
func (fwa *ffmpegWaitAble) Stop() error {
	err := fwa.cmd.Process.Signal(os.Interrupt)
	if err != nil {
		return fwa.commandWaitAble.Stop()
	}
	time.Sleep(time.Second)
	return fwa.cmd.Process.Signal(os.Interrupt)
}
func CreateFfmpegWrapper(maxSecondsWithoutOutputToStop int, ignoreHttpReuseErrors bool) FFmpegWrapper {
	return FFmpegWrapper{ffmpeg: externalApp{"ffmpeg"}, ffprobe: externalApp{"ffprobe"}, maxSecondsWithoutOutputToStop: maxSecondsWithoutOutputToStop, ignoreHttpReuseErros: ignoreHttpReuseErrors}
}
func timeStringToInt(s string) int {
	return int((s[0]-'0')*10 + s[1] - '0')
}
func timeToSeconds(time string) int {
	return timeStringToInt(time[6:8]) + 60*(timeStringToInt(time[3:5])+60*timeStringToInt(time[:2]))
}
func addFfmpegHeaders(args []string, headers map[string]string) []string {
	tempArgs := make([]string, 0, 2)
	if headers != nil {
		tempArgs = append(tempArgs, "-headers")
		headerArg := ""
		for k, v := range headers {
			headerArg += fmt.Sprintf("%s:%s\r\n", k, v)
		}
		tempArgs = append(tempArgs, headerArg)
	}
	return append(tempArgs, args...)
}
func processData(line string, sizeIndex int) (time, size int) {
	splits := strings.Split(line, "=")
	sizeStr := splits[sizeIndex]
	numberEnd := strings.Index(sizeStr, "k")
	numberStart := strings.LastIndex(sizeStr[:numberEnd], " ") + 1
	size, _ = strconv.Atoi(sizeStr[numberStart:numberEnd])
	time = timeToSeconds(splits[sizeIndex+1])
	return
}
func runFFmpeg(ffmpeg *externalApp, returnWaitError bool, lineCallback func(string) bool, finishCallback func(waitError error), args ...string) (WaitAble, *Async, error) {
	wa, oChan, err := ffmpeg.runCommandRead(context.Background(), !returnWaitError, args...)
	if err != nil {
		return nil, nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, wa)
	go func() {
		defer wg.Done()
		fullS := ""
		var s string
		var toContinue = true
		for s = range oChan {
			if toContinue {
				fullS, s = extractLineFromString(fullS + s)
				for s != "" {
					toContinue = lineCallback(s)
					if !toContinue {
						break
					}
					fullS, s = extractLineFromString(fullS)
				}
			}
		}
		for toContinue && fullS != "" {
			fullS, s = extractLineFromString(fullS)
			toContinue = lineCallback(s)
		}
		var err error
		if returnWaitError {
			err = wa.Wait()
			if err != nil {
				err = &WaitError{err: err}
			}
		}
		finishCallback(err)
	}()
	return &ffmpegWaitAble{wa.(*commandWaitAble)}, &async, nil
}
func isLineContainsHttpReuseError(line string) bool {
	return strings.Contains(line, "Cannot reuse HTTP connection for different host: ") || strings.Contains(line, "keepalive request failed for ")
}
func (ff *FFmpegWrapper) runFFmpeg(statsCallback FFmpegState, output string, returnWaitError bool, args ...string) (WaitAble, *Async, error) {
	warn := ""
	downloadStarted := false
	var async *Async
	var wa WaitAble
	var curFuncTime *time.Timer = nil
	var err error
	var stopError error
	// ffmpeg command template: ffmpeg -v warning -stats [args] -map_metadata 0 -c copy {output}
	finalArgs := make([]string, 0, 3+len(args))
	finalArgs = append(finalArgs, "-v", "warning", "-stats")
	finalArgs = append(finalArgs, args...)
	finalArgs = append(finalArgs, "-map_metadata", "0", "-c", "copy", output)
	wa, async, err = runFFmpeg(&ff.ffmpeg, returnWaitError, func(line string) bool {
		// Two different message can be here (one for video and one for audio)
		// video - frame= 2039 fps=161 q=-1.0 Lsize=   10808kB time=00:01:07.96 bitrate=1302.7kbits/s speed=5.36x
		// audio - size=    8553kB time=00:09:11.75 bitrate= 127.0kbits/s speed=3.37e+03x
		isVideo := strings.HasPrefix(line, "frame=")
		isAudio := strings.HasPrefix(line, "size=")
		if isVideo || isAudio {
			downloadStarted = true
			if curFuncTime != nil {
				curFuncTime.Reset(time.Duration(ff.maxSecondsWithoutOutputToStop) * time.Second)
			}
			if statsCallback != nil {
				startIndex := 0
				if isVideo {
					startIndex = 4
				}
				timeInSec, sizeInKb := processData(line, startIndex)
				statsCallback(sizeInKb, timeInSec)
			}
		} else {
			if !ff.ignoreHttpReuseErros || !isLineContainsHttpReuseError(line) {
				warn += line
			}
		}
		return true
	}, func(err error) {
		if !downloadStarted {
			if err == nil {
				err = errors.New("Unknown error in ffmpeg")
			}
		} else if stopError != nil {
			err = stopError
		}
		async.SetResult(nil, err, warn)
	}, finalArgs...)
	funcTime := func() {
		_ = wa.Stop()
		stopError = ServerStopSendDataError
	}
	if ff.maxSecondsWithoutOutputToStop != -1 {
		curFuncTime = time.AfterFunc(time.Duration(ff.maxSecondsWithoutOutputToStop)*time.Second, funcTime)
	}
	return wa, async, err
}
func (ff *FFmpegWrapper) Merge(output string, input ...string) (*Async, error) {
	// [-i {input}]
	finalArgs := make([]string, 0, len(input)*2+3)
	for _, i := range input {
		finalArgs = append(finalArgs, "-i", i)
	}
	_, async, err := ff.runFFmpeg(nil, output, false, finalArgs...)
	return async, err
}
func (ff *FFmpegWrapper) download(url string, setting DownloadSettings, output string, headers map[string]string, inputArgs ...string) (*Async, error) {
	if len(url) == 0 {
		return nil, &ArgumentError{stackTrack: debug.Stack(), argName: "url", argValue: url}
	}
	const kbToByte = 1024
	var statsCallback FFmpegState
	args := append(inputArgs, "-i", url)
	if setting.CallbackBeforeSplit != nil && (setting.SizeSplitThreshold > 0 || setting.TimeSplitThreshold > 0) {
		if setting.SizeSplitThreshold <= 0 {
			setting.SizeSplitThreshold = setting.MaxSizeInKb
		}
		if setting.TimeSplitThreshold <= 0 {
			setting.TimeSplitThreshold = setting.MaxTimeInSec
		}
		isAlreadyCalled := false
		statsCallback = func(sizeInKb, timeInSec int) {
			if !isAlreadyCalled && (timeInSec > setting.TimeSplitThreshold || sizeInKb > setting.SizeSplitThreshold) {
				go setting.CallbackBeforeSplit(url, setting, output)
				isAlreadyCalled = true
			}
		}
	}
	if setting.MaxTimeInSec > 0 {
		args = append(args, "-t", strconv.Itoa(setting.MaxTimeInSec))
	}
	if setting.MaxSizeInKb > 0 {
		args = append(args, "-fs", strconv.Itoa(setting.MaxSizeInKb*kbToByte))
	}
	args = addFfmpegHeaders(args, headers)
	_, async, err := ff.runFFmpeg(statsCallback, output, setting.returnWaitError, args...)
	return async, err
}
func (ff *FFmpegWrapper) DownloadSplitHeaders(url string, setting DownloadSettings, output string, headers map[string]string) (*Async, error) {
	return ff.download(url, setting, output, headers)
}
func (ff *FFmpegWrapper) DownloadSplit(url string, setting DownloadSettings, output string) (*Async, error) {
	return ff.download(url, setting, output, nil)
}
func (ff *FFmpegWrapper) Download(url, output string) (*Async, error) {
	return ff.download(url, DownloadSettings{}, output, nil)
}
func (ff *FFmpegWrapper) DownloadHeaders(url string, headers map[string]string, output string) (*Async, error) {
	return ff.download(url, DownloadSettings{}, output, headers)
}
func (ff *FFmpegWrapper) getInputSize(url string, headers map[string]string) (*Async, error) {
	args := []string{"-v", "error", "-show_entries", "format=size", "-of", "default=noprint_wrappers=1:nokey=1", url}
	args = addFfmpegHeaders(args, headers)
	wa, _, oChan, err := ff.ffprobe.runCommand(context.Background(), true, true, true, args...)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, wa)
	go func() {
		defer wg.Done()
		var sizeInBytes int
		var err error
		bytes2KB := 1.0 / 1024
		newLineLength := len("\r\n")
		for s := range oChan {
			sizeInBytes, err = strconv.Atoi(s[:len(s)-newLineLength])
		}
		async.SetResult((int)((float64)(sizeInBytes)*bytes2KB), err, "")
	}()
	return &async, nil
}
func (ff *FFmpegWrapper) GetInputSize(url string) (*Async, error) {
	return ff.getInputSize(url, nil)
}
func (ff *FFmpegWrapper) GetInputSizeHeaders(url string, headers map[string]string) (*Async, error) {
	return ff.getInputSize(url, headers)
}

type ffmpegLiveUntilNowWa struct {
	isStopped bool
	wg        *sync.WaitGroup
	dAsync    *Async
}

func (wa *ffmpegLiveUntilNowWa) Wait() error {
	if wa.dAsync != nil {
		_, err, _ := wa.dAsync.Get()
		return err
	} else {
		wa.wg.Wait()
		return nil
	}
}

func (wa *ffmpegLiveUntilNowWa) Stop() error {
	wa.isStopped = true
	if wa.dAsync != nil {
		return wa.dAsync.Stop()
	}
	return nil
}
func checkIsSeekable(liveDesc string) bool {
	const maxToNotSeekable = 100
	n := strings.Count(liveDesc, "\n")
	return n > maxToNotSeekable
}
func countLength(liveDesc string) (float64, error) {
	l := 0.0
	const lengthMark = "#EXTINF:"
	for {
		curMarkIndex := strings.Index(liveDesc, lengthMark)
		if curMarkIndex == -1 {
			return l, nil
		}
		liveDesc = liveDesc[curMarkIndex+len(lengthMark):]
		curL, err := strconv.ParseFloat(liveDesc[:strings.Index(liveDesc, ",")], 64)
		if err != nil {
			return l, nil
		}
		l += curL
	}
}
func (ff *FFmpegWrapper) DownloadLiveUntilNow(url string, output string) (*Async, error) {
	var wg sync.WaitGroup
	wa := ffmpegLiveUntilNowWa{wg: &wg, isStopped: false}
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, &wa)
	go func() {
		defer wg.Done()
		resp, err := http.DefaultClient.Get(url)
		if err != nil {
			async.SetResult(nil, err, "")
		} else {
			defer resp.Body.Close()
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				async.SetResult(nil, err, "")
			} else {
				stringData := string(data)
				if !checkIsSeekable(stringData) {
					async.SetResult(nil, &UnsupportedSeekError{}, "")
				} else {
					maxTime, err := countLength(stringData)
					if err != nil {
						async.SetResult(nil, err, "")
					} else if !wa.isStopped {
						wa.dAsync, err = ff.download(url, DownloadSettings{MaxTimeInSec: int(maxTime) + 60}, output, nil, "-live_start_index", "0")
						if err != nil {
							async.SetResult(nil, err, "")
						} else {
							_, err, warn := wa.dAsync.Get()
							async.SetResult(nil, err, warn)
						}
					}
				}
			}
		}
	}()
	return &async, nil
}
