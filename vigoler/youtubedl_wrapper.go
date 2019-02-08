package vigoler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	str "strings"
	"sync"
)

type YoutubeDlWrapper struct {
	app externalApp
}
type Format struct {
	url      string
	formatID string
	// size of the file in KB or -1 if the data is not available.
	fileSize float64
	Ext      string
	hasVideo bool
	hasAudio bool
	protocol string
}
type VideoUrl struct {
	Name    string
	IsLive  bool
	Formats []Format
	url     string
}
type HttpError struct {
	Video        string
	ErrorMessage string
}
type DownloadStatus func(url VideoUrl, percent, size float32)

func (format Format) String() string {
	return fmt.Sprintf("id=%s, size=%v, ext=%s, protocol=%s", format.formatID, format.fileSize, format.Ext, format.protocol)
}
func (e *HttpError) Error() string {
	return fmt.Sprintf("Http error while requested %s. error message is: %s", e.Video, e.ErrorMessage)
}
func (e *HttpError) Type() string {
	return "Http error"
}
func CreateYoutubeDlWrapper() YoutubeDlWrapper {
	wrapper := YoutubeDlWrapper{app: externalApp{appLocation: "youtube-dl"}}
	return wrapper
}
func (you *YoutubeDlWrapper) UpdateYoutubeDl() {
	_, _, _, err := you.app.runCommand(context.Background(), false, true, true, "-U")
	if err != nil {
		fmt.Println(err)
	}
}
func createURL(url string) string {
	urlLen := len(url)
	if urlLen > 1 && url[urlLen-1] == '\n' {
		urlLen--
	}
	if urlLen > 1 && url[urlLen-1] == '\r' {
		urlLen--
	}
	return url[:urlLen]
}
func createSingleFormat(dMap map[string]interface{}) []Format {
	url := createURL(dMap["url"].(string))
	formatID, _ := dMap["format_id"].(string)
	ext := dMap["ext"].(string)
	protocol := dMap["protocol"].(string)
	return []Format{Format{fileSize: -1, url: url, formatID: formatID, Ext: ext, protocol: protocol, hasVideo: true, hasAudio: true}}
}
func readFormats(dMap map[string]interface{}) []Format {
	mapOfFormats := dMap["formats"]
	if mapOfFormats == nil {
		return createSingleFormat(dMap)
	}
	listOfFormats := mapOfFormats.([]interface{})
	formats := make([]Format, 0, len(listOfFormats))
	for _, format := range listOfFormats {
		formatMap := format.(map[string]interface{})
		var fileSize float64
		if formatMap["filesize"] == nil {
			fileSize = -1
		} else {
			fileSize = formatMap["filesize"].(float64) / 1024
		}
		url := createURL(formatMap["url"].(string))
		formatID, _ := formatMap["format_id"].(string)
		ext := formatMap["ext"].(string)
		hasVideo := formatMap["vcodec"] != "none"
		hasAudio := formatMap["acodec"] != "none"
		protocol := formatMap["protocol"].(string)
		formats = append(formats, Format{fileSize: fileSize, url: url, formatID: formatID, Ext: ext, protocol: protocol, hasVideo: hasVideo, hasAudio: hasAudio})
	}
	return formats
}
func getUrls(output *<-chan string, url string) ([]VideoUrl, error, string) {
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
				videos = append(videos, VideoUrl{url: dMap[URL_NAME].(string), Name: dMap[TITLE_NAME].(string), IsLive: isAlive, Formats: readFormats(dMap)})
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
	return videos, err, warnOutput
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
		async.SetResult(getUrls(output, url))
	}(&async, &output, url)
	return &async, nil
}
func (youdown *YoutubeDlWrapper) GetRealUrl(url VideoUrl, format Format) (*Async, error) {
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
		realVideoURL := ""
		for s := range *output {
			if realVideoURL != "" {
				fmt.Println(s) //TODO: return error - should not happen
			}
			realVideoURL = s
		}
		realVideoURL = createURL(realVideoURL)
		async.SetResult(&realVideoURL, nil, "")
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
