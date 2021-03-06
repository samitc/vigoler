package main

import (
	"flag"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strings"
	"sync"

	. "github.com/samitc/vigoler/2/vigoler"
)

type stringArgsArray []string
type outputVideo struct {
	video    VideoUrl
	fileName string
	format   string
}

func (dua *stringArgsArray) String() string {
	return strings.Join(*dua, ",")
}
func (dua *stringArgsArray) Set(value string) error {
	*dua = append(*dua, value)
	return nil
}
func validateAsync(err error, warn, warnPrefix string) {
	if err != nil {
		panic(err)
	}
	if warn != "" {
		fmt.Println(warnPrefix + ":" + warn + "\n")
	}
}
func getAsyncData(async *Async, warnPrefix string) interface{} {
	i, err, warn := async.Get()
	validateAsync(err, warn, warnPrefix)
	return i
}
func validateFileName(fileName string) string {
	notAllowCh := []string{`\`, `/`, `:`, `|`, `?`, `"`, `*`, `<`, `>`}
	for _, ch := range notAllowCh {
		fileName = strings.Replace(fileName, ch, "", -1)
	}
	return fileName
}
func downloadBestAndMerge(url VideoUrl, videoUtils *VideoUtils, outputFormat string) *Async {
	async, err := videoUtils.DownloadBestAndMerge(url, -1, outputFormat, true)
	if err != nil {
		panic(err)
	} else {
		return async
	}
}
func liveDownload(l *zap.Logger, videos <-chan outputVideo, videoUtils *VideoUtils, wg *sync.WaitGroup) {
	defer wg.Done()
	var filesName []string
	maxSizeInKb := 9.8 * 1024 * 1024
	sizeSplitThreshold := 9.7 * 1024 * 1024
	maxTimeInSec := 5.5 * 60 * 60
	timeSplitThreshold := 5.4 * 60 * 60
	var downloadAsync []*Async
	for video := range videos {
		async, err := videoUtils.LiveDownload(&Logger{Logger: l.With(zap.Any("video", video))}, video.video, GetBestFormat(video.video.Formats, true, true), video.format, int(maxSizeInKb), int(sizeSplitThreshold), int(maxTimeInSec), int(timeSplitThreshold), nil, nil)
		if err != nil {
			fmt.Println(err)
		} else {
			downloadAsync = append(downloadAsync, async)
			filesName = append(filesName, video.fileName)
		}
	}
	for i, s := range downloadAsync {
		output := getAsyncData(s, filesName[i]).(string)
		os.Rename(output, filesName[i]+filepath.Ext(output))
	}
}
func main() {
	var downloads stringArgsArray
	var directories stringArgsArray
	var outputFormat stringArgsArray
	flag.Var(&downloads, "d", "url to download")
	flag.Var(&directories, "n", "directories names")
	flag.Var(&outputFormat, "f", "output file format")
	flag.Parse()
	l, err := zap.NewProduction(zap.WithCaller(false))
	if err != nil {
		panic(err)
	}
	youtube := CreateYoutubeDlWrapper()
	ffmpeg := CreateFfmpegWrapper(-1, false)
	curl := CreateCurlWrapper(3)
	videoUtils := VideoUtils{Youtube: &youtube, Ffmpeg: &ffmpeg, Curl: &curl}
	var pendingUrlAsync []*Async
	liveDownChan := make(chan outputVideo)
	var wg sync.WaitGroup
	for _, d := range downloads {
		async, err := youtube.GetUrls(d)
		if err != nil {
			fmt.Println(err)
		} else {
			pendingUrlAsync = append(pendingUrlAsync, async)
		}
	}
	wg.Add(1)
	var pendingDownloadAsync []*Async
	var pendingDownloadNames []string
	var pendingLiveAsync []*Async
	var pendingLiveNames []string
	go liveDownload(l, liveDownChan, &videoUtils, &wg)
	for i, a := range pendingUrlAsync {
		urls := getAsyncData(a, downloads[i]).([]VideoUrl)
		for _, url := range urls {
			fileName := directories[i] + string(os.PathSeparator) + validateFileName(url.Name)
			if url.IsLive {
				liveDownChan <- outputVideo{video: url, fileName: fileName, format: outputFormat[i]}
				as, err := videoUtils.DownloadLiveUntilNow(url, GetBestFormat(url.Formats, true, true), outputFormat[i])
				if err != nil {
					panic(err)
				}
				pendingLiveAsync = append(pendingLiveAsync, as)
				pendingLiveNames = append(pendingLiveNames, fileName)
			} else {
				as := downloadBestAndMerge(url, &videoUtils, outputFormat[i])
				pendingDownloadAsync = append(pendingDownloadAsync, as)
				pendingDownloadNames = append(pendingDownloadNames, fileName)
			}
		}
	}
	for i, a := range pendingLiveAsync {
		output, err, warn := a.Get()
		if err != nil {
			if _, ok := err.(*UnsupportedSeekError); !ok {
				validateAsync(err, warn, pendingLiveNames[i])
			}
		}
		os.Rename(output.(string), pendingLiveNames[i]+filepath.Ext(output.(string)))
	}
	for i, a := range pendingDownloadAsync {
		output := getAsyncData(a, pendingDownloadNames[i]).(string)
		os.Rename(output, pendingDownloadNames[i]+filepath.Ext(output))
	}
	close(liveDownChan)
	wg.Wait()
}
