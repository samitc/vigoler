package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/samitc/vigoler/vigoler"
	"github.com/segmentio/ksuid"
)

const (
	secondsPerMinute       = 60
	secondsPerHour         = 60 * secondsPerMinute
	secondsPerDay          = 24 * secondsPerHour
	unixToInternal   int64 = (1969*365 + 1969/4 - 1969/100 + 1969/400) * secondsPerDay
)

var (
	maxTime = time.Unix(1<<63-1-unixToInternal, 999999999)
)

type video struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	IsLive     bool     `json:"is_live"`
	Ids        []string `json:"ids,omitempty"`
	ext        string
	parentID   string
	videoURL   vigoler.VideoUrl
	async      *vigoler.Async
	updateTime time.Time
	isLogged   bool
}

func (vid *video) String() string {
	_, err, warn := vid.async.Get()
	return fmt.Sprintf("id=%s, name=%s, ext=%s, url=%v,update time=%v, error=%v, warn=%s",
		vid.ID, vid.Name, vid.ext, vid.videoURL, vid.updateTime, err, warn)
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
			if v.parentID != "" {
				if val, ok := videosMap[v.parentID]; ok {
					i := 0
					size := len(val.Ids)
					for ; i < size; i++ {
						if v.ID == val.Ids[i] {
							break
						}
					}
					val.Ids[i] = val.Ids[size-1]
					val.Ids = val.Ids[:size-1]
				}
			}
			err := os.Remove(vigoler.ValidateFileName(v.Name + "." + v.ext))
			if err != nil && !os.IsNotExist(err) {
				fmt.Println(err)
			}
			delete(videosMap, k)
		}
	}
}
func createID() string {
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
	for _, url := range urls.([]vigoler.VideoUrl) {
		videos = append(videos, video{videoURL: url, ID: createID(), Name: url.Name, IsLive: url.IsLive, isLogged: false})
	}
	return videos, nil
}
func extractLiveParameter() (maxSizeInKb, sizeSplit, maxTimeInSec, timeSplit int, err error) {
	maxSizeInKb, err = validateInt(os.Getenv("VIGOLER_LIVE_MAX_SIZE"))
	if err != nil {
		return
	}
	sizeSplit, err = validateInt(os.Getenv("VIGOLER_LIVE_SPLIT_SIZE"))
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
		maxSizeInKb, sizeSplit, maxTimeInSec, timeSplit, err := extractLiveParameter()
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
					id := createID()
					vid.Ids = append(vid.Ids, id)
					videosMap[id] = &video{Name: name, ext: ext, IsLive: false, ID: id, updateTime: time.Now(), async: async, parentID: vid.ID}
				}
			}
			vid.async, err = videoUtils.LiveDownload(vid.videoURL, vigoler.GetBestFormat(vid.videoURL.Formats, true, true), vigoler.ValidateFileName(vid.Name), maxSizeInKb, sizeSplit, maxTimeInSec, timeSplit, fileDownloadedCallback, vid)
			if err != nil {
				fmt.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				json.NewEncoder(w).Encode(vid)
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
						vid.async, err = videoUtils.DownloadBest(vid.videoURL, vigoler.ValidateFileName(vid.Name))
					} else {
						vid.async, err = videoUtils.DownloadBestMaxSize(vid.videoURL, vigoler.ValidateFileName(vid.Name), sizeInKb)
					}
					if err != nil {
						fmt.Println(err)
						writeErrorToClient(w, err)
					} else {
						json.NewEncoder(w).Encode(vid)
					}
				}
			}
		}
	}
}
func checkIfVideoExist(vid *video) *string {
	for k, v := range videosMap {
		if v.Name == vid.Name {
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
	youtubeURL := readBody(r)
	videos, err := createVideos(youtubeURL)
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
		vid.updateTime = time.Now()
		if vid.async == nil || vid.async.WillBlock() {
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(vid)
		} else {
			_, err, _ := vid.async.Get()
			if !vid.isLogged {
				fmt.Println(vid)
				vid.isLogged = true
			}
			if err != nil {
				writeErrorToClient(w, err)
			} else {
				json.NewEncoder(w).Encode(vid)
			}
		}
	}
}
func download(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vid := videosMap[vars["ID"]]
	if vid == nil {
		w.WriteHeader(http.StatusNotFound)
	} else if !vid.async.WillBlock() && !vid.IsLive {
		fileName, err, warn := vid.async.Get()
		// Get file extension and remove the '.'
		vid.ext = path.Ext(fileName.(string))[1:]
		if warn != "" {
			fmt.Println(warn)
		}
		if err != nil {
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			fileName := vigoler.ValidateFileName(vid.Name + "." + vid.ext)
			file, err := os.Open(fileName)
			defer file.Close()
			if err != nil {
				fmt.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				defer func() { vid.updateTime = time.Now() }()
				vid.updateTime = maxTime
				w.Header().Set("Content-Disposition", "attachment; filename=\""+fileName+"\"")
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
func waitAndExecute(timeEnv string, exec func()) {
	exec()
	seconds, _ := strconv.Atoi(timeEnv)
	dur := time.Second * time.Duration(seconds)
	time.Sleep(dur)
}
func main() {
	you := vigoler.CreateYoutubeDlWrapper()
	ff := vigoler.CreateFfmpegWrapper()
	curl := vigoler.CreateCurlWrapper()
	videoUtils = vigoler.VideoUtils{Youtube: &you, Ffmpeg: &ff, Curl: &curl}
	videosMap = make(map[string]*video)
	router := mux.NewRouter()
	router.HandleFunc("/videos", process).Methods("POST")
	router.HandleFunc("/videos/{ID}", downloadVideo).Methods("POST")
	router.HandleFunc("/videos/{ID}", checkFileDownloaded).Methods("GET")
	router.HandleFunc("/videos/{ID}/download", download).Methods("GET")
	go func() {
		for true {
			waitAndExecute(os.Getenv("VIGOLER_CLEANER_PERIODIC"), serverCleaner)
		}
	}()
	go func() {
		for true {
			if value, ok := os.LookupEnv("VIGOLER_YOUTUBEDL_UPDATE_PERIODIC"); ok {
				waitAndExecute(value, you.UpdateYoutubeDl)
			} else {
				waitAndExecute(os.Getenv("VIGOLER_CLEANER_PERIODIC"), you.UpdateYoutubeDl)
			}
		}
	}()
	corsObj := handlers.AllowedOrigins([]string{os.Getenv("VIGOLER_ALLOW_ORIGIN")})
	certFile := os.Getenv("VIGOLER_CERT_FILE")
	keyFile := os.Getenv("VIGOLER_KEY_FILE")
	addr := os.Getenv("VIGOLER_LISTEN_ADDR")
	muxHandlers := handlers.CORS(corsObj)(router)
	isTLS := len(certFile) != 0 && len(keyFile) != 0
	if isTLS {
		log.Fatal(http.ListenAndServeTLS(addr, certFile, keyFile, muxHandlers))
	} else {
		log.Fatal(http.ListenAndServe(addr, muxHandlers))
	}
}
