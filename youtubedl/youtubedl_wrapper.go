package youtubedl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type YoutubeDlWrapper struct {
	appLocation string
}
type Format struct {
	Number             int
	hasVideo, hasAudio bool
	FileFormat         string
	Resolution         string
	Description        string
}
type VideoUrl struct {
	url string
}

func CreateYoutubeDlWrapper() YoutubeDlWrapper {
	wrapper := YoutubeDlWrapper{appLocation: "youtube-dl"}
	return wrapper
}
func (youdown *YoutubeDlWrapper) runCommand(ctx context.Context, arg ...string) (<-chan string, error) {
	cmd := exec.CommandContext(ctx, youdown.appLocation, arg...)
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer writer.Close()
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	outputChannel := make(chan string)
	go func(outChan chan string, reader io.ReadCloser) {
		defer reader.Close()
		rd := bufio.NewReader(reader)
		str, err := rd.ReadString('\n')
		for ; err == nil; str, err = rd.ReadString('\n') {
			outChan <- str
		}
		if err != io.EOF {
			panic(err)
		}
		close(outChan)
	}(outputChannel, reader)
	return outputChannel, nil
}
func (youdown *YoutubeDlWrapper) GetFormats(url VideoUrl) (*Async, error) {
	ctx := context.Background()
	output, err := youdown.runCommand(ctx, "-F", url.url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsync(&wg)
	go func(async *Async, output *<-chan string) {
		defer async.wg.Done()
		isHeader := true
		var formats []Format
		var formatCodeStart, extensionStart, resolutionStart, noteStart int
		for s := range *output {
			if s[0] == '[' {
				continue
			}
			if isHeader {
				formatCodeStart = 0
				extensionStart = strings.Index(s, "extension")
				resolutionStart = strings.Index(s, "resolution")
				noteStart = strings.Index(s, "note")
				isHeader = false
			} else {
				num, _ := strconv.Atoi(strings.Trim(s[formatCodeStart:extensionStart], " "))
				hasVideo := true
				hasAudio := true
				if strings.Index(s, "video only") != -1 {
					hasAudio = false
				} else if strings.Index(s, "audio only") != -1 {
					hasVideo = false
				}
				formats = append(formats, Format{Number: num, FileFormat: strings.Trim(s[extensionStart:resolutionStart], " "), Resolution: strings.Trim(s[resolutionStart:noteStart], " "), hasVideo: hasVideo, hasAudio: hasAudio, Description: s})
			}
		}
		async.Result = &formats
	}(&async, &output)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) GetUrls(url string) (*Async, error) {
	ctx := context.Background()
	output, err := youdown.runCommand(ctx, "-j", url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsync(&wg)
	go func(async *Async, output *<-chan string) {
		defer async.wg.Done()
		const URL_NAME = "\"webpage_url\": \""
		const URL_NAME_LEN = len(URL_NAME)
		var videos []VideoUrl
		for s := range *output {
			urlIndex := strings.Index(s, URL_NAME)
			if urlIndex != -1 {
				videoUrl := s[urlIndex+URL_NAME_LEN : strings.Index(s[urlIndex+URL_NAME_LEN:], "\"")+urlIndex+URL_NAME_LEN ]
				videos = append(videos, VideoUrl{url: videoUrl})
			} else {
				fmt.Println(s)
			}
		}
		async.Result = &videos
	}(&async, &output)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) downloadUrl(url VideoUrl, format string) (*Async, error) {
	ctx := context.Background()
	randName := fmt.Sprintf("%010d", rand.Int())
	output, err := youdown.runCommand(ctx, "-o", randName+"%(title)s.%(ext)s", "-f", format, url.url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsync(&wg)
	go func(async *Async, output *<-chan string) {
		defer async.wg.Done()
		const DESTINATION = "Destination:"
		var dest string
		for s := range *output {
			destIndex := strings.Index(s, DESTINATION)
			if destIndex != -1 {
				dest = s[destIndex+len(DESTINATION)+1 : len(s)-1]
			}
		}
		async.Result = &dest
	}(&async, &output)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) Download(url VideoUrl, format Format) (*Async, error) {
	return youdown.downloadUrl(url, strconv.Itoa(format.Number))
}
func (youdown *YoutubeDlWrapper) DownloadBestAudio(url VideoUrl) (*Async, error) {
	return youdown.downloadUrl(url, "bestaudio")
}
func (youdown *YoutubeDlWrapper) DownloadBestVideo(url VideoUrl) (*Async, error) {
	return youdown.downloadUrl(url, "bestvideo")
}
func (youdown *YoutubeDlWrapper) DownloadBest(url VideoUrl) (*Async, error) {
	return youdown.downloadUrl(url, "best")
}
