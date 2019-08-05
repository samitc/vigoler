package main

import (
	"reflect"
	"testing"
	"time"
)

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
		{name: "video with out async", args: args{videosMap: testMap, maxTimeDiff: 5}, expectedMap: make(map[string]*video)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverCleaner(tt.args.videosMap, tt.args.maxTimeDiff)
			if !reflect.DeepEqual(tt.args.videosMap, tt.expectedMap) {
				t.Errorf("serverCleaner() = %v, want %v", tt.args.videosMap, tt.expectedMap)
			}
		})
	}
}
