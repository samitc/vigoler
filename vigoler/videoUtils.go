package vigoler

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type VideoUtils struct {
	Youtube *YoutubeDlWrapper
	Ffmpeg  *FFmpegWrapper
	Curl    *CurlWrapper
	random  *rand.Rand
}
type LiveVideoCallback func(data interface{}, fileName string, async *Async)
type TypedError interface {
	error
	Type() string
}
type FileTooBigError struct {
	url VideoUrl
}

func (e *FileTooBigError) Error() string {
	return fmt.Sprintf("File %s is too big to download", e.url.Name)
}
func (e *FileTooBigError) Type() string {
	return "File too big error"
}
func ValidateFileName(fileName string) string {
	notAllowCh := []string{`\`, `/`, `:`, `|`, `?`, `"`, `*`, `<`, `>`}
	for _, ch := range notAllowCh {
		fileName = strings.Replace(fileName, ch, "", -1)
	}
	return fileName
}
func addIndexToFileName(name string) string {
	lastDot := strings.LastIndex(name, ".")
	preLastDot := strings.LastIndex(name[:lastDot], ".")
	if preLastDot == -1 {
		name = name[:lastDot] + ".1" + name[lastDot:]
	} else {
		num, err := strconv.Atoi(name[preLastDot+1 : lastDot])
		if err != nil {
			name = name[:lastDot] + ".1" + name[lastDot:]
		} else {
			name = name[:preLastDot] + "." + strconv.Itoa(num+1) + name[lastDot:]
		}
	}
	return name
}
func (vu *VideoUtils) chooseDownload(url, output, protocol string) (*Async, error) {
	if protocol == "https" {
		return vu.Curl.Download(url, output)
	}
	return vu.Ffmpeg.Download(url, DownloadSettings{}, output)
}
func (vu *VideoUtils) recreateURL(url VideoUrl, format Format) (Format, error) {
	async, err := vu.Youtube.GetUrls(url.url)
	if err != nil {
		return Format{}, err
	}
	videos, err, _ := async.Get()
	for _, form := range (videos.([]VideoUrl))[url.idInPlaylist].Formats {
		if form.formatID == format.formatID {
			return form, nil
		}
	}
	return Format{}, errors.New("format not found")
}
func (vu *VideoUtils) LiveDownload(url VideoUrl, format Format, outputFile string, maxSizeInKb, sizeSplitThreshold, maxTimeInSec, timeSplitThreshold int, liveVideoCallback LiveVideoCallback, data interface{}) (*Async, error) {
	var wg sync.WaitGroup
	var wa multipleWaitAble
	var lastErr error
	lastWarn := ""
	lData := data
	outputFile += "." + format.Ext
	var lastResult = outputFile
	lLiveVideoCallback := liveVideoCallback
	waitForVideoToDownload := func(fAsync *Async, output string) {
		defer wg.Done()
		_, err, warn := fAsync.Get()
		wa.remove(fAsync)
		if lLiveVideoCallback != nil {
			lLiveVideoCallback(lData, output, fAsync)
		}
		lastErr, lastWarn = err, warn
	}
	downloadVideo := func(setting DownloadSettings, output string) {
		format, err := vu.recreateURL(url, format)
		if err != nil {
			lastErr, lastWarn = err, ""
		} else {
			var fAsync *Async
			fAsync, err = vu.Ffmpeg.Download(format.url, setting, output)
			if err != nil {
				lastErr, lastWarn = err, ""
			} else {
				wg.Add(1)
				wa.add(fAsync)
				waitForVideoToDownload(fAsync, output)
			}
		}
	}
	splitCallback := func(url string, setting DownloadSettings, output string) {
		output = addIndexToFileName(output)
		downloadVideo(setting, output)
	}
	fAsync, err := vu.Ffmpeg.Download(format.url, DownloadSettings{CallbackBeforeSplit: splitCallback, MaxSizeInKb: maxSizeInKb, MaxTimeInSec: maxTimeInSec, SizeSplitThreshold: sizeSplitThreshold, TimeSplitThreshold: timeSplitThreshold}, outputFile)
	if err != nil {
		return nil, err
	}
	var wga sync.WaitGroup
	async := CreateAsyncWaitGroup(&wga, &wa)
	wg.Add(1)
	wa.add(fAsync)
	wga.Add(1)
	go func() {
		wg.Wait()
		async.SetResult(lastResult, lastErr, lastWarn)
		wga.Done()
	}()
	go waitForVideoToDownload(fAsync, outputFile)
	return &async, nil
}
func (vu *VideoUtils) downloadFormat(format Format, output string) (*Async, error) {
	output += "." + format.Ext
	dAsync, err := vu.chooseDownload(format.url, output, format.protocol)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncFromAsyncAsWaitAble(&wg, dAsync)
	go func() {
		defer wg.Done()
		_, err, warn := dAsync.Get()
		async.SetResult(output, err, warn)
	}()
	return &async, nil
}
func (vu *VideoUtils) generateInt() int {
	if vu.random == nil {
		vu.random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return vu.random.Int()
}
func (vu *VideoUtils) DownloadBestAndMerge(url VideoUrl, output string, maxSizeInKb int, forceFromat string) (*Async, error) {
	bestVideoFormats := GetFormatsOrder(url.Formats, true, false)
	bestAudioFormats := GetFormatsOrder(url.Formats, false, true)
	if len(bestVideoFormats) == 0 || len(bestAudioFormats) == 0 {
		return vu.DownloadBestMaxSize(url, output, maxSizeInKb)
	}
	if forceFromat != "" {
		output += "." + forceFromat
	} else {
		output += "." + bestVideoFormats[0].Ext
	}
	videoPath := strconv.Itoa(vu.generateInt())
	audioPath := strconv.Itoa(vu.generateInt())
	var video, audio *Async
	var vErr, aErr error
	if maxSizeInKb == -1 {
		video, vErr = vu.downloadFormat(bestVideoFormats[0], videoPath)
		audio, aErr = vu.downloadFormat(bestAudioFormats[0], audioPath)
	} else {
		video, vErr = vu.downloadBestMaxSize(url, videoPath, maxSizeInKb, bestVideoFormats)
		audio, aErr = vu.downloadBestMaxSize(url, audioPath, maxSizeInKb, bestAudioFormats)
	}
	if vErr != nil || aErr != nil {
		if vErr != nil {
			return nil, vErr
		}
		return nil, aErr

	}
	var wg sync.WaitGroup
	var wa multipleWaitAble
	async := CreateAsyncWaitGroup(&wg, &wa)
	wa.add(video)
	wa.add(audio)
	wg.Add(1)
	go func() {
		defer wg.Done()
		tWarn := ""
		audioPath, err, aWarn := audio.Get()
		tWarn += aWarn
		if err != nil {
			async.SetResult(nil, err, tWarn)
			return
		}
		wa.remove(audio)
		videoPath, err, vWarn := video.Get()
		tWarn += vWarn
		if err != nil {
			async.SetResult(nil, err, tWarn)
			return
		}
		wa.remove(video)
		merge, err := vu.Ffmpeg.Merge(output, videoPath.(string), audioPath.(string))
		if err != nil {
			async.SetResult(nil, err, tWarn)
		} else {
			wa.add(merge)
			_, err, warn := merge.Get()
			wa.remove(merge)
			tWarn += warn
			async.SetResult(output, err, tWarn)
		}
		if _, err := os.Stat(output); err == nil || os.IsExist(err) {
			os.Remove(videoPath.(string))
			os.Remove(audioPath.(string))
		}
	}()
	return &async, nil
}
func (vu *VideoUtils) getBestFormatSize(async *Async, formats []Format, sizeInKBytes int) (*Format, error, string) {
	for _, format := range formats {
		if !async.isStopped {
			as, err := vu.Ffmpeg.GetInputSize(format.url)
			if err != nil {
				return nil, err, ""
			}
			size, err, warn := as.Get()
			if err != nil {
				return nil, err, warn
			}
			if size.(int) < sizeInKBytes {
				return &format, nil, ""
			}
		} else {
			return nil, &CancelError{}, ""
		}
	}
	return nil, nil, ""
}
func (vu *VideoUtils) findBestFormat(url VideoUrl, sizeInKBytes int, formats []Format, output string) (*Async, error) {
	var wg sync.WaitGroup
	async := CreateAsyncWaitGroup(&wg, nil)
	wg.Add(1)
	go func(async *Async, wg *sync.WaitGroup) {
		defer wg.Done()
		format, err, warn := vu.getBestFormatSize(async, formats, sizeInKBytes)
		if err != nil {
			async.SetResult(nil, err, warn)
		} else {
			if format == nil {
				async.SetResult(nil, &FileTooBigError{url: url}, warn)
			} else {
				output += "." + format.Ext
				as, err := vu.chooseDownload(format.url, output, format.protocol)
				if err != nil {
					async.SetResult(nil, err, "")
				} else {
					_, err, warn := as.Get()
					async.SetResult(output, err, warn)
				}
			}
		}
	}(&async, &wg)
	return &async, nil
}
func (vu *VideoUtils) downloadBestFormats(url VideoUrl, output string, formats []Format, sizeInKBytes int) (*Async, error) {
	var async *Async
	var err error
	if sizeInKBytes == -1 {
		async, err = vu.downloadFormat(formats[0], output)
	} else {
		async, err = vu.findBestFormat(url, sizeInKBytes, formats, output)
	}
	return async, err
}
func (vu *VideoUtils) asyncRename(pAsync *Async, err error, tempOutput, output string) (*Async, error) {
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncFromAsyncAsWaitAble(&wg, pAsync)
	go func() {
		defer wg.Done()
		asyncOutput, err, warn := pAsync.Get()
		if err != nil {
			async.SetResult(nil, err, warn)
		} else {
			oldOutput := asyncOutput.(string)
			newOutput := output + oldOutput[len(tempOutput):]
			err = os.Rename(oldOutput, newOutput)
			async.SetResult(newOutput, err, warn)
		}
	}()
	return &async, nil
}
func (vu *VideoUtils) DownloadBest(url VideoUrl, output string) (*Async, error) {
	tempOutput := strconv.Itoa(vu.generateInt())
	dAsync, err := vu.downloadBestFormats(url, tempOutput, GetFormatsOrder(url.Formats, true, true)[0:1], -1)
	return vu.asyncRename(dAsync, err, tempOutput, output)
}
func reduceFormats(url VideoUrl, formats []Format, sizeInKBytes int) ([]Format, error) {
	if sizeInKBytes == -1 {
		return formats[0:1], nil
	}
	var fIndex = -1
	var lastKnownIndex = -1
	for i, f := range formats {
		if f.fileSize == -1 {
			lastKnownIndex = i
		} else {
			if (int)(f.fileSize) < sizeInKBytes {
				fIndex = i
				break
			}
			lastKnownIndex = -1
		}
	}
	if fIndex == -1 {
		if lastKnownIndex == -1 {
			return nil, &FileTooBigError{url: url}
		}
		lastKnownIndex = 0
		fIndex = len(formats) - 1
	}
	if lastKnownIndex == -1 {
		lastKnownIndex = fIndex
	}
	return formats[lastKnownIndex : fIndex+1], nil
}
func (vu *VideoUtils) downloadBestMaxSize(url VideoUrl, output string, sizeInKBytes int, formats []Format) (*Async, error) {
	rFormats, err := reduceFormats(url, formats, sizeInKBytes)
	if err != nil {
		return nil, err
	}
	return vu.downloadBestFormats(url, output, rFormats, sizeInKBytes)
}
func (vu *VideoUtils) DownloadBestMaxSize(url VideoUrl, output string, sizeInKBytes int) (*Async, error) {
	tempOutput := strconv.Itoa(vu.generateInt())
	async, err := vu.downloadBestMaxSize(url, tempOutput, sizeInKBytes, GetFormatsOrder(url.Formats, true, true))
	return vu.asyncRename(async, err, tempOutput, output)
}
