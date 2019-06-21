package vigoler

import (
	"io/ioutil"
	"math"
	"path"
	"testing"
)

func readTestFile(fileName string) string {
	data, err := ioutil.ReadFile(path.Join("test_files", fileName))
	if err != nil {
		panic(err)
	}
	return string(data)
}
func Test_checkIsSeekable(t *testing.T) {
	type args struct {
		liveDesc string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "seekable", args: args{liveDesc: readTestFile("seekable")}, want: true},
		{name: "not seekable", args: args{liveDesc: readTestFile("not_seekable")}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkIsSeekable(tt.args.liveDesc); got != tt.want {
				t.Errorf("checkIsSeekable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_countLength(t *testing.T) {
	type args struct {
		liveDesc string
	}
	tests := []struct {
		name    string
		args    args
		want    float64
		wantErr bool
	}{
		{name: "not seekable", args: args{liveDesc: readTestFile("not_seekable")}, want: 44.9, wantErr: false},
		{name: "seekable", args: args{liveDesc: readTestFile("seekable")}, want: 7229.9, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := countLength(tt.args.liveDesc)
			if (err != nil) != tt.wantErr {
				t.Errorf("countLength() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("countLength() = %v, want %v", got, tt.want)
			}
		})
	}
}
