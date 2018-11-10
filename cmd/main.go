package main

import (
	"flag"
	"fmt"
	. "github.com/samitc/vigoler/vigoler"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type empty struct{}
type semaphore chan empty
type stringArgsArray []string
type outputVideo struct {
	video     VideoUrl
	directory string
	format    string
}
type downloadVideoMetadata struct {
	video, audio *Async
	directory    string
	fileName     string
	url          VideoUrl
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
func downloadBestAndMerge(url VideoUrl, youtube *YoutubeDlWrapper, ffmpeg *FFmpegWrapper, outputFormat string, directory string, wg *sync.WaitGroup, sem *semaphore) {
	video, err := youtube.DownloadBestVideo(url)
	if err != nil {
		fmt.Println(err)
	} else {
		audio, err := youtube.DownloadBestAudio(url)
		if err != nil {
			fmt.Println(err)
		} else {
			var format string
			if outputFormat == "" {
				format = url.Ext
			} else {
				format = outputFormat
			}
			wg.Add(1)
			go func(metadata downloadVideoMetadata) {
				defer wg.Done()
				metadata.audio.Get()
				metadata.video.Get()
				if metadata.audio.Error != nil || metadata.video.Error != nil {
					err := metadata.audio.Error
					if metadata.video.Error != nil {
						err = metadata.video.Error
					}
					if _, ok := err.(*BadFormatError); ok {
						downloadBest(metadata.url, youtube, metadata.fileName, metadata.directory)
					} else {
						panic(err)
					}
				} else {
					audioPath := getAsyncData(metadata.audio, metadata.fileName).(*string)
					videoPath := getAsyncData(metadata.video, metadata.fileName).(*string)
					sem.Wait()
					merge, err := ffmpeg.Merge(metadata.directory+string(os.PathSeparator)+metadata.fileName, *videoPath, *audioPath)
					if err != nil {
						fmt.Println(err)
					}
					getAsyncData(merge, metadata.fileName)
					sem.Signal()
					if _, err := os.Stat(metadata.directory + string(os.PathSeparator) + metadata.fileName); err == nil || os.IsExist(err) {
						os.Remove(*audioPath)
						os.Remove(*videoPath)
					}
				}
			}(downloadVideoMetadata{directory: directory, video: video, audio: audio, fileName: validateFileName(url.Name) + "." + format, url: url})
		}
	}
}
func downloadBest(url VideoUrl, youtube *YoutubeDlWrapper, fileName string, directory string) {
	async, err := youtube.DownloadBest(url)
	if err != nil {
		panic(err)
	}
	videoPath := getAsyncData(async, fileName)
	os.Rename(*videoPath.(*string), directory+string(os.PathSeparator)+fileName)
}
func liveDownload(videos <-chan outputVideo, youtube *YoutubeDlWrapper, ffmpeg *FFmpegWrapper, wg *sync.WaitGroup) {
	defer wg.Done()
	var pendingAsync []*Async
	var filesName []string
	for video := range videos {
		async, err := youtube.GetRealVideoUrlBest(video.video)
		if err != nil {
			fmt.Println(err)
		} else {
			pendingAsync = append(pendingAsync, async)
			var format string
			if video.format != "" {
				format = video.format
			} else {
				format = video.video.Ext
			}
			filesName = append(filesName, video.directory+string(os.PathSeparator)+validateFileName(video.video.Name)+"."+format)
		}
	}
	maxSizeInKb := 9.8 * 1024 * 1024
	sizeSplitThreshold := 9.7 * 1024 * 1024
	maxTimeInSec := 5.5 * 60 * 60
	timeSplitThreshold := 5.4 * 60 * 60
	splitCallback := func(url string, setting DownloadSettings, output string) {
		wg.Add(1)
		defer wg.Done()
		lastDot := strings.LastIndex(output, ".")
		preLastDot := strings.LastIndex(output[:lastDot], ".")
		if preLastDot == -1 {
			output = output[:lastDot] + ".1" + output[lastDot:]
		} else {
			num, err := strconv.Atoi(output[preLastDot+1 : lastDot])
			if err != nil {
				output = output[:lastDot] + ".1" + output[lastDot:]
			} else {
				output = output[:preLastDot] + "." + strconv.Itoa(num+1) + output[lastDot:]
			}
		}
		async, err := ffmpeg.Download(url, setting, output)
		if err != nil {
			fmt.Println(err)
		}
		getAsyncData(async, url)
	}
	var downloadAsync []*Async
	for i, video := range pendingAsync {
		videoUrl := getAsyncData(video, filesName[i]).(*string)
		async, err := ffmpeg.Download(*videoUrl, DownloadSettings{MaxSizeInKb: int(maxSizeInKb), MaxTimeInSec: int(maxTimeInSec), SizeSplitThreshold: int(sizeSplitThreshold), TimeSplitThreshold: int(timeSplitThreshold), CallbackBeforeSplit: splitCallback}, filesName[i])
		if err != nil {
			fmt.Println(err)
		} else {
			downloadAsync = append(downloadAsync, async)
		}
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
	go liveDownload(liveDownChan, &youtube, &ffmpeg, &wg)
	for i, a := range pendingUrlAsync {
		urls := getAsyncData(a, downloads[i]).(*[]VideoUrl)
		for _, url := range *urls {
			if url.IsLive {
				liveDownChan <- outputVideo{video: url, directory: directories[i], format: outputFormat[i]}
			} else {
				downloadBestAndMerge(url, &youtube, &ffmpeg, outputFormat[i], directories[i], &wg, &sem)
			}
		}
	}
	close(liveDownChan)
	wg.Wait()
	//time.Sleep(20 * time.Second)
	//a := 0
	//_ = a
}
