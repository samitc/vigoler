package vigoler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
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
	format             string
	Number             int
	FileFormat         string
	Resolution         string
	Description        string
	hasVideo, hasAudio bool
	MaxFileSizeInMb    int
}
type VideoUrl struct {
	Name    string
	IsLive  bool
	Ext     string
	Formats []Format
	url     string
}
type BadFormatError struct {
	Video  VideoUrl
	Format string
}
type HttpError struct {
	Video        string
	ErrorMessage string
}
type DownloadStatus func(url VideoUrl, percent, size float32)

func (e *BadFormatError) Error() string {
	return fmt.Sprintf("Bad format %s for url %s", e.Format, e.Video.url)
}
func (e *HttpError) Error() string {
	return fmt.Sprintf("Http errer while requested %s. error message is: %s", e.Video, e.ErrorMessage)
}
func (e *BadFormatError) Type() string {
	return "Bad format"
}
func (e *HttpError) Type() string {
	return "Http error"
}
func fillFormat(format Format) Format {
	if format.format == "" {
		format.format = strconv.Itoa(format.Number)
	}
	if format.MaxFileSizeInMb > 0 {
		format.format += "[filesize<" + strconv.Itoa(format.MaxFileSizeInMb) + "m]"
	}
	return format
}
func CreateYoutubeDlWrapper() YoutubeDlWrapper {
	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)
	wrapper := YoutubeDlWrapper{app: externalApp{appLocation: "youtube-dl"}, random: *r1}
	return wrapper
}
func (youdown *YoutubeDlWrapper) GetFormats(url VideoUrl) (*Async, error) {
	ctx := context.Background()
	wa, output, err := youdown.app.runCommandChan(ctx, "-F", url.url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, wa)
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
		async.SetResult(&formats, nil, "")
	}(&async, &output)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) GetUrls(url string) (*Async, error) {
	ctx := context.Background()
	wa, output, err := youdown.app.runCommandChan(ctx, "-i", "-j", url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, wa)
	go func(async *Async, output *<-chan string, url string) {
		defer async.wg.Done()
		const URL_NAME = "webpage_url"
		const ALIVE_NAME = "is_live"
		const TITLE_NAME = "title"
		const EXT_NAME = "ext"
		var videos []VideoUrl
		var err error
		warnOutput := ""
		videoIndex := 0
		preWarnIndex := -1
		for s := range *output {
			hasWarn := false
			errorIndex := str.Index(s, "ERROR")
			warnIndex := str.Index(s, "WARNING")
			if errorIndex != -1 || warnIndex != -1 {
				hasWarn = true
			} else {
				j := []byte(s)
				var data interface{}
				json.Unmarshal(j, &data)
				if data == nil {
					hasWarn = true
				} else {
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
			if hasWarn {
				warnVideoIndex := videoIndex
				if preWarnIndex != -1 {
					warnVideoIndex--
				}
				warnOutput += "WARN IN VIDEO NUMBER: " + strconv.Itoa(warnVideoIndex) + ". " + s
				if str.Index(s, "Unable to download webpage: HTTP Error 503") != -1 {
					err = &HttpError{Video: url}
				}
			}
			if !hasWarn || preWarnIndex == -1 {
				videoIndex++
			}
			preWarnIndex = warnIndex
		}
		async.SetResult(&videos, err, warnOutput)
	}(&async, &output, url)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) downloadUrl(url VideoUrl, format string, status DownloadStatus) (*Async, error) {
	if url.IsLive {
		return nil, errors.New("can not download live video") //TODO: make new error type
	}
	ctx := context.Background()
	outputFileName := strconv.Itoa(youdown.random.Int())
	wa, _, output, err := youdown.app.runCommand(ctx, true, true, false, "-o", outputFileName, "-f", format, url.url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, wa)
	go func(url VideoUrl, async *Async, output *<-chan string, format string, status DownloadStatus) {
		defer async.wg.Done()
		const DESTINATION = "Destination:"
		var dest = ""
		warn := ""
		var err error
		extractLineFromString := func(partString string) (nextPartString, fullString string) {
			partString = str.Replace(partString, "\r", "\n", 1)
			if i := str.Index(partString, "\n"); i >= 0 {
				i++
				return partString[i:], partString[:i]
			}
			return partString, ""
		}
		fullS := ""
		for s := range *output {
			fullS += s
			fullS, s = extractLineFromString(fullS)
			if s != "" && s != "\n" {
				if -1 != str.Index(s, "ERROR") {
					if s[:len(s)-1] == "ERROR: requested format not available" { // s contain also \n
						err = &BadFormatError{Video: url, Format: format}
						break
					} else {
						if warn == "" {
							warn += url.url + "\n"
						}
						warn = warn + s + "\n"
					}
				}
				if dest == "" {
					destIndex := str.Index(s, DESTINATION)
					if destIndex != -1 {
						dest = s[destIndex+len(DESTINATION)+1 : len(s)-1]
					}
				} else {
					if status != nil {
						//0.0% of 1.07GiB at 241.96KiB/s ETA 01:16:55
						perPos := str.Index(s, "%")
						startPerPos := str.Index(s, "]") + 1
						temp := str.Replace(s[startPerPos:perPos], " ", "", -1)
						curPer, err := strconv.ParseFloat(str.Replace(s[startPerPos:perPos], " ", "", -1), 32)
						if err != nil {
							fmt.Println(err)
							continue
						}
						sizeEndPos := perPos + 5
						for ; '0' <= s[sizeEndPos] && s[sizeEndPos] <= '9' || s[sizeEndPos] == '.'; sizeEndPos++ {
						}
						temp = s[perPos+5 : sizeEndPos]
						_ = temp
						size, err := strconv.ParseFloat(s[perPos+5:sizeEndPos], 32)
						if err != nil {
							fmt.Println(err)
							continue
						}
						if s[sizeEndPos] == 'G' {
							size *= 1024
						}
						status(url, float32(curPer), float32(size))
					}
				}
			}
		}
		if async.isStopped {
			files, dErr := filepath.Glob(dest + "*")
			if dErr != nil {
				fmt.Println(dErr)
			} else {
				for _, f := range files {
					os.Remove(f)
				}
			}
			os.Remove(dest)
			err = &CancelError{}
			dest = ""
		}
		async.SetResult(&dest, err, warn)
	}(url, &async, &output, format, status)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) Download(url VideoUrl, format Format, status DownloadStatus) (*Async, error) {
	return youdown.downloadUrl(url, fillFormat(format).format, status)
}
func (youdown *YoutubeDlWrapper) getRealVideoUrl(url VideoUrl, format Format) (*Async, error) {
	ctx := context.Background()
	wa, output, err := youdown.app.runCommandChan(ctx, "-g", "-f", fillFormat(format).format, url.url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, wa)
	go func(async *Async, output *<-chan string) {
		defer async.wg.Done()
		for s := range *output {
			if async.Result != nil {
				fmt.Println(s) //TODO: return error - should not happen
			}
			async.SetResult(&s, nil, "")
		}
	}(&async, &output)
	return &async, nil
}
func GetBestFormat(formats []Format, needVideo, needAudio bool) Format {
	for i := len(formats) - 1; i >= 0; i++ {
		if formats[i].hasVideo == needVideo && formats[i].hasAudio == needAudio {
			return formats[i]
		}
	}
	return formats[len(formats)-1]
}
