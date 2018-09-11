package main

import (
	"fmt"
	"vigoler/youtubedl"
)

func main() {
	BASE_URL := "https://www.youtube.com/playlist?list=PL9hW1uS6HUftRY4bk3ScHu4WvvMU0wMkD"
	youtube := youtubedl.CreateYoutubeDlWrapper()
	async, err := youtube.GetUrls(BASE_URL)
	if err != nil {
		fmt.Println(err)
	}
	urls := async.Get().(*[]youtubedl.VideoUrl)
	formats, err := youtube.GetFormats((*urls)[0])
	if err != nil {
		fmt.Println(err)
	}
	video, err := youtube.DownloadBestVideo((*urls)[0])
	if err != nil {
		fmt.Println(err)
	}
	audio, err := youtube.DownloadBestAudio((*urls)[0])
	if err != nil {
		fmt.Println(err)
	}
	format, err := youtube.Download((*urls)[0], (*formats.Get().(*[]youtubedl.Format))[0])
	audio.Get()
	video.Get()
	format.Get()
	fmt.Println(*audio.Get().(*string))
}
