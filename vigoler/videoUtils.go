package vigoler

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

type VideoUtils struct {
	Youtube *YoutubeDlWrapper
	Ffmpeg  *FFmpegWrapper
}
type multipleWaitAble struct {
	waitAbles []*Async
	isStopped bool
}
type LiveVideoCallback func(data interface{}, fileName string, async *Async)
type TypedError interface {
	error
	Type() string
}
type onDownloadEnd func(err error, warn string) (string, error, string)

func (mwa *multipleWaitAble) add(async *Async) {
	mwa.waitAbles = append(mwa.waitAbles, async)
}
func (mwa *multipleWaitAble) remove(async *Async) {
	waitAbleLen := len(mwa.waitAbles) - 1
	for i, v := range mwa.waitAbles {
		if v == async {
			mwa.waitAbles[i] = mwa.waitAbles[waitAbleLen]
			break
		}
	}
	mwa.waitAbles = mwa.waitAbles[:waitAbleLen]
}
func (mwa *multipleWaitAble) Wait() error {
	for _, wa := range mwa.waitAbles {
		_, err, _ := wa.Get()
		if err != nil {
			return err
		}
	}
	return nil
}
func (mwa *multipleWaitAble) Stop() error {
	for _, wa := range mwa.waitAbles {
		err := wa.Stop()
		if err != nil {
			return err
		}
	}
	mwa.isStopped = true
	return nil
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
func (vu *VideoUtils) LiveDownload(url VideoUrl, format Format, outputFile *string, maxSizeInKb, sizeSplitThreshold, maxTimeInSec, timeSplitThreshold int, liveVideoCallback LiveVideoCallback, data interface{}) (*Async, error) {
	var wg sync.WaitGroup
	var wa multipleWaitAble
	var lastResult interface{}
	var lastErr error
	lastWarn := ""
	lData := data
	realURLAsync, err := vu.Youtube.GetRealVideoUrl(url, format)
	if err != nil {
		return nil, err
	}
	lLiveVideoCallback := liveVideoCallback
	waitForVideoToDownload := func(fAsync *Async, output string) {
		defer wg.Done()
		_, err, warn := fAsync.Get()
		wa.remove(fAsync)
		if lLiveVideoCallback != nil {
			lLiveVideoCallback(lData, output, fAsync)
		}
		lastResult, lastErr, lastWarn = nil, err, warn
	}
	downloadVideo := func(setting DownloadSettings, output string) {
		realURLAsync, err := vu.Youtube.GetRealVideoUrl(url, format)
		if err != nil {
			lastResult, lastErr, lastWarn = nil, err, ""
		} else {
			realURL, err, warn := realURLAsync.Get()
			if err != nil {
				lastResult, lastErr, lastWarn = nil, err, warn
			} else {
				fAsync, err := vu.Ffmpeg.Download(*realURL.(*string), setting, output)
				if err != nil {
					lastResult, lastErr, lastWarn = nil, err, ""
				} else {
					wg.Add(1)
					wa.add(fAsync)
					waitForVideoToDownload(fAsync, output)
				}
			}
		}
	}
	splitCallback := func(url string, setting DownloadSettings, output string) {
		output = addIndexToFileName(output)
		downloadVideo(setting, output)
	}
	realURL, err, _ := realURLAsync.Get()
	if err != nil {
		return nil, err
	}
	fAsync, err := vu.Ffmpeg.Download(*realURL.(*string), DownloadSettings{CallbackBeforeSplit: splitCallback, MaxSizeInKb: maxSizeInKb, MaxTimeInSec: maxTimeInSec, SizeSplitThreshold: sizeSplitThreshold, TimeSplitThreshold: timeSplitThreshold}, *outputFile)
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
	go waitForVideoToDownload(fAsync, *outputFile)
	return &async, nil
}
func (vu *VideoUtils) DownloadBestAndMerge(url VideoUrl, output string) (*Async, error) {
	video, vErr := vu.Youtube.DownloadVideoUrl(url, GetBestFormat(url.Formats, true, false), nil)
	audio, aErr := vu.Youtube.DownloadVideoUrl(url, GetBestFormat(url.Formats, false, true), nil)
	if vErr != nil || aErr != nil {
		if vErr != nil {
			return nil, vErr
		}
		return nil, aErr

	}
	var wg sync.WaitGroup
	var wa multipleWaitAble
	wa.isStopped = false
	async := CreateAsyncWaitGroup(&wg, &wa)
	wa.add(video)
	wa.add(audio)
	wg.Add(1)
	go func(video, audio *Async, output string, url VideoUrl) {
		defer wg.Done()
		videoPath, vErr, vWarn := video.Get()
		wa.remove(video)
		audioPath, aErr, aWarn := audio.Get()
		wa.remove(audio)
		if vErr != nil || aErr != nil {
			err := aErr
			if vErr != nil {
				err = vErr
			}
			if _, ok := err.(*BadFormatError); ok {
				if !wa.isStopped {
					dAsync, err := vu.DownloadBest(url, output)
					if err != nil {
						async.SetResult(nil, err, "")
					} else {
						wa.add(dAsync)
						_, err, warn := dAsync.Get()
						wa.remove(dAsync)
						async.SetResult(nil, err, warn)
					}
				}
			} else {
				async.SetResult(nil, err, vWarn+aWarn)
			}
		} else {
			if !wa.isStopped {
				merge, err := vu.Ffmpeg.Merge(output, *videoPath.(*string), *audioPath.(*string))
				if err != nil {
					async.SetResult(nil, err, "")
				} else {
					wa.add(merge)
					_, err, warn := merge.Get()
					wa.remove(merge)
					async.SetResult(nil, err, warn)
				}
				if _, err := os.Stat(output); err == nil || os.IsExist(err) {
					os.Remove(*videoPath.(*string))
					os.Remove(*audioPath.(*string))
				}
			}
		}
	}(video, audio, output, url)
	return &async, nil
}
func (vu *VideoUtils) findBestFormat(url VideoUrl, sizeInBytes int, formats []Format) (*Async, error) {
	var wg sync.WaitGroup
	async := CreateAsyncWaitGroup(&wg, nil)
	wg.Add(1)
	go func(async *Async, wg *sync.WaitGroup) {
		defer wg.Done()
		var err error
		for _, format := range formats {
			if !async.isStopped {
				var as *Async
				as, err = vu.Youtube.DownloadVideoUrl(url, format, func(url VideoUrl, prec, sizeInMb float32) {
					if (int)(sizeInMb*1024*1024) > sizeInBytes {
						as.Stop()
					}
				})
				async.async = as
				if err != nil {
					async.SetResult(nil, err, "")
					break
				} else {
					var dest interface{}
					var warn string
					dest, err, warn = as.Get()
					if err != nil {
						if _, ok := err.(*CancelError); !ok {
							async.SetResult(nil, err, warn)
						}
					} else {
						async.SetResult(dest.(*string), nil, warn)
					}
				}
			}
		}
	}(&async, &wg)
	return &async, nil
}
func (vu *VideoUtils) downloadBestFormats(url VideoUrl, output string, formats []Format, sizeInBytes int) (*Async, error) {
	var async *Async
	var err error
	if len(formats) == 1 {
		async, err = vu.Youtube.DownloadVideoUrl(url, formats[0], nil)
	} else {
		async, err = vu.findBestFormat(url, sizeInBytes, formats)
	}
	var mainAsync Async
	if err == nil {
		var wg sync.WaitGroup
		wg.Add(1)
		mainAsync = CreateAsyncFromAsyncAsWaitAble(&wg, async)
		go func(resAsync, downloadAsync *Async, wg *sync.WaitGroup) {
			defer wg.Done()
			dest, err, warn := downloadAsync.Get()
			if err == nil {
				os.Rename(*dest.(*string), output)
			}
			resAsync.SetResult(nil, err, warn)
		}(&mainAsync, async, &wg)
	}
	return &mainAsync, err
}
func (vu *VideoUtils) DownloadBest(url VideoUrl, output string) (*Async, error) {
	return vu.downloadBestFormats(url, output, GetFormatsOrder(url.Formats, true, true)[0:0], -1)
}
func (vu *VideoUtils) DownloadBestMaxSize(url VideoUrl, output string, sizeInBytes int) (*Async, error) {
	formats := GetFormatsOrder(url.Formats, true, true)
	var fIndex = 0
	var lastKnownIndex = -1
	if sizeInBytes != -1 {
		for i, f := range formats {
			if f.fileSize == -1 {
				lastKnownIndex = i
			} else {
				if (int)(f.fileSize) < sizeInBytes {
					fIndex = i
					break
				}
				lastKnownIndex = -1
			}
		}
	}
	if lastKnownIndex == -1 {
		lastKnownIndex = fIndex
	}
	return vu.downloadBestFormats(url, output, formats[lastKnownIndex:fIndex+1], sizeInBytes)
}
