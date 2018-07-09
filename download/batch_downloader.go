package download

import (
	"fmt"
	"time"
	"strings"
	"github.com/cavaliercoder/grab"
	"github.com/sethgrid/curse"
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
	const timeoutDuration = 5 * time.Second
	var realGrabRequests []*grab.Request
	for _, request := range requests {
		realGrabRequests = append(realGrabRequests, request.Wrapped())
	}

	responseChannel := c.DownloadClient.DoBatch(-1, realGrabRequests...)
	proxyResponseChannel := NewChannelProxy(responseChannel)

	return WaitForComplete(proxyResponseChannel.Channel, timeoutDuration)
}

func MapToErrorDownload(downloadResponses []IProxyResponse) ErrorDownload {
	var failedDownloadRequests []IProxyRequest
	var errstrings []string
	shouldRetry := true

	for _, downloadResponse := range downloadResponses {
		if downloadResponse != nil {
			var errorText string
			if downloadResponse.Err() == nil {
				errorText = "no errors!"
			} else {
				errorText = downloadResponse.Err().Error()
			}
			fmt.Println(fmt.Sprintf("during for loop: DidTimeout: %t Error: %s", downloadResponse.DidTimeout(), errorText))
		}

		if downloadResponse != nil && downloadResponse.Err() != nil {
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

	fmt.Println(fmt.Sprintf("failedDownloadRequests count: %d", len(failedDownloadRequests)))
	fmt.Println(fmt.Sprintf("shouldRetry: %s", shouldRetry))

	return ErrorDownload{
		Requests:    failedDownloadRequests,
		Error:       error,
		ShouldRetry: shouldRetry,
	}
}

func WaitForComplete(channelProxy <-chan IProxyResponse, timeoutDuration time.Duration) ErrorDownload{
	var downloadResponses []IProxyResponse
	var downloadResponseCounters []int
	ticker := time.NewTicker(500 * time.Millisecond)

	defer ticker.Stop()

	for {
		select {
		case downloadResponse := <-channelProxy:
			if downloadResponse != nil {
				// a new response has been received and has started downloading
				downloadResponses = append(downloadResponses, downloadResponse)
				downloadResponseCounters = append(downloadResponseCounters, 0)
			} else {
				return MapToErrorDownload(downloadResponses)
			}

		case <-ticker.C:
			updateUI(downloadResponses)
			for i, response := range downloadResponses {
				if !response.IsComplete() && response.BytesPerSecond() <= 0 && downloadResponseCounters[i] < 10 {
					downloadResponseCounters[i] ++
					if downloadResponseCounters[i] == 10 {
						response.Done()  //stop download
						response.SetDidTimeout()
					}
				}
			}
		}
	}
}

func updateUI(responses []IProxyResponse) {
	c,_ := curse.New()
	for i, resp := range responses {
		lineNumber := len(responses) - i
		c.MoveUp(lineNumber)
		c.EraseCurrentLine()
		fmt.Printf("Downloading %s %d / %d bytes (%d%%) - %.02fKBp/s ETA: %ds \033[K\n",
			resp.Filename(),
			resp.BytesComplete(),
			resp.Size(),
			int(100*resp.Progress()),
			resp.BytesPerSecond()/1024,
			int64(resp.ETA().Sub(time.Now()).Seconds()))
		c.MoveDown(lineNumber)
	}
}
