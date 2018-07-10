package download

import (
	"fmt"
	"time"
	"strings"
	"github.com/cavaliercoder/grab"
	"github.com/sethgrid/curse"
	"io"
	"os"
)

type BatchDownloader struct {
	DownloadClient *grab.Client
}

type ErrorDownload struct {
	ShouldRetry bool
	Error       error
	Requests    []IProxyRequest
}

func NewBatchDownloader() *BatchDownloader {
	return &BatchDownloader{DownloadClient: grab.NewClient()}
}

func (c *BatchDownloader) Do (requests ...IProxyRequest) ErrorDownload {
	var realGrabRequests []*grab.Request
	for _, request := range requests {
		realGrabRequests = append(realGrabRequests, request.Wrapped())
	}

	responseChannel := c.DownloadClient.DoBatch(-1, realGrabRequests...)
	proxyResponseChannel := NewChannelProxy(responseChannel)

	return WaitForComplete(proxyResponseChannel.Channel)
}

func MapToErrorDownload(downloadResponses []IProxyResponse) ErrorDownload {
	var failedDownloadRequests []IProxyRequest
	var errstrings []string
	shouldRetry := true
	for _, downloadResponse := range downloadResponses {
		if downloadResponse.Err() != nil {
			failedDownloadRequests = append(failedDownloadRequests, downloadResponse.Request())
			errstrings = append(errstrings, downloadResponse.Err().Error())
			shouldRetry = shouldRetry && downloadResponse.DidTimeout()
		}
	}

	var error error
	if len(errstrings) > 0 {
		error = fmt.Errorf(strings.Join(errstrings, "\n"))
	} else {
		error = nil
		shouldRetry = false
	}

	return ErrorDownload{
		Requests:    failedDownloadRequests,
		Error:       error,
		ShouldRetry: shouldRetry,
	}
}

func WaitForComplete(channelProxy <-chan IProxyResponse) ErrorDownload{
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	return ProcessDownload(channelProxy, ticker.C, false)
}

func ProcessDownload(downloadResponseChannel <-chan IProxyResponse, tickerChannel <-chan time.Time, verbose bool) ErrorDownload{
	var downloadResponses []IProxyResponse
	startTime := time.Now()
    stalledCount := 0
	for {
		select {
		case downloadResponse := <-downloadResponseChannel:
			if downloadResponse != nil {
				downloadResponses = append(downloadResponses, downloadResponse)
			} else {
				return MapToErrorDownload(downloadResponses)
			}
		case <-tickerChannel:
			stalledCount = CheckDownloadStatus(os.Stdout, startTime, downloadResponses, verbose, stalledCount)
		}
	}
}
func CheckDownloadStatus(writer io.Writer, startTime time.Time, downloadResponses []IProxyResponse, verbose bool, stalledCount int) int {
	elapsedTime := time.Since(startTime)
	if elapsedTime.Seconds() > 10 {
		updateUI(writer, downloadResponses, verbose)
		stalledDownloads := FindStalledDownloads(downloadResponses)

		if len(stalledDownloads) > 0 {
			stalledCount++
		} else {
			stalledCount = 0
		}

		if stalledCount == 10 {
			for _, stalledDownload := range stalledDownloads {
				stalledDownload.Cancel()
				stalledDownload.SetDidTimeout()
			}
		}
	} else {
		writer.Write([]byte("preparing to download"))
	}
	return stalledCount
}

func FindStalledDownloads(responses []IProxyResponse) []IProxyResponse {
	stillDownloading := func(response IProxyResponse) bool {
		return response.BytesPerSecond() > 0
	}

	stalledDownloads := []IProxyResponse{}

	for _,response := range responses {
		if response.IsComplete() {
			continue
		} else if stillDownloading(response) {
			return []IProxyResponse{}
		} else {
			stalledDownloads = append(stalledDownloads, response)
		}
	}
	return stalledDownloads

}

func updateUI(writer io.Writer, responses []IProxyResponse, verbose bool) {
	print := func(resp IProxyResponse)  {
		message := fmt.Sprintf("Downloading %s %d / %d bytes (%d%%) - %.02fKBp/s ETA: %ds \033[K\n",
			resp.Filename(),
			resp.BytesComplete(),
			resp.Size(),
			int(100*resp.Progress()),
			resp.BytesPerSecond()/1024,
			int64(resp.ETA().Sub(time.Now()).Seconds()))
		writer.Write([]byte(message))
	}
	if verbose {
		for _, resp := range responses {
			print(resp)
		}
	} else {
		c,_ := curse.New()
		for i, resp := range responses {
			lineNumber := len(responses) - i
			c.MoveUp(lineNumber)
			c.EraseCurrentLine()
			print(resp)
			c.MoveDown(lineNumber)
		}
	}
}
