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
func (vu *VideoUtils) LiveDownload(url *string, outputFile *string, maxSizeInKb, sizeSplitThreshold, maxTimeInSec, timeSplitThreshold int) (*Async, error) {
	var wg sync.WaitGroup
	var wa multipleWaitAble
	async := CreateAsyncWaitGroup(&wg, &wa)
	waitForVideoToDownload := func(fAsync *Async) {
		defer wg.Done()
		_, err, warn := fAsync.Get()
		wa.remove(fAsync)
		if err != nil {
			async.SetResult(nil, err, warn)
		} else {
			async.SetResult(nil, nil, warn)
		}
	}
	downloadVideo := func(url string, setting DownloadSettings, output string) {
		fAsync, err := vu.Ffmpeg.Download(url, setting, output)
		if err != nil {
			async.SetResult(nil, err, "")
		} else {
			wg.Add(1)
			waitForVideoToDownload(fAsync)
		}
	}
	splitCallback := func(url string, setting DownloadSettings, output string) {
		output = addIndexToFileName(output)
		downloadVideo(url, setting, output)
	}
	fAsync, err := vu.Ffmpeg.Download(*url, DownloadSettings{CallbackBeforeSplit: splitCallback, MaxSizeInKb: maxSizeInKb, MaxTimeInSec: maxTimeInSec, SizeSplitThreshold: sizeSplitThreshold, TimeSplitThreshold: timeSplitThreshold}, *outputFile)
	if err != nil {
		return nil, err
	} else {
		wg.Add(1)
		wa.add(fAsync)
		go waitForVideoToDownload(fAsync)
	}
	return &async, nil
}
func (vu *VideoUtils) DownloadBestAndMerge(url VideoUrl, output string) (*Async, error) {
	video, vErr := vu.Youtube.Download(url, CreateBestVideoFormat())
	audio, aErr := vu.Youtube.Download(url, CreateBestAudioFormat())
	if vErr != nil || aErr != nil {
		if vErr != nil {
			return nil, vErr
		} else {
			return nil, aErr
		}
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
func (vu *VideoUtils) download(url VideoUrl, output string, format Format) (*Async, error) {
	var wg sync.WaitGroup
	yAsync, err := vu.Youtube.Download(url, format)
	async := CreateAsyncFromAsyncAsWaitAble(&wg, yAsync)
	if err != nil {
		return nil, err
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fileOutput, err, warn := yAsync.Get()
			if err != nil {
				async.SetResult(nil, err, warn)
			} else {
				os.Rename(*fileOutput.(*string), output)
				async.SetResult(nil, nil, warn)
			}
		}()
	}
	return &async, nil
}
func (vu *VideoUtils) DownloadBest(url VideoUrl, output string) (*Async, error) {
	return vu.download(url, output, CreateBestFormat())
}
func (vu *VideoUtils) DownloadBestMaxSize(url VideoUrl, output string, sizeInMb int) (*Async, error) {
	format := CreateBestFormat()
	format.MaxFileSizeInMb = sizeInMb
	return vu.download(url, output, format)
}
