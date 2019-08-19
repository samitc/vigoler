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
	"github.com/samitc/vigoler/2/vigoler"
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
	fileName   string
}

func (vid *video) StringInitData() string {
	return fmt.Sprintf("id=%s, name=%s, url=%v", vid.ID, vid.Name, vid.videoURL)
}
func (vid *video) StringData() string {
	_, err, warn := vid.async.Get()
	return fmt.Sprintf("id=%s, ext=%s, update time=%v, error=%v, warn=%s", vid.ID, vid.ext, vid.updateTime, err, warn)
}

func (vid *video) String() string {
	_, err, warn := vid.async.Get()
	return fmt.Sprintf("id=%s, name=%s, ext=%s, file name=%s, url=%v, update time=%v, error=%v, warn=%s",
		vid.ID, vid.Name, vid.ext, vid.fileName, vid.videoURL, vid.updateTime, err, warn)
}

var videosMap map[string]*video
var videoUtils vigoler.VideoUtils
var supportLive = strings.ToLower(os.Getenv("VIGOLER_SUPPORT_LIVE")) == "true"

func serverCleaner(videosMap map[string]*video, maxTimeDiff int) {
	curTime := time.Now()
	for k, v := range videosMap {
		if (int)(curTime.Sub(v.updateTime).Seconds()) > maxTimeDiff {
			if v.async != nil {
				err := v.async.Stop()
				if _, ok := err.(*vigoler.CancelError); err != nil && !ok {
					fmt.Println(v, err)
				}
				logVid(v)
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
			err := os.Remove(v.fileName)
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
func logVid(vid *video) {
	if !vid.isLogged {
		fmt.Println(vid.StringData())
		vid.isLogged = true
	}
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
		if supportLive || !url.IsLive {
			vid := video{videoURL: url, ID: createID(), Name: url.Name, IsLive: url.IsLive, isLogged: false}
			fmt.Println(vid.StringInitData())
			videos = append(videos, vid)
		}
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
func addIndexToFileName(name string) string {
	lastDot := strings.LastIndex(name, ".")
	curIndex, err := strconv.Atoi(name[lastDot+1:])
	if err != nil {
		return name + ".1"
	} else {
		return name[:lastDot+1] + strconv.Itoa(curIndex+1)
	}
}
func downloadLiveUntilNow(vid *video) error {
	async, err := videoUtils.DownloadLiveUntilNow(vid.videoURL, vigoler.GetBestFormat(vid.videoURL.Formats, true, true), "")
	if err != nil {
		return err
	}
	go func() {
		outputI, err, _ := async.Get()
		if err == nil {
			output := outputI.(string)
			ext := path.Ext(output)[1:]
			id := createID()
			vid.Ids = append(vid.Ids, id)
			nVid := &video{Name: vid.Name + ".0", fileName: output, ext: ext, IsLive: false, ID: id, updateTime: time.Now(), async: async, parentID: vid.ID}
			videosMap[id] = nVid
			fmt.Println(nVid.StringInitData())
		}
	}()
	return nil
}
func downloadVideoLive(w http.ResponseWriter, vid *video) {
	maxSizeInKb, sizeSplit, maxTimeInSec, timeSplit, err := extractLiveParameter()
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		var fileDownloadedCallback vigoler.LiveVideoCallback
		lastName := ""
		fileDownloadedCallback = func(data interface{}, fileName string, async *vigoler.Async) {
			_, err, _ = async.Get()
			if err == nil {
				vid := data.(*video)
				ext := path.Ext(fileName)[1:]
				var name string
				if lastName == "" {
					name = vid.Name
				} else {
					name = addIndexToFileName(lastName)
				}
				lastName = name
				id := createID()
				vid.Ids = append(vid.Ids, id)
				nVid := &video{Name: name, fileName: fileName, ext: ext, IsLive: false, ID: id, updateTime: time.Now(), async: async, parentID: vid.ID}
				videosMap[id] = nVid
				fmt.Println(nVid.StringInitData())
			}
		}
		vid.async, err = videoUtils.LiveDownload(vid.videoURL, vigoler.GetBestFormat(vid.videoURL.Formats, true, true), "", maxSizeInKb, sizeSplit, maxTimeInSec, timeSplit, fileDownloadedCallback, vid)
		if err != nil {
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			if strings.ToLower(os.Getenv("VIGOLER_LIVE_FROM_START")) == "true" {
				err = downloadLiveUntilNow(vid)
				if err != nil {
					fmt.Printf("Error when download live from start of id=%s, error is:%v", vid.ID, err)
				}
			}
			json.NewEncoder(w).Encode(vid)
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
					if strings.ToLower(os.Getenv("VIGOLER_DOWNLOAD_AND_MERGE")) == "true" {
						vid.async, err = videoUtils.DownloadBestAndMerge(vid.videoURL, sizeInKb, os.Getenv("VIGOLER_MERGE_FORMAT"), true)
					} else if sizeInKb == -1 {
						vid.async, err = videoUtils.DownloadBest(vid.videoURL, "")
					} else {
						vid.async, err = videoUtils.DownloadBestMaxSize(vid.videoURL, sizeInKb, "")
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
		if v.videoURL.WebPageURL == vid.videoURL.WebPageURL {
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
			logVid(vid)
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
		fileName, err, _ := vid.async.Get()
		// Get file extension and remove the '.'
		if fileName != nil {
			vid.fileName = fileName.(string)
			vid.ext = path.Ext(fileName.(string))[1:]
		}
		logVid(vid)
		if err != nil {
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			fileName := vid.Name + "." + vid.ext
			file, err := os.Open(vid.fileName)
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
	maxLiveWithoutOutput := -1
	if maxLiveWithoutOutputEnv, ok := os.LookupEnv("VIGOLER_LIVE_STOP_TIMEOUT"); ok {
		var err error
		maxLiveWithoutOutput, err = strconv.Atoi(maxLiveWithoutOutputEnv)
		if err != nil {
			panic(err)
		}
	}
	ff := vigoler.CreateFfmpegWrapper(maxLiveWithoutOutput, os.Getenv("VIGOLER_IGNORE_HTTP_REUSE_ERRORS") == "true")
	curl := vigoler.CreateCurlWrapper()
	videoUtils = vigoler.VideoUtils{Youtube: &you, Ffmpeg: &ff, Curl: &curl}
	videosMap = make(map[string]*video)
	router := mux.NewRouter()
	router.HandleFunc("/videos", process).Methods("POST")
	router.HandleFunc("/videos/{ID}", downloadVideo).Methods("POST")
	router.HandleFunc("/videos/{ID}", checkFileDownloaded).Methods("GET")
	router.HandleFunc("/videos/{ID}/download", download).Methods("GET")
	maxTimeDiff, err := strconv.Atoi(os.Getenv("VIGOLER_MAX_TIME_DIFF"))
	if err != nil {
		panic(err)
	}
	go func(maxTimeDiff int) {
		for true {
			waitAndExecute(os.Getenv("VIGOLER_CLEANER_PERIODIC"), func() {
				serverCleaner(videosMap, maxTimeDiff)
			})
		}
	}(maxTimeDiff)
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
	logName := os.Getenv("VIGOLER_LOG_NAME")
	var logFile *log.Logger
	if logName != "" {
		file, err := os.OpenFile(logName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		logFile = log.New(file, "", 0)
	}
	server := &http.Server{Addr: addr, Handler: muxHandlers, ErrorLog: logFile}
	if isTLS {
		log.Fatal(server.ListenAndServeTLS(certFile, keyFile))
	} else {
		log.Fatal(server.ListenAndServe())
	}
}
