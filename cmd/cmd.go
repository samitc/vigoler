package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	. "github.com/samitc/vigoler/vigoler"
)

type empty struct{}
type semaphore chan empty
type stringArgsArray []string
type outputVideo struct {
	video     VideoUrl
	directory string
	format    string
}

func (s *semaphore) Signal() {
	<-*s
}

func (s *semaphore) Wait() {
	*s <- empty{}
}
func (dua *stringArgsArray) String() string {
	return strings.Join(*dua, ",")
}
func (dua *stringArgsArray) Set(value string) error {
	*dua = append(*dua, value)
	return nil
}
func getAsyncData(async *Async, warnPrefix string) interface{} {
	i, err, warn := async.Get()
	if err != nil {
		panic(err)
	}
	if warn != "" {
		fmt.Println(warnPrefix + ":" + warn + "\n")
	}
	return i
}
func getDefaultExtention(url VideoUrl) string {
	return GetBestFormat(url.Formats, true, true).Ext
}
func downloadBestAndMerge(url VideoUrl, videoUtils *VideoUtils, outputFormat string, directory string, wg *sync.WaitGroup, sem *semaphore) (*Async, string) {
	var format string
	if outputFormat == "" {
		format = getDefaultExtention(url)
	} else {
		format = outputFormat
	}
	fileName := ValidateFileName(url.Name) + "." + format
	async, err := videoUtils.DownloadBestAndMerge(url, directory+string(os.PathSeparator)+fileName)
	if err != nil {
		panic(err)
	} else {
		return async, fileName
	}
}
func liveDownload(videos <-chan outputVideo, videoUtils *VideoUtils, wg *sync.WaitGroup) {
	defer wg.Done()
	var filesName []string
	maxSizeInKb := 9.8 * 1024 * 1024
	sizeSplitThreshold := 9.7 * 1024 * 1024
	maxTimeInSec := 5.5 * 60 * 60
	timeSplitThreshold := 5.4 * 60 * 60
	var downloadAsync []*Async
	for video := range videos {
		var format string
		if video.format != "" {
			format = video.format
		} else {
			format = getDefaultExtention(video.video)
		}
		fileName := video.directory + string(os.PathSeparator) + ValidateFileName(video.video.Name) + "." + format
		async, err := videoUtils.LiveDownload(video.video, GetBestFormat(video.video.Formats, true, true), fileName, int(maxSizeInKb), int(sizeSplitThreshold), int(maxTimeInSec), int(timeSplitThreshold), nil, nil)
		if err != nil {
			fmt.Println(err)
		} else {
			downloadAsync = append(downloadAsync, async)
		}
		filesName = append(filesName, video.directory+string(os.PathSeparator)+ValidateFileName(video.video.Name)+"."+format)
	}
	for i, s := range downloadAsync {
		getAsyncData(s, filesName[i])
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
	youtube := CreateYoutubeDlWrapper()
	ffmpeg := CreateFfmpegWrapper()
	videoUtils := VideoUtils{Youtube: &youtube, Ffmpeg: &ffmpeg}
	numberOfCores := runtime.NumCPU()
	sem := make(semaphore, numberOfCores)
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
	go liveDownload(liveDownChan, &videoUtils, &wg)
	for i, a := range pendingUrlAsync {
		urls := getAsyncData(a, downloads[i]).(*[]VideoUrl)
		for _, url := range *urls {
			if url.IsLive {
				liveDownChan <- outputVideo{video: url, directory: directories[i], format: outputFormat[i]}
			} else {
				as, fn := downloadBestAndMerge(url, &videoUtils, outputFormat[i], directories[i], &wg, &sem)
				pendingDownloadAsync = append(pendingDownloadAsync, as)
				pendingDownloadNames = append(pendingDownloadNames, fn)
			}
		}
	}
	for i, a := range pendingDownloadAsync {
		getAsyncData(a, pendingDownloadNames[i])
	}
	close(liveDownChan)
	wg.Wait()
}
