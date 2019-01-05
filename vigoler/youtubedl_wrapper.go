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
	url      string
	formatID string
	// size of the file in bytes or -1 if the data is not available.
	fileSize float64
	ext      string
	hasVideo bool
	hasAudio bool
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
	return fmt.Sprintf("Http error while requested %s. error message is: %s", e.Video, e.ErrorMessage)
}
func (e *BadFormatError) Type() string {
	return "Bad format"
}
func (e *HttpError) Type() string {
	return "Http error"
}
func CreateYoutubeDlWrapper() YoutubeDlWrapper {
	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)
	wrapper := YoutubeDlWrapper{app: externalApp{appLocation: "youtube-dl"}, random: *r1}
	return wrapper
}
func readFormats(dMap map[string]interface{}) []Format {
	listOfFormats := dMap["formats"].([]interface{})
	formats := make([]Format, 0, len(listOfFormats))
	for _, format := range listOfFormats {
		formatMap := format.(map[string]interface{})
		var fileSize float64
		if formatMap["filesize"] == nil {
			fileSize = -1
		} else {
			fileSize = formatMap["filesize"].(float64)
		}
		url := formatMap["url"].(string)
		formatID, _ := formatMap["format_id"].(string)
		ext := formatMap["ext"].(string)
		hasVideo := formatMap["vcodec"] != "none"
		hasAudio := formatMap["acodec"] != "none"
		formats = append(formats, Format{fileSize: fileSize, url: url, formatID: formatID, ext: ext, hasVideo: hasVideo, hasAudio: hasAudio})
	}
	return formats
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
					videos = append(videos, VideoUrl{Ext: dMap[EXT_NAME].(string), url: dMap[URL_NAME].(string), Name: dMap[TITLE_NAME].(string), IsLive: isAlive, Formats: readFormats(dMap)})
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
func (youdown *YoutubeDlWrapper) DownloadVideoUrl(url VideoUrl, format Format, status DownloadStatus) (*Async, error) {
	if url.IsLive {
		return nil, errors.New("can not download live video") //TODO: make new error type
	}
	ctx := context.Background()
	outputFileName := strconv.Itoa(youdown.random.Int())
	wa, _, output, err := youdown.app.runCommand(ctx, true, true, false, "-o", outputFileName, "-f", format.formatID, url.url)
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
		fullS := ""
		for s := range *output {
			fullS += s
			fullS, s = extractLineFromString(fullS)
			if s != "" {
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
	}(url, &async, &output, format.formatID, status)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) GetRealVideoUrl(url VideoUrl, format Format) (*Async, error) {
	ctx := context.Background()
	wa, output, err := youdown.app.runCommandChan(ctx, "-g", "-f", format.formatID, url.url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, wa)
	go func(async *Async, output *<-chan string) {
		defer async.wg.Done()
		realVideoUrl := ""
		for s := range *output {
			if realVideoUrl != "" {
				fmt.Println(s) //TODO: return error - should not happen
			}
			realVideoUrl = s
		}
		async.SetResult(&realVideoUrl, nil, "")
	}(&async, &output)
	return &async, nil
}
func GetBestFormat(formats []Format, needVideo, needAudio bool) Format {
	return GetFormatsOrder(formats, needVideo, needAudio)[0]
}

// GetFormatsOrder Return the formats in descending from the best format to the worst format.
// needVideo and needAudio determinate if the format contain video and audio respectively.
// needVideo and needAudio can be both true.
func GetFormatsOrder(formats []Format, needVideo, needAudio bool) []Format {
	oFormats := make([]Format, 0)
	for i := len(formats) - 1; i >= 0; i-- {
		if formats[i].hasVideo == needVideo && formats[i].hasAudio == needAudio {
			oFormats = append(oFormats, formats[i])
		}
	}
	return oFormats
}
