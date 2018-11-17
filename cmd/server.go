package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/samitc/vigoler/vigoler"
	"github.com/segmentio/ksuid"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
)

type video struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Ext      string           `json:"ext"`
	videoUrl vigoler.VideoUrl `json:"-"`
	async    *vigoler.Async   `json:"-"`
}

var videosMap map[string]*video
var videoUtils VideoUtils

func validateMaxFileSize(fileSize string) (int, error) {
	return strconv.Atoi(fileSize)

}
func readBody(r *http.Request) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(r.Body)
	return buf.String()
}
func downloadVideo(w http.ResponseWriter, r *http.Request) {
	size, err := validateMaxFileSize(os.Getenv("VIGOLER_MAX_FILE_SIZE"))
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		format := vigoler.CreateBestFormat()
		format.MaxFileSizeInMb = size
		vars := mux.Vars(r)
		vid := videosMap[vars["ID"]]
		vid.async, err = videoUtils.DownloadBestMaxSize(vid.videoUrl, validateFileName(vid.Name+"."+vid.Ext), size)
		if err != nil {
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		json.NewEncoder(w).Encode(vid)
	}
}
func createVideos(url string) ([]video, error) {
	async, err := videoUtils.Youtube.GetUrls(url)
	if err != nil {
		return nil, err
	}
	urls, err, warn := async.Get()
	if err != nil {
		return nil, err
	}
	if warn != "" {
		fmt.Println(warn)
	}
	videos := make([]video, 0)
	for _, url := range *urls.(*[]vigoler.VideoUrl) {
		videos = append(videos, video{videoUrl: url, ID: ksuid.New().String(), Name: url.Name, Ext: url.Ext})
	}
	return videos, nil
}
func process(w http.ResponseWriter, r *http.Request) {
	youtubeUrl := readBody(r)
	videos, err := createVideos(youtubeUrl)
	if err != nil {
		fmt.Println(err)
	} else {
		for _, vid := range videos {
			videosMap[vid.ID] = &vid
		}
	}
	json.NewEncoder(w).Encode(videos)
}
func download(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vid := videosMap[vars["ID"]]
	if vid == nil {
		w.WriteHeader(http.StatusNotFound)
	} else if !vid.async.WillBlock() {
		_, err, warn := vid.async.Get()
		if warn != "" {
			fmt.Println(warn)
		}
		if err != nil {
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			fileName := validateFileName(vid.Name + "." + vid.Ext)
			file, err := os.Open(fileName)
			if err != nil {
				fmt.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
				w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
				fs, err := file.Stat()
				if err != nil {
					fmt.Println(err)
				} else {
					w.Header().Set("Content-Length", strconv.FormatInt(fs.Size(), 10))
				}
				io.Copy(w, file)
			}

		}
	} else {
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(vid)
	}
}
func main() {
	you := vigoler.CreateYoutubeDlWrapper()
	ff := vigoler.CreateFfmpegWrapper()
	videoUtils = VideoUtils{Youtube: &you, Ffmpeg: &ff}
	videosMap = make(map[string]*video)
	router := mux.NewRouter()
	router.HandleFunc("/videos", process).Methods("GET")
	router.HandleFunc("/videos/{ID}", downloadVideo).Methods("POST")
	router.HandleFunc("/videos/{ID}", download).Methods("GET")
	log.Fatal(http.ListenAndServe(":8000", router))
}
