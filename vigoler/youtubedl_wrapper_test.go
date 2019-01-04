package vigoler

import (
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
