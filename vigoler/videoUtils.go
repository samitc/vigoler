package vigoler

import (
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
	random  *rand.Rand
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
type FileTooBigError struct {
	url VideoUrl
}

func (e *FileTooBigError) Error() string {
	return fmt.Sprintf("File %s is too big to download", e.url.url)
}
func (e *FileTooBigError) Type() string {
	return "File too big error"
}
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
func (vu *VideoUtils) LiveDownload(url VideoUrl, format Format, outputFile string, maxSizeInKb, sizeSplitThreshold, maxTimeInSec, timeSplitThreshold int, liveVideoCallback LiveVideoCallback, data interface{}) (*Async, error) {
	var wg sync.WaitGroup
	var wa multipleWaitAble
	var lastErr error
	lastWarn := ""
	lData := data
	outputFile += "." + format.Ext
	var lastResult = outputFile
	realURLAsync, err := vu.Youtube.GetRealUrl(url, format)
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
		lastErr, lastWarn = err, warn
	}
	downloadVideo := func(setting DownloadSettings, output string) {
		realURLAsync, err := vu.Youtube.GetRealUrl(url, format)
		if err != nil {
			lastErr, lastWarn = err, ""
		} else {
			realURL, err, warn := realURLAsync.Get()
			if err != nil {
				lastErr, lastWarn = err, warn
			} else {
				fAsync, err := vu.Ffmpeg.Download(*realURL.(*string), setting, output)
				if err != nil {
					lastErr, lastWarn = err, ""
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
	fAsync, err := vu.Ffmpeg.Download(*realURL.(*string), DownloadSettings{CallbackBeforeSplit: splitCallback, MaxSizeInKb: maxSizeInKb, MaxTimeInSec: maxTimeInSec, SizeSplitThreshold: sizeSplitThreshold, TimeSplitThreshold: timeSplitThreshold}, outputFile)
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
func (vu *VideoUtils) downloadFormat(url VideoUrl, format Format, setting DownloadSettings, output string) (*Async, error) {
	var wg sync.WaitGroup
	urlAsync, err := vu.Youtube.GetRealUrl(url, format)
	if err != nil {
		return nil, err
	}
	var wa multipleWaitAble
	wa.add(urlAsync)
	async := CreateAsyncWaitGroup(&wg, &wa)
	wg.Add(1)
	go func() {
		defer wg.Done()
		realURL, err, warn := urlAsync.Get()
		if err != nil {
			async.SetResult(nil, err, warn)
			return
		}
		wa.remove(urlAsync)
		downloadAsync, err := vu.Ffmpeg.Download(*realURL.(*string), setting, output)
		if err != nil {
			async.SetResult(nil, err, warn)
			return
		}
		wa.add(downloadAsync)
		_, err, dWarn := downloadAsync.Get()
		if err != nil {
			async.SetResult(nil, err, warn+dWarn)
			return
		}
		wa.remove(downloadAsync)
		async.SetResult(output, nil, warn+dWarn)
	}()
	return &async, nil
}
func (vu *VideoUtils) DownloadBestAndMerge(url VideoUrl, output string) (*Async, error) {
	bestVideoFormats := GetFormatsOrder(url.Formats, true, false)
	bestAudioFormats := GetFormatsOrder(url.Formats, false, true)
	if len(bestVideoFormats) == 0 || len(bestAudioFormats) == 0 {
		return vu.DownloadBest(url, output)
	}
	if vu.random == nil {
		vu.random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	output += "." + bestVideoFormats[0].Ext
	videoPath := strconv.Itoa(vu.random.Int()) + "." + bestVideoFormats[0].Ext
	audioPath := strconv.Itoa(vu.random.Int()) + "." + bestAudioFormats[0].Ext
	video, vErr := vu.downloadFormat(url, bestVideoFormats[0], DownloadSettings{}, videoPath)
	audio, aErr := vu.downloadFormat(url, bestAudioFormats[0], DownloadSettings{}, audioPath)
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
		_, err, aWarn := audio.Get()
		tWarn += aWarn
		if err != nil {
			async.SetResult(nil, err, tWarn)
			return
		}
		wa.remove(audio)
		_, err, vWarn := video.Get()
		tWarn += vWarn
		if err != nil {
			async.SetResult(nil, err, tWarn)
			return
		}
		wa.remove(video)
		merge, err := vu.Ffmpeg.Merge(output, videoPath, audioPath)
		if err != nil {
			async.SetResult(nil, err, tWarn)
		} else {
			wa.add(merge)
			_, err, warn := merge.Get()
			wa.remove(merge)
			tWarn += warn
			async.SetResult(nil, err, tWarn)
		}
		if _, err := os.Stat(output); err == nil || os.IsExist(err) {
			os.Remove(videoPath)
			os.Remove(audioPath)
		}
	}()
	return &async, nil
}
func (vu *VideoUtils) findBestFormat(url VideoUrl, sizeInKBytes int, formats []Format, output string) (*Async, error) {
	var wg sync.WaitGroup
	async := CreateAsyncWaitGroup(&wg, nil)
	wg.Add(1)
	go func(async *Async, wg *sync.WaitGroup) {
		defer wg.Done()
		for _, format := range formats {
			if !async.isStopped {
				as, err := vu.Youtube.GetRealUrl(url, format)
				if err != nil {
					async.SetResult(nil, err, "")
					break
				}
				url, err, warn := as.Get()
				if err != nil {
					async.SetResult(nil, err, warn)
					break
				}
				as, err = vu.Ffmpeg.GetInputSize(*url.(*string))
				if err != nil {
					async.SetResult(nil, err, "")
					break
				}
				size, err, warn := as.Get()
				if err != nil {
					async.SetResult(nil, err, warn)
					break
				}
				if size.(int) < sizeInKBytes {
					output += "." + format.Ext
					as, err := vu.Ffmpeg.Download(*url.(*string), DownloadSettings{}, output)
					if err != nil {
						async.SetResult(nil, err, "")
					} else {
						_, err, warn := as.Get()
						async.SetResult(output, err, warn)
					}
					break
				}
			}
		}
	}(&async, &wg)
	return &async, nil
}
func (vu *VideoUtils) downloadBestFormats(url VideoUrl, output string, formats []Format, sizeInKBytes int) (*Async, error) {
	var async *Async
	var err error
	if len(formats) == 1 {
		async, err = vu.downloadFormat(url, formats[0], DownloadSettings{}, output+"."+formats[0].Ext)
	} else {
		async, err = vu.findBestFormat(url, sizeInKBytes, formats, output)
	}
	return async, err
}
func (vu *VideoUtils) DownloadBest(url VideoUrl, output string) (*Async, error) {
	return vu.downloadBestFormats(url, output, GetFormatsOrder(url.Formats, true, true)[0:1], -1)
}
func (vu *VideoUtils) DownloadBestMaxSize(url VideoUrl, output string, sizeInKBytes int) (*Async, error) {
	formats := GetFormatsOrder(url.Formats, true, true)
	var fIndex = -1
	var lastKnownIndex = -1
	if sizeInKBytes != -1 {
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
	return vu.downloadBestFormats(url, output, formats[lastKnownIndex:fIndex+1], sizeInKBytes)
}
