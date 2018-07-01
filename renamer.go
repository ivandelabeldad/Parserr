package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sonarr-parser-helper/api"
	"strings"
)

// Show ...
type Show struct {
	HistoryRecord  api.HistoryRecord
	QueueElement   api.QueueElement
	HasBeenRenamed bool
}

// FixFailedShows ...
func FixFailedShows() ([]Show, error) {
	shows, err := loadFailedShows()
	if err != nil {
		return nil, err
	}
	for _, s := range shows {
		err = s.FixNaming()
		if err != nil {
			log.Printf("error fixing show %s: %s", s.QueueElement.Title, err.Error())
		}
	}
	return shows, nil
}

// loadFailedShows ...
func loadFailedShows() ([]Show, error) {
	shows := make([]Show, 0)
	queue, err := api.GetQueue()
	if err != nil {
		return nil, err
	}
	history, err := api.GetHistory(1)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(queue); i++ {
		isNotCompleted := queue[i].Status != api.StatusCompleted
		isNotFailed := queue[i].TrackedDownloadStatus != api.TrackedDownloadStatusWarning
		if isNotCompleted || isNotFailed {
			continue
		}
		found := false
		for _, he := range history.Records {
			sameDownloadID := queue[i].DownloadID == he.DownloadID
			sameEpisode := queue[i].Episode.EpisodeNumber == he.Episode.EpisodeNumber
			sameSeason := queue[i].Episode.SeasonNumber == he.Episode.SeasonNumber
			if sameDownloadID && sameSeason && sameEpisode {
				found = true
				shows = append(shows, Show{HistoryRecord: he, QueueElement: queue[i]})
				log.Printf("failed show detected: %s", queue[i].Title)
			}
		}
		if !found {
			i--
			history, err = addPageToHistory(history)
			if err != nil {
				return nil, err
			}
		}
	}
	return shows, nil
}

// IsBroken ...
func (s Show) IsBroken() bool {
	return s.HistoryRecord.TrackedDownloadStatus == api.TrackedDownloadStatusWarning
}

func (s Show) guessFileName() (string, error) {
	if len(s.QueueElement.StatusMessages) == 1 {
		return s.QueueElement.StatusMessages[0].Title, nil
	}
	episode := s.QueueElement.Episode
	regexString := fmt.Sprintf("%d.{0,4}%d", episode.SeasonNumber, episode.EpisodeNumber)
	regex := regexp.MustCompile(regexString)
	for _, message := range s.QueueElement.StatusMessages {
		if regex.MatchString(message.Title) {
			return message.Title, nil
		}
	}
	return "", fmt.Errorf("imposible to guess file name for %s", s.QueueElement.Title)
}

func (s Show) guessFinalName(filename string) (string, error) {
	finalTitle := s.HistoryRecord.SourceTitle
	fmt.Printf("final title initial: %s\n", finalTitle)
	if len(s.QueueElement.StatusMessages) == 1 {
		return finalTitle, nil
	}
	episode := s.QueueElement.Episode
	regexString := fmt.Sprintf("[.\\-_ ]([\\-_0-9sSeExX]{2,10})[.\\-_ ]")
	regex := regexp.MustCompile(regexString)
	if !regex.MatchString(finalTitle) {
		return "", fmt.Errorf("unable to guess final episode name of %s", filename)
	}
	match := regex.FindString(finalTitle)
	new := fmt.Sprintf(".S%.2dE%.2d.", episode.SeasonNumber, episode.EpisodeNumber)
	finalTitle = strings.Replace(finalTitle, match, new, 1)
	fmt.Printf("final title final: %s\n", finalTitle)
	return finalTitle, nil
}

// FixNaming Try to rename downloaded files to the original
// torrent name.
func (s Show) FixNaming() error {
	filename, err := s.guessFileName()
	if err != nil {
		return err
	}
	oldPath, err := locationOfFile(os.Getenv(api.EnvSonarrDownloadFolder), filename)
	if err != nil {
		return err
	}
	finalName, err := s.guessFinalName(filename)
	if err != nil {
		return err
	}
	newPath := path.Join(s.QueueElement.Series.Path, finalName+filepath.Ext(oldPath))
	log.Printf("renaming %s to %s", oldPath, newPath)
	err = moveFromTo(oldPath, newPath)
	if err != nil {
		return err
	}
	s.HasBeenRenamed = true
	return nil
}

func moveFromTo(sourcePath, destPath string) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("couldn't open source file: %s", err)
	}
	outputFile, err := os.Create(destPath)
	if err != nil {
		inputFile.Close()
		return fmt.Errorf("couldn't open dest file: %s", err)
	}
	defer outputFile.Close()
	_, err = io.Copy(outputFile, inputFile)
	inputFile.Close()
	if err != nil {
		return fmt.Errorf("writing to output file failed: %s", err)
	}
	err = os.Remove(sourcePath)
	if err != nil {
		return fmt.Errorf("Failed removing original file: %s", err)
	}
	return nil
}

func addPageToHistory(h api.History) (api.History, error) {
	newPage := h.Page + 1
	newHistory, err := api.GetHistory(newPage)
	if err != nil {
		return h, err
	}
	h.Records = append(h.Records, newHistory.Records...)
	h.Page = newPage
	return h, nil
}

// locationOfFile Search recursively on root for a file with filename
// and return its path
func locationOfFile(root, filename string) (string, error) {
	var location string
	var err error
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if info.Name() == filename {
			location = path
			return fmt.Errorf("ok")
		}
		return nil
	})
	if err != nil && err.Error() == "ok" {
		err = nil
	}
	if location == "" {
		err = fmt.Errorf("%s doesn't exists inside %s", filename, root)
	}
	return location, err
}
