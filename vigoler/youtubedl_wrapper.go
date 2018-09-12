package vigoler

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	str "strings"
	"sync"
)

type YoutubeDlWrapper struct {
	app externalApp
}
type Format struct {
	Number             int
	hasVideo, hasAudio bool
	FileFormat         string
	Resolution         string
	Description        string
}
type VideoUrl struct {
	url    string
	IsLive bool
}

func CreateYoutubeDlWrapper() YoutubeDlWrapper {
	wrapper := YoutubeDlWrapper{app: externalApp{appLocation: "youtube-dl"}}
	return wrapper
}
func (youdown *YoutubeDlWrapper) GetFormats(url VideoUrl) (*Async, error) {
	ctx := context.Background()
	output, err := youdown.app.runCommandChan(ctx, "-F", url.url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := createAsyncWaitGroup(&wg)
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
				extensionStart = str.Index(s, "extension")
				resolutionStart = str.Index(s, "resolution")
				noteStart = str.Index(s, "note")
				isHeader = false
			} else {
				num, _ := strconv.Atoi(str.Trim(s[formatCodeStart:extensionStart], " "))
				hasVideo := true
				hasAudio := true
				if str.Index(s, "video only") != -1 {
					hasAudio = false
				} else if str.Index(s, "audio only") != -1 {
					hasVideo = false
				}
				formats = append(formats, Format{Number: num, FileFormat: str.Trim(s[extensionStart:resolutionStart], " "), Resolution: str.Trim(s[resolutionStart:noteStart], " "), hasVideo: hasVideo, hasAudio: hasAudio, Description: s})
			}
		}
		async.Result = &formats
	}(&async, &output)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) GetUrls(url string) (*Async, error) {
	ctx := context.Background()
	output, err := youdown.app.runCommandChan(ctx, "-j", url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := createAsyncWaitGroup(&wg)
	go func(async *Async, output *<-chan string) {
		defer async.wg.Done()
		const URL_NAME = "\"webpage_url\": \""
		const ALIVE_NAME = "\"is_live\": "
		const URL_NAME_LEN = len(URL_NAME)
		var videos []VideoUrl
		for s := range *output {
			urlIndex := str.Index(s, URL_NAME)
			if urlIndex != -1 {
				if str.Count(s, URL_NAME) > 1 {
					fmt.Println(s) //TODO: should not happen - two video in the same line
				}
				isAlive := str.Index(s, ALIVE_NAME) + len(ALIVE_NAME)
				urlAlive := false
				if s[isAlive:isAlive+4] == "true" {
					urlAlive = true
				}
				videoUrl := s[urlIndex+URL_NAME_LEN : str.Index(s[urlIndex+URL_NAME_LEN:], "\"")+urlIndex+URL_NAME_LEN ]
				videos = append(videos, VideoUrl{url: videoUrl, IsLive: urlAlive})
			} else {
				fmt.Println(s) //TODO
			}
		}
		async.Result = &videos
	}(&async, &output)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) downloadUrl(url VideoUrl, format string) (*Async, error) {
	if url.IsLive {
		return nil, errors.New("can not download live video") //TODO: make new error type
	}
	ctx := context.Background()
	randName := fmt.Sprintf("%010d", rand.Int())
	output, err := youdown.app.runCommandChan(ctx, "-o", randName+"%(title)s.%(ext)s", "-f", format, url.url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := createAsyncWaitGroup(&wg)
	go func(async *Async, output *<-chan string) {
		defer async.wg.Done()
		const DESTINATION = "Destination:"
		var dest string
		for s := range *output {
			destIndex := str.Index(s, DESTINATION)
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
func (youdown *YoutubeDlWrapper) getRealVideoUrl(url VideoUrl, format string) (*Async, error) {
	ctx := context.Background()
	output, err := youdown.app.runCommandChan(ctx, "-g", "-f", format, url.url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := createAsyncWaitGroup(&wg)
	go func(async *Async, output *<-chan string) {
		defer async.wg.Done()
		for s := range *output {
			if async.Result != nil {
				fmt.Println(s) //TODO: return error - should not happen
			}
			async.Result = &s
		}
	}(&async, &output)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) GetRealVideoUrl(url VideoUrl, format Format) (*Async, error) {
	return youdown.getRealVideoUrl(url, strconv.Itoa(format.Number))
}
func (youdown *YoutubeDlWrapper) GetRealVideoUrlBestAudio(url VideoUrl) (*Async, error) {
	return youdown.getRealVideoUrl(url, "bestaudio")
}
func (youdown *YoutubeDlWrapper) GetRealVideoUrlBestVideo(url VideoUrl) (*Async, error) {
	return youdown.getRealVideoUrl(url, "bestvideo")
}
func (youdown *YoutubeDlWrapper) GetRealVideoUrlBest(url VideoUrl) (*Async, error) {
	return youdown.getRealVideoUrl(url, "best")
}
