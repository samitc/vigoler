package vigoler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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
	fileSize    float64
	Ext         string
	hasVideo    bool
	hasAudio    bool
	protocol    string
	httpHeaders map[string]string
}
type VideoUrl struct {
	url          string
	Name         string
	IsLive       bool
	Formats      []Format
	idInPlaylist int
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
func createURL(urlStr string) string {
	newURL, _ := url.Parse(urlStr)
	return newURL.String()
}
func createSingleFormat(formatMap map[string]interface{}) Format {
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
	httpHeaderMap := formatMap["http_headers"].(map[string]interface{})
	httpHeaders := make(map[string]string)
	for k, v := range httpHeaderMap {
		httpHeaders[k] = v.(string)
	}
	return Format{fileSize: fileSize, url: url, formatID: formatID, Ext: ext, protocol: protocol, hasVideo: hasVideo, hasAudio: hasAudio, httpHeaders: httpHeaders}
}
func readFormats(dMap map[string]interface{}) []Format {
	mapOfFormats := dMap["formats"]
	if mapOfFormats == nil {
		return []Format{createSingleFormat(dMap)}
	}
	listOfFormats := mapOfFormats.([]interface{})
	formats := make([]Format, 0, len(listOfFormats))
	for _, format := range listOfFormats {
		formats = append(formats, createSingleFormat(format.(map[string]interface{})))
	}
	return formats
}
func getURLData(output *<-chan string, url string) ([]map[string]interface{}, string, error) {
	var mapData []map[string]interface{}
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
				mapData = append(mapData, data.(map[string]interface{}))
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
	return mapData, warnOutput, err
}
func extractDataFromMap(dMap map[string]interface{}) (string, bool) {
	const ALIVE_NAME = "is_live"
	const TITLE_NAME = "title"
	var isAlive bool
	if dMap[ALIVE_NAME] == nil {
		isAlive = false
	} else {
		isAlive = dMap[ALIVE_NAME].(bool)
	}
	return dMap[TITLE_NAME].(string), isAlive
}
func getUrls(output *<-chan string, url string) ([]VideoUrl, error, string) {
	maps, warn, err := getURLData(output, url)
	var videos []VideoUrl
	for i, dMap := range maps {
		name, isLive := extractDataFromMap(dMap)
		videos = append(videos, VideoUrl{url: url, idInPlaylist: i, Name: name, IsLive: isLive, Formats: readFormats(dMap)})
	}
	return videos, err, warn
}
func (youdown *YoutubeDlWrapper) getMetaData(url string) (*Async, *<-chan string, error) {
	ctx := context.Background()
	wa, output, err := youdown.app.runCommandChan(ctx, "-i", "-j", url)
	if err != nil {
		return nil, nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, wa)
	return &async, &output, nil
}
func (youdown *YoutubeDlWrapper) GetUrls(url string) (*Async, error) {
	async, output, err := youdown.getMetaData(url)
	if err != nil {
		return nil, err
	}
	go func() {
		defer async.wg.Done()
		async.SetResult(getUrls(output, url))
	}()
	return async, nil
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
