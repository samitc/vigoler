package vigoler

import (
	"io/ioutil"
	"reflect"
	"testing"
)

func TestFormats(t *testing.T) {
	formatsArray := []Format{
		Format{formatID: "1", hasAudio: true, hasVideo: false},
		Format{formatID: "2", hasAudio: true, hasVideo: false},
		Format{formatID: "3", hasAudio: false, hasVideo: true},
		Format{formatID: "4", hasAudio: false, hasVideo: true},
		Format{formatID: "5", hasAudio: false, hasVideo: true},
		Format{formatID: "6", hasAudio: false, hasVideo: true},
		Format{formatID: "7", hasAudio: true, hasVideo: true},
		Format{formatID: "8", hasAudio: true, hasVideo: true},
		Format{formatID: "9", hasAudio: true, hasVideo: true},
		Format{formatID: "10", hasAudio: true, hasVideo: true},
		Format{formatID: "11", hasAudio: true, hasVideo: true},
		Format{formatID: "12", hasAudio: true, hasVideo: true},
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
	if got != expected {
		t.Errorf("%s = %v, wanted = %v", desc, got, expected)
	}
}
func assertBool(t *testing.T, desc string, got, expected bool) {
	if got != expected {
		t.Errorf("%s = %v, wanted = %v", desc, got, expected)
	}
}
func assert(t *testing.T, desc string, got, expected interface{}) {
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("%s = %v, wanted %v", desc, got, expected)
	}
}
func Test_getUrls(t *testing.T) {
	sChan := make(chan string)
	url := "https://openload.co/embed/video_id"
	vidName := "fmovie.2018.720p.mp4"
	vidFormatURL := "https://openload.co/stream/video_id~1548610975~192.168.0.0~u-x4488e?mime=true"
	vidIsLive := false
	headersMap := make(map[string]string)
	headersMap["Accept-Charset"] = "Accept-Charset"
	headersMap["User-Agent"] = "User-Agent"
	headersMap["Cookie"] = "Cookie"
	headersMap["Accept-Language"] = "Accept-Language"
	headersMap["Accept-Encoding"] = "Accept-Encoding"
	headersMap["Accept"] = "Accept"
	vidFormat := Format{url: vidFormatURL, httpHeaders: headersMap, formatID: "0", fileSize: -1, Ext: "mp4", protocol: "https", hasVideo: true, hasAudio: true}
	dat, err := ioutil.ReadFile("test_files/no_formats.json")
	if err != nil {
		t.Error(err)
		return
	}
	go func() {
		sChan <- string(dat)
		close(sChan)
	}()
	pChan := (<-chan string)(sChan)
	vidUrls, err, warn := getUrls(&(pChan), url)
	if err != nil {
		t.Errorf("getUrls error = %v", err)
		return
	}
	if warn != "" {
		t.Errorf("getUrls warning = %s", warn)
	}
	if len(vidUrls) != 1 {
		t.Errorf("getUrls number of videos = %v", len(vidUrls))
	}
	for _, vid := range vidUrls {
		assertString(t, "getUrls return video name", vid.Name, vidName)
		assertBool(t, "getUrls return isLive", vid.IsLive, vidIsLive)
		if len(vid.Formats) != 1 {
			t.Errorf("getUrls number of formats = %v", len(vidUrls))
		}
		if len(vid.Formats) > 0 {
			assert(t, "getUrls return format", vid.Formats[0], vidFormat)
		}
	}
}
