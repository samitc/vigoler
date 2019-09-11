package vigoler

import (
	"reflect"
	"testing"
)

func Test_reduceFormats(t *testing.T) {
	type args struct {
		url          VideoUrl
		formats      []Format
		sizeInKBytes int
	}
	formats := []Format{
		{formatID: "1", fileSize: 10000},
		{formatID: "2", fileSize: 1000},
		{formatID: "7", fileSize: -1},
		{formatID: "3", fileSize: 100},
		{formatID: "6", fileSize: -1},
		{formatID: "8", fileSize: -1},
		{formatID: "4", fileSize: 10},
		{formatID: "5", fileSize: 1},
	}
	video := VideoUrl{Name: "file"}
	tests := []struct {
		name    string
		args    args
		want    []Format
		wantErr bool
	}{
		{"no max size", args{url: video, sizeInKBytes: -1, formats: formats}, formats[0:1], false},
		{"max bigger", args{url: video, sizeInKBytes: 9999999999, formats: formats}, formats[0:1], false},
		{"max smaller", args{url: video, sizeInKBytes: 0, formats: formats}, nil, true},
		{"unknown size 1", args{url: video, sizeInKBytes: 500, formats: formats}, formats[2:4], false},
		{"unknown size 2", args{url: video, sizeInKBytes: 50, formats: formats}, formats[5:7], false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reduceFormats(tt.args.url, tt.args.formats, tt.args.sizeInKBytes)
			if (err != nil) != tt.wantErr {
				t.Errorf("reduceFormats() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reduceFormats() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVideoUtils_needToDownloadBestFormat(t *testing.T) {
	type args struct {
		bestVideoFormats            []Format
		bestAudioFormats            []Format
		bestFormats                 []Format
		mergeOnlyIfHigherResolution bool
	}
	bestFormat := []Format{{
		height: 960,
		width:  1080,
	}}
	worstFormat := []Format{{
		height: 640,
		width:  860,
	}}
	bestAudioFormats := []Format{{}}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Best already merge with highest quality and force merge",
			args: args{
				bestVideoFormats:            worstFormat,
				bestAudioFormats:            bestAudioFormats,
				bestFormats:                 bestFormat,
				mergeOnlyIfHigherResolution: false,
			},
			want: false,
		},
		{
			name: "Best already merge with highest quality",
			args: args{
				bestVideoFormats:            worstFormat,
				bestAudioFormats:            bestAudioFormats,
				bestFormats:                 bestFormat,
				mergeOnlyIfHigherResolution: true,
			},
			want: true,
		},
		{
			name: "Best is not already merge with highest quality",
			args: args{
				bestVideoFormats:            bestFormat,
				bestAudioFormats:            bestAudioFormats,
				bestFormats:                 worstFormat,
				mergeOnlyIfHigherResolution: true,
			},
			want: false,
		},
		{
			name: "Best is not already merge with highest quality and force merge",
			args: args{
				bestVideoFormats:            bestFormat,
				bestAudioFormats:            bestAudioFormats,
				bestFormats:                 worstFormat,
				mergeOnlyIfHigherResolution: false,
			},
			want: false,
		},
		{
			name: "No audio",
			args: args{
				bestVideoFormats:            bestFormat,
				bestAudioFormats:            nil,
				bestFormats:                 worstFormat,
				mergeOnlyIfHigherResolution: true,
			},
			want: true,
		},
		{
			name: "No audio with force merge",
			args: args{
				bestVideoFormats:            bestFormat,
				bestAudioFormats:            nil,
				bestFormats:                 worstFormat,
				mergeOnlyIfHigherResolution: false,
			},
			want: true,
		},
		{
			name: "No video",
			args: args{
				bestVideoFormats:            nil,
				bestAudioFormats:            bestAudioFormats,
				bestFormats:                 worstFormat,
				mergeOnlyIfHigherResolution: true,
			},
			want: true,
		},
		{
			name: "No video with force merge",
			args: args{
				bestVideoFormats:            nil,
				bestAudioFormats:            bestAudioFormats,
				bestFormats:                 worstFormat,
				mergeOnlyIfHigherResolution: false,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vu := &VideoUtils{}
			if got := vu.needToDownloadBestFormat(tt.args.bestVideoFormats, tt.args.bestAudioFormats, tt.args.bestFormats, tt.args.mergeOnlyIfHigherResolution); got != tt.want {
				t.Errorf("needToDownloadBestFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}
