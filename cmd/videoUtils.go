package main

import (
	"github.com/samitc/vigoler/vigoler"
	"os"
	"strconv"
	"strings"
	"sync"
)

type VideoUtils struct {
	Youtube *vigoler.YoutubeDlWrapper
	Ffmpeg  *vigoler.FFmpegWrapper
}
type multipleWaitAble struct {
	waitAbles []*vigoler.Async
	isStopped bool
}

func (mwa *multipleWaitAble) add(async *vigoler.Async) {
	mwa.waitAbles = append(mwa.waitAbles, async)
}
func (mwa *multipleWaitAble) remove(async *vigoler.Async) {
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
func validateFileName(fileName string) string {
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
func (vu *VideoUtils) LiveDownload(url *string, outputFile *string, maxSizeInKb, sizeSplitThreshold, maxTimeInSec, timeSplitThreshold int) (*vigoler.Async, error) {
	var wg sync.WaitGroup
	var wa multipleWaitAble
	async := vigoler.CreateAsyncWaitGroup(&wg, &wa)
	waitForVideoToDownload := func(fAsync *vigoler.Async) {
		defer wg.Done()
		_, err, warn := fAsync.Get()
		wa.remove(fAsync)
		if err != nil {
			async.SetResult(nil, err, warn)
		} else {
			async.SetResult(nil, nil, warn)
		}
	}
	downloadVideo := func(url string, setting vigoler.DownloadSettings, output string) {
		fAsync, err := vu.Ffmpeg.Download(url, setting, output)
		if err != nil {
			async.SetResult(nil, err, "")
		} else {
			wg.Add(1)
			waitForVideoToDownload(fAsync)
		}
	}
	splitCallback := func(url string, setting vigoler.DownloadSettings, output string) {
		output = addIndexToFileName(output)
		downloadVideo(url, setting, output)
	}
	fAsync, err := vu.Ffmpeg.Download(*url, vigoler.DownloadSettings{CallbackBeforeSplit: splitCallback, MaxSizeInKb: maxSizeInKb, MaxTimeInSec: maxTimeInSec, SizeSplitThreshold: sizeSplitThreshold, TimeSplitThreshold: timeSplitThreshold}, *outputFile)
	if err != nil {
		return nil, err
	} else {
		wg.Add(1)
		wa.add(fAsync)
		go waitForVideoToDownload(fAsync)
	}
	return &async, nil
}
func (vu *VideoUtils) DownloadBestAndMerge(url vigoler.VideoUrl, output string) (*vigoler.Async, error) {
	video, vErr := vu.Youtube.Download(url, vigoler.CreateBestVideoFormat())
	audio, aErr := vu.Youtube.Download(url, vigoler.CreateBestAudioFormat())
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
	async := vigoler.CreateAsyncWaitGroup(&wg, &wa)
	wa.add(video)
	wa.add(audio)
	wg.Add(1)
	go func(video, audio *vigoler.Async, output string, url vigoler.VideoUrl) {
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
			if _, ok := err.(*vigoler.BadFormatError); ok {
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
func (vu *VideoUtils) download(url vigoler.VideoUrl, output string, format vigoler.Format) (*vigoler.Async, error) {
	var wg sync.WaitGroup
	yAsync, err := vu.Youtube.Download(url, format)
	async := vigoler.CreateAsyncFromAsyncAsWaitAble(&wg, yAsync)
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
func (vu *VideoUtils) DownloadBest(url vigoler.VideoUrl, output string) (*vigoler.Async, error) {
	return vu.download(url, output, vigoler.CreateBestFormat())
}
func (vu *VideoUtils) DownloadBestMaxSize(url vigoler.VideoUrl, output string, sizeInMb int) (*vigoler.Async, error) {
	format := vigoler.CreateBestFormat()
	format.MaxFileSizeInMb = sizeInMb
	return vu.download(url, output, format)
}
