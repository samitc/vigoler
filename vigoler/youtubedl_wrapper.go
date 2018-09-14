package vigoler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	str "strings"
	"sync"
	"time"
)

type YoutubeDlWrapper struct {
	app    externalApp
	random rand.Rand
}
type Format struct {
	Number             int
	FileFormat         string
	Resolution         string
	Description        string
	hasVideo, hasAudio bool
}
type VideoUrl struct {
	Name   string
	IsLive bool
	Ext    string
	url    string
}

func CreateYoutubeDlWrapper() YoutubeDlWrapper {
	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)
	wrapper := YoutubeDlWrapper{app: externalApp{appLocation: "youtube-dl"}, random: *r1}
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
		async.setResult(&formats, nil, "")
	}(&async, &output)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) GetUrls(url string) (*Async, error) {
	ctx := context.Background()
	output, err := youdown.app.runCommandChan(ctx, "-i", "-j", url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := createAsyncWaitGroup(&wg)
	go func(async *Async, output *<-chan string) {
		defer async.wg.Done()
		const URL_NAME = "webpage_url"
		const ALIVE_NAME = "is_live"
		const TITLE_NAME = "title"
		const EXT_NAME = "ext"
		var videos []VideoUrl
		warnOutput := ""
		for s := range *output {
			errorIndex := str.Index(s, "ERROR")
			warnIndex := str.Index(s, "WARNING")
			if errorIndex != -1 || warnIndex != -1 {
				warnOutput += s
			} else {
				j := []byte(s)
				var data interface{}
				json.Unmarshal(j, &data)
				dMap := data.(map[string]interface{})
				var isAlive bool
				if dMap[ALIVE_NAME] == nil {
					isAlive = false
				} else {
					isAlive = dMap[ALIVE_NAME].(bool)
				}
				videos = append(videos, VideoUrl{Ext: dMap[EXT_NAME].(string), url: dMap[URL_NAME].(string), Name: dMap[TITLE_NAME].(string), IsLive: isAlive})
			}
		}
		async.setResult(&videos, nil, warnOutput)
	}(&async, &output)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) downloadUrl(url VideoUrl, format string) (*Async, error) {
	if url.IsLive {
		return nil, errors.New("can not download live video") //TODO: make new error type
	}
	ctx := context.Background()
	randName := fmt.Sprintf("%010d", youdown.random.Int())
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
		async.setResult(&dest, nil, "")
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
			async.setResult(&s, nil, "")
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
