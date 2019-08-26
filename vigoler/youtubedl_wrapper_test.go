package vigoler

import (
	"errors"
	"io/ioutil"
	"reflect"
	"strings"
	"testing"
)

func TestFormats(t *testing.T) {
	formatsArray := []Format{
		{formatID: "1", hasAudio: true, hasVideo: false},
		{formatID: "2", hasAudio: true, hasVideo: false},
		{formatID: "3", hasAudio: false, hasVideo: true},
		{formatID: "4", hasAudio: false, hasVideo: true},
		{formatID: "5", hasAudio: false, hasVideo: true},
		{formatID: "6", hasAudio: false, hasVideo: true},
		{formatID: "7", hasAudio: true, hasVideo: true},
		{formatID: "8", hasAudio: true, hasVideo: true},
		{formatID: "9", hasAudio: true, hasVideo: true},
		{formatID: "10", hasAudio: true, hasVideo: true},
		{formatID: "11", hasAudio: true, hasVideo: true},
		{formatID: "12", hasAudio: true, hasVideo: true},
	}
	reverse := func(rFormats []Format) []Format {
		formats := append(rFormats[:0:0], rFormats...)
		for i := len(formats)/2 - 1; i >= 0; i-- {
			opp := len(formats) - 1 - i
			formats[i], formats[opp] = formats[opp], formats[i]
		}
		return formats
	}
	type args struct {
		formats   []Format
		needVideo bool
		needAudio bool
	}
	tests := []struct {
		name string
		args args
		want []Format
	}{
		{"empty", args{make([]Format, 0), true, true}, make([]Format, 0)},
		{"audioAndVideo", args{formatsArray, true, true}, reverse(formatsArray[6:])},
		{"audio", args{formatsArray, false, true}, reverse(formatsArray[0:2])},
		{"Video", args{formatsArray, true, false}, reverse(formatsArray[2:6])},
	}
	t.Run("GetFormatsOrder", func(t *testing.T) {
		for _, tt := range tests[0 : len(tests)-1] {
			t.Run(tt.name, func(t *testing.T) {
				if got := GetFormatsOrder(tt.args.formats, tt.args.needVideo, tt.args.needAudio); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("GetFormatsOrder() = %v, want %v", got, tt.want)
				}
			})
		}
	})
	t.Run("GetBestFormat", func(t *testing.T) {
		for _, tt := range tests[1 : len(tests)-1] {
			t.Run(tt.name, func(t *testing.T) {
				if got := GetBestFormat(tt.args.formats, tt.args.needVideo, tt.args.needAudio); !reflect.DeepEqual(got, tt.want[0]) {
					t.Errorf("GetBestFormat() = %v, want %v", got, tt.want)
				}
			})
		}
	})
}
func assertString(t *testing.T, desc string, got, expected string) {
	assert(t, desc, got, expected)
}
func assertBool(t *testing.T, desc string, got, expected bool) {
	assert(t, desc, got, expected)
}
func assert(t *testing.T, desc string, got, expected interface{}) {
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("%s = %v, wanted %v", desc, got, expected)
	}
}
func idsToFormats(ids ...string) []Format {
	formats := make([]Format, 0, len(ids))
	for _, id := range ids {
		formats = append(formats, Format{
			formatID: id,
		})
	}
	return formats
}
func getUrlsTest(t *testing.T, url, vidName, inputFileName string, vidIsLive bool, formats []Format, checkOnlyIds bool, err error) {
	data, uErr := ioutil.ReadFile(inputFileName)
	if uErr != nil {
		t.Fatal(uErr)
	}
	sChan := make(chan string)
	go func() {
		for _, line := range strings.Split(string(data), "\n") {
			sChan <- line
		}
		close(sChan)
	}()
	pChan := (<-chan string)(sChan)
	vidUrls, uErr, warn := getUrls(&(pChan), url)
	if uErr != nil {
		assert(t, "getUrls error ", uErr, err)
	} else if warn != "" {
		t.Errorf("getUrls warning = %s", warn)
	}
	if len(formats) != 0 && len(vidUrls) != 1 {
		t.Errorf("getUrls number of videos = %v", len(vidUrls))
	}
	for _, vid := range vidUrls {
		assertString(t, "getUrls return video name", vid.Name, vidName)
		assertBool(t, "getUrls return isLive", vid.IsLive, vidIsLive)
		if len(formats) != len(vid.Formats) {
			t.Errorf("getUrls number of formats = %v", len(vid.Formats))
		}
		for i, f := range formats {
			if checkOnlyIds {
				assertString(t, "getUrls return format", f.formatID, vid.Formats[i].formatID)
			} else {
				assert(t, "getUrls return format", f, vid.Formats[i])
			}
		}
	}
}
func Test_getUrls(t *testing.T) {
	url := "https://openload.co/embed/video_id"
	vidName := "fmovie.2018.720p.mp4"
	vidFormatURL := "https://openload.co/stream/video_id~1548610975~192.168.0.0~u-x4488e?mime=true"
	headersMap := make(map[string]string)
	headersMap["Accept-Charset"] = "Accept-Charset"
	headersMap["User-Agent"] = "User-Agent"
	headersMap["Cookie"] = "Cookie"
	headersMap["Accept-Language"] = "Accept-Language"
	headersMap["Accept-Encoding"] = "Accept-Encoding"
	headersMap["Accept"] = "Accept"
	vidFormat := Format{url: vidFormatURL, httpHeaders: headersMap, formatID: "0", fileSize: -1, Ext: "mp4", protocol: "https", hasVideo: true, hasAudio: true, width: -1, height: -1}
	getUrlsTest(t, url, vidName, "test_files/no_formats.json", false, []Format{vidFormat}, false, nil)
}
func Test_getUrlsFormatsOrder(t *testing.T) {
	url := "https://www.youtube.com/watch?v=MUMlwUe-BCo"
	vidName := "Remember 11: The Age Of Infinity (Blind) Ep 16: Who Are You Yuni?"
	formatsOrder := []string{"139", "140", "160", "133", "134", "135", "136", "43", "18", "22"}
	getUrlsTest(t, url, vidName, "test_files/non_order_formats.json", false, idsToFormats(formatsOrder...), true, nil)
}
func Test_getUrlsError(t *testing.T) {
	url := "https://www.youtube.com/watch?v=ERROR_VIDEO"
	getUrlsTest(t, url, "", "test_files/youtube_error_output", false, []Format{}, false, errors.New("ERROR: If the owner of this video has granted you access, please sign in."))
}
