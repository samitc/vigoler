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
