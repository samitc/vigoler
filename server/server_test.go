package main

import (
	"github.com/samitc/vigoler/2/vigoler"
	"os"
	"reflect"
	"testing"
	"time"
	"unsafe"
)

func assertServerCleaner(t *testing.T, videosMap map[string]*video, maxTimeDiff int, expectedMap map[string]*video) {
	serverCleaner(videosMap, maxTimeDiff)
	if !reflect.DeepEqual(videosMap, expectedMap) {
		t.Errorf("serverCleaner() = %v, want %v", videosMap, expectedMap)
	}
}
func Test_serverCleaner(t *testing.T) {
	type args struct {
		videosMap   map[string]*video
		maxTimeDiff int
	}
	testMap := make(map[string]*video)
	cleanTime := time.Now().Add(-10 * time.Second)
	testMap["1"] = &video{ID: "1", fileName: "file name", ext: "mp4", Name: "name", IsLive: false, isLogged: false, updateTime: cleanTime}
	tests := []struct {
		name        string
		args        args
		expectedMap map[string]*video
	}{
		{name: "video with out async", args: args{videosMap: testMap, maxTimeDiff: 5}, expectedMap: map[string]*video{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertServerCleaner(t, tt.args.videosMap, tt.args.maxTimeDiff, tt.expectedMap)
		})
	}
}
func setResInAsync(async *vigoler.Async, res string) {
	pointerVal := reflect.ValueOf(async)
	val := reflect.Indirect(pointerVal)
	member := val.FieldByName("result")
	ptrToY := unsafe.Pointer(member.UnsafeAddr())
	realPtrToY := (*interface{})(ptrToY)
	*realPtrToY = res
}
func Test_serverCleanerVideoNotDownload(t *testing.T) {
	const fileName = "serverCleanerVideoNotDownload.test"
	f, err := os.Create(fileName)
	defer os.Remove(fileName)
	if err != nil {
		panic(err)
	}
	err = f.Close()
	if err != nil {
		panic(err)
	}
	videosMap := make(map[string]*video)
	async := &vigoler.Async{}
	setResInAsync(async, fileName)
	videosMap["1"] = &video{
		ID:         "1",
		async:      async,
		updateTime: time.Now().Add(-2 * time.Second),
	}
	assertServerCleaner(t, videosMap, 0, map[string]*video{})
	_, err = os.Stat(fileName)
	if err == nil {
		t.Errorf("serverCleaner() does not delete file when async not called.")
	}
}
func Test_duplicate(t *testing.T) {
	testMap := make(map[string]*video)
	addVideos(testMap, []video{{ID: "1", IsLive: true, videoURL: vigoler.VideoUrl{WebPageURL: "video"}, Name: "name"}})
	v1 := testMap["1"]
	addVideos(testMap, []video{{ID: "2", IsLive: true, videoURL: vigoler.VideoUrl{WebPageURL: "video"}, Name: "name"}})
	if v1 != testMap["1"] {
		t.Errorf("video duplicate add changed video in map")
	}
}
