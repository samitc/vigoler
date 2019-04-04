package vigoler

import (
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func Test_finishManagerDownload(t *testing.T) {
	const fileName = "test_file"
	tempFilesNum := []int{0, 1, 2, 3, 4, 5}
	for _, n := range tempFilesNum {
		file, _ := os.Create(fileName + strconv.Itoa(n))
		file.Close()
	}
	type args struct {
		res           downloadGo
		finished      []int
		savePartIndex int
		output        string
		outputFile    *os.File
	}
	tests := []struct {
		name    string
		args    args
		want    []int
		want1   int
		want2   string
		wantErr bool
	}{
		{"FirstPartLastBug", args{downloadGo{index: 0, err: nil}, tempFilesNum[1:], 0, fileName, nil}, []int{}, len(tempFilesNum), fileName, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, got2, err := finishManagerDownload(tt.args.res, tt.args.finished, tt.args.savePartIndex, tt.args.output, tt.args.outputFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("finishManagerDownload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("finishManagerDownload() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("finishManagerDownload() got1 = %v, want %v", got1, tt.want1)
			}
			if !reflect.DeepEqual(got2.Name(), tt.want2) {
				t.Errorf("finishManagerDownload() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
	os.Remove(fileName)
}

func TestCurlWrapper_downloadParts(t *testing.T) {
	curl := CreateCurlWrapper()
	timeoutFunc := func(d time.Duration, isFinish *bool, t *testing.T) {
		<-time.After(d)
		if !*isFinish {
			panic("Test: " + t.Name() + " time out")
		}
	}
	type args struct {
		url              string
		output           string
		videoSizeInBytes int
		headers          *map[string]string
	}
	fileURL := "file:///" + os.Args[0]
	tests := []struct {
		name    string
		curl    *CurlWrapper
		args    args
		wantErr bool
	}{
		{"SizeLessThenPartBug", &curl, args{fileURL, "1.out", 0, nil}, false},
		{"SizeLessThenTwoPartsBug", &curl, args{fileURL, "2.out", minPartSizeInBytes + 1, nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isFinish := false
			go timeoutFunc(10*time.Second, &isFinish, t)
			got, err := tt.curl.downloadSize(tt.args.url, tt.args.output, tt.args.videoSizeInBytes, tt.args.headers)
			if (err != nil) != tt.wantErr {
				t.Errorf("CurlWrapper.downloadParts() error = %v, wantErr %v", err, tt.wantErr)
			}
			_, err, _ = got.Get()
			if err != nil {
				t.Errorf("CurlWrapper.downloadParts() error = %v", err)
			}
			isFinish = true
		})
	}
	_ = os.Remove("1.out")
	_ = os.Remove("2.out")
}
