package vigoler

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type VideoUtils struct {
	Youtube                  *YoutubeDlWrapper
	Ffmpeg                   *FFmpegWrapper
	Curl                     *CurlWrapper
	MinLiveErrorRetryingTime int
	random                   *rand.Rand
}
type LiveVideoCallback func(data interface{}, fileName string, async *Async)
type TypedError interface {
	error
	Type() string
}
type LogError interface {
	error
	LogAttributes() map[string]interface{}
}
type FileTooBigError struct {
	url VideoUrl
}
type FormatNotFoundError struct {
	format Format
	warn   string
	videos []VideoUrl
}

func (e *FileTooBigError) Error() string {
	return fmt.Sprintf("File %s is too big to download", e.url.Name)
}
func (e *FileTooBigError) Type() string {
	return "File too big error"
}
func (e *FormatNotFoundError) Error() string {
	return fmt.Sprintf("Format %s not found", e.format.formatID)
}
func (e *FormatNotFoundError) LogAttributes() map[string]interface{} {
	return map[string]interface{}{"warn": e.warn, "videos": e.videos}
}
func (vu *VideoUtils) createFileName(ext string, format Format) string {
	if vu.random == nil {
		vu.random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	file := strconv.Itoa(vu.random.Int()) + "."
	if ext != "" {
		file += ext
	} else {
		file += format.Ext
	}
	return file
}
func (vu *VideoUtils) chooseDownload(url, output, protocol string, headers map[string]string) (*Async, error) {
	if protocol == "https" {
		return vu.Curl.DownloadHeaders(url, headers, output)
	}
	return vu.Ffmpeg.DownloadHeaders(url, headers, output)
}
func (vu *VideoUtils) recreateURL(url VideoUrl, format Format) (Format, error) {
	const retryingTime = 2
	var lastWarn string
	var lastVideos []VideoUrl
	for i := 0; i < retryingTime; i++ {
		async, err := vu.Youtube.GetUrls(url.url)
		if err != nil {
			return Format{}, err
		}
		videos, _, warn := async.Get()
		lastVideos = videos.([]VideoUrl)
		lastWarn = warn
		for _, video := range lastVideos {
			if url.ID == video.ID {
				for _, form := range video.Formats {
					if form.formatID == format.formatID {
						return form, nil
					}
				}
			}
		}
	}
	return Format{}, &FormatNotFoundError{
		format: format,
		warn:   lastWarn,
		videos: lastVideos,
	}
}
func (vu *VideoUtils) LiveDownload(log *Logger, url VideoUrl, format Format, ext string, maxSizeInKb, sizeSplitThreshold, maxTimeInSec, timeSplitThreshold int, liveVideoCallback LiveVideoCallback, data interface{}) (*Async, error) {
	var wg sync.WaitGroup
	var wa multipleWaitAble
	var lastErr error
	lastWarn := ""
	lData := data
	runsIndex := int32(0)
	lLiveVideoCallback := liveVideoCallback
	var downloadVideo func(errorsCount time.Time, setting DownloadSettings)
	waitForVideoToDownload := func(fAsync *Async, output string, errorTime time.Time, setting DownloadSettings) {
		log.startDownloadLive(url, output)
		_, err, warn := fAsync.Get()
		wa.remove(fAsync)
		if _, isWaitError := err.(*WaitError); isWaitError || err == ServerStopSendDataError {
			log.finishDownloadLive(url, output, warn, err)
			err = nil
			now := time.Now()
			if int(now.Sub(errorTime).Seconds()) > vu.MinLiveErrorRetryingTime {
				fAsync.err = nil
				if lLiveVideoCallback != nil {
					lLiveVideoCallback(lData, output, fAsync)
				}
				if !wa.isStopped {
					downloadVideo(now, setting)
				}
			}
		} else {
			if err != nil {
				log.finishDownloadLiveError(url, output, warn, err)
			} else {
				log.finishDownloadLive(url, output, warn, err)
			}
			if lLiveVideoCallback != nil {
				lLiveVideoCallback(lData, output, fAsync)
			}
		}
		if lastErr == nil {
			lastErr, lastWarn = err, warn
		}
	}
	downloadVideo = func(errorTime time.Time, setting DownloadSettings) {
		wg.Add(1)
		defer wg.Done()
		format, err := vu.recreateURL(url, format)
		if err != nil {
			if logError, ok := err.(LogError); ok {
				log.logLogError(url, "Recreate live to download", logError)
			} else {
				log.liveRecreatedError(url, err)
			}
			if lastErr == nil {
				lastErr, lastWarn = err, ""
			}
		} else {
			output := vu.createFileName(ext, format)
			log.liveRecreated(url, output)
			var fAsync *Async
			curRunIndex := atomic.AddInt32(&runsIndex, 1)
			fAsync, err = vu.Ffmpeg.DownloadSplit(format.url, setting, output, log.withLiveId(int(curRunIndex)))
			if err != nil {
				log.liveDownloadError(url, output, err)
				if lastErr == nil {
					lastErr, lastWarn = err, ""
				}
			} else {
				wa.add(fAsync)
				waitForVideoToDownload(fAsync, output, errorTime, setting)
			}
		}
	}
	splitCallback := func(url string, setting DownloadSettings, output string) {
		if !wa.isStopped {
			downloadVideo(time.Time{}, setting)
		}
	}
	output := vu.createFileName(ext, format)
	setting := DownloadSettings{CallbackBeforeSplit: splitCallback, MaxSizeInKb: maxSizeInKb, MaxTimeInSec: maxTimeInSec, SizeSplitThreshold: sizeSplitThreshold, TimeSplitThreshold: timeSplitThreshold, returnWaitError: true}
	curRunIndex := atomic.AddInt32(&runsIndex, 1)
	fAsync, err := vu.Ffmpeg.DownloadSplit(format.url, setting, output, log.withLiveId(int(curRunIndex)))
	if err != nil {
		return nil, err
	}
	var wga sync.WaitGroup
	async := CreateAsyncWaitGroup(&wga, &wa)
	wa.add(fAsync)
	wga.Add(1)
	go func() {
		waitForVideoToDownload(fAsync, output, time.Time{}, setting)
		wg.Wait()
		async.SetResult(nil, lastErr, lastWarn)
		wga.Done()
	}()
	return &async, nil
}
func (vu *VideoUtils) DownloadLiveUntilNow(url VideoUrl, format Format, ext string) (*Async, error) {
	output := vu.createFileName(ext, format)
	as, err := vu.Ffmpeg.DownloadLiveUntilNow(format.url, output)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncFromAsyncAsWaitAble(&wg, as)
	go func() {
		defer wg.Done()
		_, err, warn := as.Get()
		async.SetResult(output, err, warn)
	}()
	return &async, nil
}
func (vu *VideoUtils) downloadFormat(format Format, ext string) (*Async, error) {
	output := vu.createFileName(ext, format)
	dAsync, err := vu.chooseDownload(format.url, output, format.protocol, format.httpHeaders)
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
func formatLess(a, b *Format) bool {
	return a.width < b.width || (a.width == b.width && a.height < b.height)
}
func (vu *VideoUtils) needToDownloadBestFormat(bestVideoFormats, bestAudioFormats, bestFormats []Format, mergeOnlyIfHigherResolution bool) bool {
	return (len(bestVideoFormats) == 0 || len(bestAudioFormats) == 0) || (mergeOnlyIfHigherResolution && len(bestFormats) > 0 && formatLess(&bestVideoFormats[0], &bestFormats[0]))
}
func (vu *VideoUtils) DownloadBestAndMerge(url VideoUrl, maxSizeInKb int, ext string, mergeOnlyIfHigherResolution bool) (*Async, error) {
	bestVideoFormats := GetFormatsOrder(url.Formats, true, false)
	bestAudioFormats := GetFormatsOrder(url.Formats, false, true)
	bestFormats := GetFormatsOrder(url.Formats, true, true)
	if vu.needToDownloadBestFormat(bestVideoFormats, bestAudioFormats, bestFormats, mergeOnlyIfHigherResolution) {
		return vu.downloadBestMaxSize(url, maxSizeInKb, ext, bestFormats)
	}
	var video, audio *Async
	var vErr, aErr error
	if maxSizeInKb == -1 {
		video, vErr = vu.downloadFormat(bestVideoFormats[0], ext)
		audio, aErr = vu.downloadFormat(bestAudioFormats[0], ext)
	} else {
		video, vErr = vu.downloadBestMaxSize(url, maxSizeInKb, ext, bestVideoFormats)
		audio, aErr = vu.downloadBestMaxSize(url, maxSizeInKb, ext, bestAudioFormats)
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
		wasErr := false
		tWarn := ""
		audioPath, err, aWarn := audio.Get()
		tWarn += aWarn
		if err != nil {
			async.SetResult(nil, err, tWarn)
			wasErr = true
		}
		wa.remove(audio)
		videoPath, err, vWarn := video.Get()
		tWarn += vWarn
		if !wasErr && err != nil {
			async.SetResult(nil, err, tWarn)
			wasErr = true
		}
		wa.remove(video)
		audioPathStr := audioPath.(string)
		videoPathStr := videoPath.(string)
		defer func() {
			_ = os.Remove(audioPathStr)
			_ = os.Remove(videoPathStr)
		}()
		if !wasErr {
			output := vu.createFileName(ext, bestVideoFormats[0])
			merge, err := vu.Ffmpeg.Merge(output, videoPathStr, audioPathStr)
			if err != nil {
				async.SetResult(nil, err, tWarn)
			} else {
				wa.add(merge)
				_, err, warn := merge.Get()
				wa.remove(merge)
				tWarn += warn
				if err != nil {
					_ = os.Remove(output)
				}
				async.SetResult(output, err, tWarn)
			}
		}
	}()
	return &async, nil
}
func (vu *VideoUtils) getBestFormatSize(async *Async, formats []Format, sizeInKBytes int) (*Format, string, error) {
	for _, format := range formats {
		if !async.isStopped {
			as, err := vu.Ffmpeg.GetInputSizeHeaders(format.url, format.httpHeaders)
			if err != nil {
				return nil, "", err
			}
			size, err, warn := as.Get()
			if err != nil {
				return nil, warn, err
			}
			if size.(int) < sizeInKBytes {
				return &format, "", nil
			}
		} else {
			return nil, "", &CancelError{}
		}
	}
	return nil, "", nil
}
func (vu *VideoUtils) findBestFormat(url VideoUrl, sizeInKBytes int, formats []Format, ext string) (*Async, error) {
	var wg sync.WaitGroup
	async := CreateAsyncWaitGroup(&wg, nil)
	wg.Add(1)
	go func(async *Async, wg *sync.WaitGroup) {
		defer wg.Done()
		format, warn, err := vu.getBestFormatSize(async, formats, sizeInKBytes)
		if err != nil {
			async.SetResult(nil, err, warn)
		} else {
			if format == nil {
				async.SetResult(nil, &FileTooBigError{url: url}, warn)
			} else {
				output := vu.createFileName(ext, *format)
				as, err := vu.chooseDownload(format.url, output, format.protocol, format.httpHeaders)
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
func (vu *VideoUtils) downloadBestFormats(url VideoUrl, ext string, formats []Format, sizeInKBytes int) (*Async, error) {
	var async *Async
	var err error
	if sizeInKBytes == -1 {
		async, err = vu.downloadFormat(formats[0], ext)
	} else {
		async, err = vu.findBestFormat(url, sizeInKBytes, formats, ext)
	}
	return async, err
}
func (vu *VideoUtils) DownloadBest(url VideoUrl, ext string) (*Async, error) {
	return vu.downloadBestFormats(url, ext, GetFormatsOrder(url.Formats, true, true)[0:1], -1)
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
func (vu *VideoUtils) downloadBestMaxSize(url VideoUrl, sizeInKBytes int, ext string, formats []Format) (*Async, error) {
	rFormats, err := reduceFormats(url, formats, sizeInKBytes)
	if err != nil {
		return nil, err
	}
	return vu.downloadBestFormats(url, ext, rFormats, sizeInKBytes)
}
func (vu *VideoUtils) DownloadBestMaxSize(url VideoUrl, sizeInKBytes int, ext string) (*Async, error) {
	return vu.downloadBestMaxSize(url, sizeInKBytes, ext, GetFormatsOrder(url.Formats, true, true))
}
