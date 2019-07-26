package vigoler

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"testing"
	"time"
)

func Test_finishManagerDownload(t *testing.T) {
	tempFilesNum := []int{0, 1, 2, 3, 4, 5}
	testDownloadGo := make([]downloadGo, 0, len(tempFilesNum))
	for i := range tempFilesNum {
		testDownloadGo = append(testDownloadGo, downloadGo{index: tempFilesNum[i], buf: []byte{(byte)(tempFilesNum[i])}})
		testDownloadGo[i].index = tempFilesNum[i]
	}
	var buf bytes.Buffer
	type args struct {
		res           downloadGo
		finished      []downloadGo
		savePartIndex int
		output        io.Writer
	}
	tests := []struct {
		name    string
		args    args
		want    []downloadGo
		want1   int
		wantErr bool
	}{
		{"FirstPartLastBug", args{downloadGo{index: 0, buf: []byte{0}}, testDownloadGo[1:], 0, &buf}, []downloadGo{}, len(tempFilesNum), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := finishManagerDownload(tt.args.res, tt.args.finished, tt.args.savePartIndex, tt.args.output)
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
			if buf.Len() != len(tempFilesNum) {
				t.Errorf("finishManagerDownload() buf len is not equels")
			}
			for i := range tempFilesNum {
				curByte, _ := buf.ReadByte()
				if curByte != (byte)(tempFilesNum[i]) {
					t.Fatalf("finishManagerDownload() buf is not equels")
				}
			}
		})
	}
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
