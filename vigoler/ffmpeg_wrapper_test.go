package vigoler

import (
	"io/ioutil"
	"math"
	"os"
	"path"
	"testing"
	"time"
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
func downloadStop(t *testing.T, outputFileName string, returnWaitError bool) error {
	ffmpeg := CreateFfmpegWrapper(10, true)
	wa, addr, err := runFFmpegTestLiveVideo(23450, 10)
	if err != nil {
		panic(err)
	}
	async, err := ffmpeg.download(nil, addr, DownloadSettings{
		SizeSplitThreshold:  999999999,
		TimeSplitThreshold:  999999999,
		CallbackBeforeSplit: func(url string, setting DownloadSettings, output string) {},
		returnWaitError:     returnWaitError,
	}, outputFileName, nil)
	clean := func() {
		_ = async.Stop()
		_ = os.Remove(outputFileName)
	}
	defer clean()
	if err != nil {
		panic(err)
	}
	err = wa.Wait()
	if err != nil {
		panic(err)
	}
	timer := time.AfterFunc(time.Duration(15)*time.Second, func() {
		clean()
		panic("Test: " + t.Name() + " timeout")
	})
	_, err, _ = async.Get()
	timer.Stop()
	_, fileErr := os.Stat(outputFileName)
	if fileErr != nil && os.IsNotExist(fileErr) {
		t.Error("downloadstop() deleted output file")
	}
	return err
}
func TestFFmpegWrapper_downloadStop(t *testing.T) {
	const outputFileName = "downloadStopTest.mp4"
	err := downloadStop(t, outputFileName, false)
	if err != ServerStopSendDataError {
		t.Fatalf("downloadstop() error = %v", err)
	}
}
func TestFFmpegWrapper_downloadStopErr(t *testing.T) {
	const outputFileName = "downloadStopTest.mp4"
	err := downloadStop(t, outputFileName, true)
	if err != ServerStopSendDataError {
		t.Fatalf("downloadstop() error = %v", err)
	}
}
