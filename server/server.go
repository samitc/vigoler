package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/samitc/vigoler/vigoler"
	"github.com/segmentio/ksuid"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type video struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Ext        string           `json:"ext"`
	IsLive     bool             `json:"is_live"`
	Ids        []string         `json:"ids,omitempty"`
	videoUrl   vigoler.VideoUrl `json:"-"`
	async      *vigoler.Async   `json:"-"`
	updateTime time.Time        `json:"-"`
	isLogged   bool             `json:"-"`
}

func (vid *video) String() string {
	_, err, warn := vid.async.Get()
	return fmt.Sprintf("id=%s, name=%s, ext=%s, url=%v,update time=%v, error=%v, warn=%s",
		vid.ID, vid.Name, vid.Ext, vid.videoUrl, vid.updateTime, err, warn)
}

var videosMap map[string]*video
var videoUtils vigoler.VideoUtils

func serverCleaner() {
	maxTimeDiff, _ := strconv.Atoi(os.Getenv("VIGOLER_MAX_TIME_DIFF"))
	curTime := time.Now()
	for k, v := range videosMap {
		if (int)(curTime.Sub(v.updateTime).Seconds()) > maxTimeDiff {
			if v.async != nil {
				err := v.async.Stop()
				if err != nil {
					fmt.Println(err)
				}
			}
			err := os.Remove(vigoler.ValidateFileName(v.Name + "." + v.Ext))
			if err != nil && !os.IsNotExist(err) {
				fmt.Println(err)
			}
			delete(videosMap, k)
		}
	}
}
func createId() string {
	return ksuid.New().String()
}
func validateInt(fileSize string) (int, error) {
	return strconv.Atoi(fileSize)
}
func readBody(r *http.Request) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(r.Body)
	return buf.String()
}
func createVideos(url string) ([]video, error) {
	async, err := videoUtils.Youtube.GetUrls(url)
	if err != nil {
		return nil, err
	}
	urls, err, warn := async.Get()
	if warn != "" {
		fmt.Println(warn)
	}
	if err != nil {
		return nil, err
	}
	videos := make([]video, 0)
	for _, url := range *urls.(*[]vigoler.VideoUrl) {
		videos = append(videos, video{videoUrl: url, ID: createId(), Name: url.Name, Ext: url.Ext, IsLive: url.IsLive, isLogged: false})
	}
	return videos, nil
}
func extractLiveParameter() (err error, maxSizeInKb, sizeSplit, maxTimeInSec, timeSplit int) {
	maxSizeInKb, err = validateInt(os.Getenv("VIGOLER_LIVE_MAX_SIZE"))
	if err != nil {
		return
	}
	sizeSplit, err = validateInt(os.Getenv("VIGOLER_LIVE_SPLIT_Size"))
	if err != nil {
		return
	}
	maxTimeInSec, err = validateInt(os.Getenv("VIGOLER_LIVE_MAX_TIME"))
	if err != nil {
		return
	}
	timeSplit, err = validateInt(os.Getenv("VIGOLER_LIVE_SPLIT_TIME"))
	if err != nil {
		return
	}
	return
}
func downloadVideoLive(w http.ResponseWriter, vid *video) {
	if strings.ToLower(os.Getenv("VIGOLER_SUPPORT_LIVE")) != "true" {
		w.WriteHeader(http.StatusMethodNotAllowed)
	} else {
		async, err := videoUtils.Youtube.GetRealVideoUrl(vid.videoUrl, vigoler.CreateBestFormat())
		if err != nil {
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			res, err, warn := async.Get()
			if err != nil {
				fmt.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				if len(warn) > 0 {
					fmt.Println(warn)
				}
				fileName := vigoler.ValidateFileName(vid.Name + "." + vid.Ext)
				err, maxSizeInKb, sizeSplit, maxTimeInSec, timeSplit := extractLiveParameter()
				if err != nil {
					fmt.Println(err)
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					var fileDownloadedCallback vigoler.LiveVideoCallback
					fileDownloadedCallback = func(data interface{}, fileName string, async *vigoler.Async) {
						_, err, _ = async.Get()
						if err == nil {
							vid := data.(*video)
							ext := path.Ext(fileName)[1:]
							name := fileName[:len(fileName)-(len(ext)+1)]
							id := createId()
							vid.Ids = append(vid.Ids, id)
							videosMap[id] = &video{Name: name, Ext: ext, IsLive: false, ID: id, updateTime: time.Now(), async: async}
						}
					}
					vid.async, err = videoUtils.LiveDownload(res.(*string), &fileName, maxSizeInKb, sizeSplit, maxTimeInSec, timeSplit, fileDownloadedCallback, vid)
					if err != nil {
						fmt.Println(err)
						w.WriteHeader(http.StatusInternalServerError)
					} else {
						json.NewEncoder(w).Encode(vid)
					}
				}
			}
		}
	}
}
func downloadVideo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vid := videosMap[vars["ID"]]
	if vid == nil {
		w.WriteHeader(http.StatusNotFound)
	} else {
		if vid.async != nil {
			json.NewEncoder(w).Encode(vid)
		} else {
			if vid.IsLive {
				downloadVideoLive(w, vid)
			} else {
				sizeInKb, err := validateInt(os.Getenv("VIGOLER_MAX_FILE_SIZE"))
				if err != nil {
					fmt.Println(err)
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					vid.updateTime = time.Now()
					if sizeInKb == -1 {
						vid.async, err = videoUtils.DownloadBest(vid.videoUrl, vigoler.ValidateFileName(vid.Name+"."+vid.Ext))
					} else {
						vid.async, err = videoUtils.DownloadBestMaxSize(vid.videoUrl, vigoler.ValidateFileName(vid.Name+"."+vid.Ext), sizeInKb/1024)
					}
					if err != nil {
						fmt.Println(err)
						w.WriteHeader(http.StatusInternalServerError)
					}
					json.NewEncoder(w).Encode(vid)
				}
			}
		}
	}
}
func checkIfVideoExist(vid *video) *string {
	for k, v := range videosMap {
		if v.Name == vid.Name && v.Ext == vid.Ext {
			return &k
		}
	}
	return nil
}
func writeErrorToClient(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	if typedError, ok := err.(vigoler.TypedError); ok {
		w.Write([]byte(typedError.Type()))
	}
}
func process(w http.ResponseWriter, r *http.Request) {
	youtubeUrl := readBody(r)
	videos, err := createVideos(youtubeUrl)
	if err != nil {
		fmt.Println(err)
		writeErrorToClient(w, err)
	} else {
		curTime := time.Now()
		for i := 0; i < len(videos); i++ {
			key := checkIfVideoExist(&videos[i])
			if key != nil {
				videos[i] = *videosMap[*key]
			}
			videos[i].updateTime = curTime
			videosMap[videos[i].ID] = &videos[i]
		}
	}
	json.NewEncoder(w).Encode(videos)
}
func checkFileDownloaded(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vid := videosMap[vars["ID"]]
	if vid == nil {
		w.WriteHeader(http.StatusNotFound)
	} else {
		if vid.async == nil || vid.async.WillBlock() {
			w.WriteHeader(http.StatusAccepted)
		}
		_, err, _ := vid.async.Get()
		if !vid.isLogged {
			fmt.Print(vid)
		}
		if err != nil {
			writeErrorToClient(w, err)
		} else {
			json.NewEncoder(w).Encode(vid)
		}
	}
}
func download(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vid := videosMap[vars["ID"]]
	if vid == nil {
		w.WriteHeader(http.StatusNotFound)
	} else if !vid.async.WillBlock() && !vid.IsLive {
		_, err, warn := vid.async.Get()
		if warn != "" {
			fmt.Println(warn)
		}
		if err != nil {
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			fileName := vigoler.ValidateFileName(vid.Name + "." + vid.Ext)
			file, err := os.Open(fileName)
			defer file.Close()
			if err != nil {
				fmt.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				vid.updateTime = time.Now()
				w.Header().Set("Content-Disposition", "attachment; filename=\""+fileName+"\"")
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
	videoUtils = vigoler.VideoUtils{Youtube: &you, Ffmpeg: &ff}
	videosMap = make(map[string]*video)
	router := mux.NewRouter()
	router.HandleFunc("/videos", process).Methods("POST")
	router.HandleFunc("/videos/{ID}", downloadVideo).Methods("POST")
	router.HandleFunc("/videos/{ID}", checkFileDownloaded).Methods("GET")
	router.HandleFunc("/videos/{ID}/download", download).Methods("GET")
	go func() {
		for true {
			seconds, _ := strconv.Atoi(os.Getenv("VIGOLER_CLEANER_PERIODIC"))
			dur := time.Second * time.Duration(seconds)
			time.Sleep(dur)
			serverCleaner()
		}
	}()
	corsObj := handlers.AllowedOrigins([]string{os.Getenv("VIGOLER_ALLOW_ORIGIN")})
	certFile := os.Getenv("VIGOLER_CERT_FILE")
	keyFile := os.Getenv("VIGOLER_KEY_FILE")
	addr := os.Getenv("VIGOLER_LISTEN_ADDR")
	muxHandlers := handlers.CORS(corsObj)(router)
	isTls := len(certFile) != 0 && len(keyFile) != 0
	if isTls {
		log.Fatal(http.ListenAndServeTLS(addr, certFile, keyFile, muxHandlers))
	} else {
		log.Fatal(http.ListenAndServe(addr, muxHandlers))
	}
}
