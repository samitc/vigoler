package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	. "github.com/samitc/vigoler/2/vigoler"
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
func validateFileName(fileName string) string {
	notAllowCh := []string{`\`, `/`, `:`, `|`, `?`, `"`, `*`, `<`, `>`}
	for _, ch := range notAllowCh {
		fileName = strings.Replace(fileName, ch, "", -1)
	}
	return fileName
}
func downloadBestAndMerge(url VideoUrl, videoUtils *VideoUtils, outputFormat string, directory string, wg *sync.WaitGroup, sem *semaphore) (*Async, string) {
	fileName := directory + string(os.PathSeparator) + validateFileName(url.Name)
	async, err := videoUtils.DownloadBestAndMerge(url, -1, outputFormat)
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
		async, err := videoUtils.LiveDownload(video.video, GetBestFormat(video.video.Formats, true, true), video.format, int(maxSizeInKb), int(sizeSplitThreshold), int(maxTimeInSec), int(timeSplitThreshold), nil, nil)
		if err != nil {
			fmt.Println(err)
		} else {
			downloadAsync = append(downloadAsync, async)
		}
		filesName = append(filesName, video.directory+string(os.PathSeparator)+validateFileName(video.video.Name))
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
	youtube := CreateYoutubeDlWrapper()
	ffmpeg := CreateFfmpegWrapper(-1)
	curl := CreateCurlWrapper()
	videoUtils := VideoUtils{Youtube: &youtube, Ffmpeg: &ffmpeg, Curl: &curl}
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
		urls := getAsyncData(a, downloads[i]).([]VideoUrl)
		for _, url := range urls {
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
		output := getAsyncData(a, pendingDownloadNames[i])
		os.Rename(output.(string), pendingDownloadNames[i])
	}
	close(liveDownChan)
	wg.Wait()
}
