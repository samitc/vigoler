package main

import (
	"bytes"
	"encoding/json"
	"go.uber.org/zap"
	"io"
	"math"
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
var (
	liveFormat = os.Getenv("VIGOLER_LIVE_FORMAT")
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

var videosMap map[string]*video
var videoUtils vigoler.VideoUtils
var supportLive = strings.ToLower(os.Getenv("VIGOLER_SUPPORT_LIVE")) == "true"
var log = createLogger()

func createLogger() logger {
	l, err := zap.NewProduction(zap.WithCaller(false))
	if err != nil {
		panic(err)
	}
	return logger{logger: l}
}

func serverCleaner(videosMap map[string]*video, maxTimeDiff int) {
	curTime := time.Now()
	for k, v := range videosMap {
		if (int)(curTime.Sub(v.updateTime).Seconds()) > maxTimeDiff {
			log.deleteVideo(v)
			if v.async != nil {
				err := v.async.Stop()
				if err != nil {
					log.deleteVideoError(v, err)
				}
				err = finishAsync(v)
				if _, ok := err.(*vigoler.CancelError); err != nil && !ok {
					log.deleteVideoError(v, err)
				}
				err = os.Remove(v.fileName)
				if err != nil && !os.IsNotExist(err) {
					log.deleteVideoError(v, err)
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
func logVid(vid *video, warn string, err error) {
	if !vid.isLogged {
		log.logVideoFinish(vid, warn, err)
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
		log.warnInVideoCreate(url, warn)
	}
	if err != nil {
		return nil, err
	}
	videos := make([]video, 0)
	for _, url := range urls.([]vigoler.VideoUrl) {
		if supportLive || !url.IsLive {
			vid := video{videoURL: url, ID: createID(), Name: url.Name, IsLive: url.IsLive, isLogged: false}
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
	async, err := videoUtils.DownloadLiveUntilNow(vid.videoURL, vigoler.GetBestFormat(vid.videoURL.Formats, true, true), liveFormat)
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
			log.newVideo(nVid)
		}
	}()
	return nil
}
func downloadVideoLive(w http.ResponseWriter, vid *video) {
	maxSizeInKb, sizeSplit, maxTimeInSec, timeSplit, err := extractLiveParameter()
	if err != nil {
		panic(err)
	} else {
		lastName := ""
		fileDownloadedCallback := func(data interface{}, fileName string, async *vigoler.Async) {
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
				log.newVideo(nVid)
			}
		}
		vid.async, err = videoUtils.LiveDownload(vid.videoURL, vigoler.GetBestFormat(vid.videoURL.Formats, true, true), liveFormat, maxSizeInKb, sizeSplit, maxTimeInSec, timeSplit, fileDownloadedCallback, vid)
		if err != nil {
			log.downloadVideoError(vid, "live", err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			if strings.ToLower(os.Getenv("VIGOLER_LIVE_FROM_START")) == "true" {
				err = downloadLiveUntilNow(vid)
				if err != nil {
					log.downloadVideoError(vid, "live from start", err)
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
					panic(err)
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
						log.downloadVideoError(vid, "download", err)
						writeErrorToClient(w, err)
					} else {
						json.NewEncoder(w).Encode(vid)
					}
				}
			}
			log.startDownloadVideo(vid)
		}
	}
}
func checkIfVideoExist(videosMap map[string]*video, vid *video) *string {
	for k, v := range videosMap {
		if v.videoURL.WebPageURL == vid.videoURL.WebPageURL && v.IsLive == vid.IsLive {
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
func addVideos(videosMap map[string]*video, videos []video) {
	curTime := time.Now()
	for i := 0; i < len(videos); i++ {
		key := checkIfVideoExist(videosMap, &videos[i])
		var vid *video
		if key != nil {
			vid = videosMap[*key]
			videos[i] = *vid
		} else {
			vid = &videos[i]
			log.newVideo(vid)
			videosMap[videos[i].ID] = vid
		}
		vid.updateTime = curTime
	}
}
func videos(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(videosMap)
}
func process(w http.ResponseWriter, r *http.Request) {
	youtubeURL := readBody(r)
	videos, err := createVideos(youtubeURL)
	if err != nil {
		log.errorInVideoCreate(youtubeURL, err)
		writeErrorToClient(w, err)
	} else {
		addVideos(videosMap, videos)
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
			err := finishAsync(vid)
			if err != nil {
				writeErrorToClient(w, err)
			} else {
				json.NewEncoder(w).Encode(vid)
			}
		}
	}
}
func finishAsync(vid *video) error {
	fileName, err, warn := vid.async.Get()
	// Get file extension and remove the '.'
	if fileName != nil {
		vid.fileName = fileName.(string)
		vid.ext = path.Ext(fileName.(string))[1:]
	}
	logVid(vid, warn, err)
	return err
}
func download(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vid := videosMap[vars["ID"]]
	if vid == nil {
		w.WriteHeader(http.StatusNotFound)
	} else if !vid.async.WillBlock() && !vid.IsLive {
		err := finishAsync(vid)
		if err != nil {
			log.videoAsyncError(vid, err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			fileName := vid.Name + "." + vid.ext
			file, err := os.Open(vid.fileName)
			defer file.Close()
			if err != nil {
				log.errorOpenVideoOutputFile(vid, fileName, err)
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				defer func() { vid.updateTime = time.Now() }()
				vid.updateTime = maxTime
				w.Header().Set("Content-Disposition", "attachment; filename=\""+fileName+"\"")
				fs, err := file.Stat()
				if err != nil {
					log.errorOpenVideoOutputFile(vid, fileName, err)
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
func waitAndExecute(timeEnv string, exec func() error) {
	err := exec()
	if err != nil {
		log.logError("Error on wait and execute", err)
	}
	seconds, _ := strconv.Atoi(timeEnv)
	dur := time.Second * time.Duration(seconds)
	time.Sleep(dur)
}
func getDefaultNumericEnv(name string, defaultValue int) (int, error) {
	if env, ok := os.LookupEnv(name); ok {
		return strconv.Atoi(env)
	} else {
		return defaultValue, nil
	}
}
func main() {
	you := vigoler.CreateYoutubeDlWrapper()
	maxLiveWithoutOutput, err := getDefaultNumericEnv("VIGOLER_LIVE_STOP_TIMEOUT", -1)
	if err != nil {
		panic(err)
	}
	maxCurlErrorRetryCount, err := getDefaultNumericEnv("VIGOLER_CURL_ERROR_RETRY", 1)
	if err != nil {
		panic(err)
	}
	ff := vigoler.CreateFfmpegWrapper(maxLiveWithoutOutput, os.Getenv("VIGOLER_IGNORE_HTTP_REUSE_ERRORS") == "true")
	curl := vigoler.CreateCurlWrapper(maxCurlErrorRetryCount)
	maxRetry, err := getDefaultNumericEnv("VIGOLER_LIVE_MIN_RETRY_TIME", math.MaxInt64)
	if err != nil {
		panic(err)
	}
	videoUtils = vigoler.VideoUtils{Youtube: &you, Ffmpeg: &ff, Curl: &curl, MinLiveErrorRetryingTime: maxRetry, Log: &vigoler.Logger{Logger: log.logger}}
	videosMap = make(map[string]*video)
	router := mux.NewRouter()
	router.HandleFunc("/videos", videos).Methods(http.MethodGet)
	router.HandleFunc("/videos", process).Methods(http.MethodPost)
	router.HandleFunc("/videos/{ID}", checkFileDownloaded).Methods(http.MethodGet)
	router.HandleFunc("/videos/{ID}", downloadVideo).Methods(http.MethodPost)
	router.HandleFunc("/videos/{ID}/download", download).Methods(http.MethodGet)
	maxTimeDiff, err := strconv.Atoi(os.Getenv("VIGOLER_MAX_TIME_DIFF"))
	if err != nil {
		panic(err)
	}
	go func(maxTimeDiff int) {
		for true {
			waitAndExecute(os.Getenv("VIGOLER_CLEANER_PERIODIC"), func() error {
				serverCleaner(videosMap, maxTimeDiff)
				return nil
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
	httpLog := zap.NewStdLog(log.logger)
	server := &http.Server{Addr: addr, Handler: muxHandlers, ErrorLog: httpLog}
	if isTLS {
		panic(server.ListenAndServeTLS(certFile, keyFile))
	} else {
		panic(server.ListenAndServe())
	}
}
