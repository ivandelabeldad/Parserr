package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	// APIURL ...
	APIURL = "/api"
	// APIQueueURL ...
	APIQueueURL = APIURL + "/queue"
	// APICommandURL ...
	APICommandURL = APIURL + "/command"
	// APIHistoryURL ...
	APIHistoryURL = APIURL + "/history"
	// APIEpisodeURL ...
	APIEpisodeURL = APIURL + "/episode"
	// APIMovieURL ...
	APIMovieURL = APIURL + "/movie"
	// StatusCompleted ...
	StatusCompleted = "Completed"
	// TrackedDownloadStatusWarning ...
	TrackedDownloadStatusWarning = "Warning"
	// MaxTime Max interval to check series and clean them
	MaxTime = time.Second * 30
	// CheckInterval Time between requests to check if rescan is completed
	CheckInterval = time.Second * 5
	// DefaultRetries ...
	DefaultRetries = 3
)

// Scanneable Can execute Scan to check new files
type Scanneable interface {
	ScanCommand() CommandBody
}

// DownloadFinishedChecker Can execute Scan to check new files
type DownloadFinishedChecker interface {
	CheckFinishedDownloadsCommand() CommandBody
}

// Renameable Can execute Scan to check new files
type Renameable interface {
	RenameCommand(ids []int) CommandBody
}

// DownloadScanner Can execute DownloadScan to import files manually
type DownloadScanner interface {
	DownloadScan(path string) CommandBody
}

// Config ...
type Config interface {
	GetURL() string
	GetAPIKey() string
	GetDownloadFolder() string
	GetType() string
}

// RRAPI Complete Sonarr/Radarr API
type RRAPI interface {
	Config
	Scanneable
	Renameable
	DownloadFinishedChecker
	DownloadScanner
	GetQueue() (queue []QueueElem, err error)
	DeleteQueueItem(id int) error
	GetHistory(page int) (history History, err error)
	GetEpisode(id int) (episode Episode, err error)
	GetMovie(id int) (movie Movie, err error)
	ExecuteCommand(c CommandBody) (cs CommandStatus, err error)
	ExecuteCommandAndWait(c CommandBody, retries int) (cs CommandStatus, err error)
	GetCommandStatus(id int) (cs CommandStatus, err error)
}

// API ..
type API struct {
	URL            string
	APIKey         string
	DownloadFolder string
	Type           string
}

// GetURL ...
func (a API) GetURL() string {
	return a.URL
}

// GetAPIKey ...
func (a API) GetAPIKey() string {
	return a.APIKey
}

// GetDownloadFolder ...
func (a API) GetDownloadFolder() string {
	return a.DownloadFolder
}

// GetType ...
func (a API) GetType() string {
	return a.Type
}

// Sonarr ...
type Sonarr struct{ API }

// NewSonarr Create an API
func NewSonarr(url, apiKey, downloadFolder string) Sonarr {
	return Sonarr{
		API{
			URL:            url,
			APIKey:         apiKey,
			DownloadFolder: downloadFolder,
			Type:           TypeShow,
		},
	}
}

// Radarr ...
type Radarr struct{ API }

// NewRadarr Create an API
func NewRadarr(url, apiKey, downloadFolder string) Radarr {
	return Radarr{
		API{
			URL:            url,
			APIKey:         apiKey,
			DownloadFolder: downloadFolder,
			Type:           TypeMovie,
		},
	}
}

// DownloadScan Create a command instance to force to rescan series form disk
func (s Sonarr) DownloadScan(path string) CommandBody {
	return CommandBody{Name: "DownloadedEpisodesScan", Path: path}
}

// DownloadScan Create a command instance to force to rescan movies form disk
func (r Radarr) DownloadScan(path string) CommandBody {
	panic(fmt.Errorf("radarr doesn't implement DownloadScan"))
}

// ScanCommand Create a command instance to force to rescan series form disk
func (s Sonarr) ScanCommand() CommandBody {
	return CommandBody{Name: "RescanSeries"}
}

// ScanCommand Create a command instance to force to rescan movies form disk
func (r Radarr) ScanCommand() CommandBody {
	return CommandBody{Name: "RescanMovie"}
}

// RenameCommand ...
func (s Sonarr) RenameCommand(ids []int) CommandBody {
	return CommandBody{
		Name:      "RenameSeries",
		SeriesIds: ids,
	}
}

// RenameCommand ...
func (r Radarr) RenameCommand(ids []int) CommandBody {
	return CommandBody{
		Name:     "RenameMovies",
		MovieIds: ids,
	}
}

// NewAPI Return an instance of an API
func NewAPI(url, apiKey, downloadFolder, apiType string) RRAPI {
	if apiType == TypeMovie {
		return Radarr{
			API{
				URL:            url,
				APIKey:         apiKey,
				DownloadFolder: downloadFolder,
				Type:           apiType,
			},
		}
	}
	return Sonarr{
		API{
			URL:            url,
			APIKey:         apiKey,
			DownloadFolder: downloadFolder,
			Type:           apiType,
		},
	}
}

// CheckFinishedDownloadsCommand ...
func (a API) CheckFinishedDownloadsCommand() CommandBody {
	return CommandBody{
		Name: "CheckForFinishedDownload",
	}
}

// GetQueue ...
func (a API) GetQueue() (queue []QueueElem, err error) {
	body, err := get(a.getURL(APIQueueURL).String())
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &queue)
	return
}

// DeleteQueueItem ...
func (a API) DeleteQueueItem(id int) (err error) {
	u := a.getURL(APIQueueURL + "/" + strconv.Itoa(id)).String()
	client := &http.Client{}
	req, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return
	}
	res, err := client.Do(req)
	if err != nil {
		return
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("error deleting item from queue, status code %d", res.StatusCode)
	}
	return nil
}

// GetHistory ...
func (a API) GetHistory(page int) (history History, err error) {
	u := a.getURL(APIHistoryURL)
	query := u.Query()
	query.Add("page", strconv.Itoa(page))
	query.Add("pageSize", "10")
	u.RawQuery = query.Encode()
	body, err := get(u.String())
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &history)
	if history.PageSize == 0 {
		return history, fmt.Errorf("history fetched 0 results, no more items")
	}
	return
}

// GetEpisode ...
func (a API) GetEpisode(id int) (episode Episode, err error) {
	u := a.getURL(APIEpisodeURL + "/" + strconv.Itoa(id))
	body, err := get(u.String())
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &episode)
	return
}

// GetMovie ...
func (a API) GetMovie(id int) (movie Movie, err error) {
	u := a.getURL(APIMovieURL + "/" + strconv.Itoa(id))
	body, err := get(u.String())
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &movie)
	return
}

// ExecuteCommand ...
func (a API) ExecuteCommand(c CommandBody) (cs CommandStatus, err error) {
	log.Printf("executing: %s", c.Name)
	j, err := json.Marshal(c)
	if err != nil {
		return
	}
	body, err := post(a.getURL(APICommandURL).String(), bytes.NewReader(j))
	err = json.Unmarshal(body, &cs)
	return
}

// ExecuteCommandAndWait ...
func (a API) ExecuteCommandAndWait(c CommandBody, retries int) (cs CommandStatus, err error) {
	for i := 0; i < retries; i++ {
		cs, err = a.ExecuteCommand(c)
		if err != nil {
			continue
		}
		totalWait := CheckInterval
		for totalWait <= MaxTime {
			time.Sleep(CheckInterval)
			cs, err = a.GetCommandStatus(cs.ID)
			if err == nil {
				if cs.State == CommandStateCompleted {
					log.Printf("finished %s successfully", c.Name)
					return
				}
				log.Printf("waiting response from %s", c.Name)
			}
			totalWait += CheckInterval
		}
		if i != retries-1 {
			log.Printf("timeout, retring another time: %d of %d", i+1, retries)
		}
	}
	return cs, fmt.Errorf("timeout checking command %s, not completed", c.Name)
}

// GetCommandStatus ...
func (a API) GetCommandStatus(id int) (cs CommandStatus, err error) {
	u := a.getURL(APICommandURL + "/" + strconv.Itoa(id))
	body, err := get(u.String())
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &cs)
	return
}

// get Wrapper for http.Get. Add authentication handling automatically.
func get(u string) (body []byte, err error) {
	res, err := http.Get(u)
	if err != nil {
		return
	}
	if res.StatusCode == 401 {
		return nil, fmt.Errorf("authorization invalid")
	}
	defer res.Body.Close()
	body, err = ioutil.ReadAll(res.Body)
	return
}

// post Wrapper for http.Post. Add authentication handling automatically.
func post(u string, bodyReq io.Reader) (body []byte, err error) {
	res, err := http.Post(u, "application/json", bodyReq)
	if err != nil {
		return
	}
	if res.StatusCode == 401 {
		return nil, fmt.Errorf("authorization invalid")
	}
	defer res.Body.Close()
	body, err = ioutil.ReadAll(res.Body)
	return
}

func (a API) getURL(path string) *url.URL {
	u := &url.URL{
		Scheme: "http",
		Host:   a.URL,
		Path:   path,
	}
	q := u.Query()
	q.Set("apikey", a.APIKey)
	u.RawQuery = q.Encode()
	return u
}
