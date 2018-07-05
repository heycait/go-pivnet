package download

import (
	"fmt"
	"time"
	"os"
	"strings"
	"github.com/cavaliercoder/grab"
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
	stalledDownloadChannel := make(chan IProxyResponse)
	var downloadResponses []IProxyResponse
	ticker := time.NewTicker(500 * time.Millisecond)
	stalledDownloadTimer := time.AfterFunc(timeoutDuration, func() {
		for _, response := range downloadResponses {
			if response != nil && !response.IsComplete() && response.BytesPerSecond() == 0 {
				stalledDownloadChannel <- response
			}
		}
	})

	defer ticker.Stop()
	defer stalledDownloadTimer.Stop()

	for {
		select {
		case downloadResponse := <-channelProxy:
			if downloadResponse != nil {
				// a new response has been received and has started downloading
				downloadResponses = append(downloadResponses, downloadResponse)
			} else {
				fmt.Println("mapping to error download")
				return MapToErrorDownload(downloadResponses)
			}

		case <-ticker.C:
			updateUI(downloadResponses)
			for _, response := range downloadResponses {
				if response != nil && response.BytesPerSecond() > 0 {
					stalledDownloadTimer.Reset(timeoutDuration)
				} else {
					//a download is stalling
				}
			}

		case stalledResponse := <-stalledDownloadChannel:
			stalledResponse.Done()  //stop download
			stalledResponse.SetDidTimeout()
			fmt.Println(fmt.Sprintf("Download has stalled: %s ; %s", stalledResponse.Filename(), stalledResponse.DidTimeout()))
		}
	}
}

func updateUI(responses []IProxyResponse) {
	fmt.Println("***************\n\n\n\n\n********************")
	// print newly completed downloads
	for _, resp := range responses {
		if resp != nil && resp.IsComplete() {
			if resp.Err() != nil {
				fmt.Fprintf(os.Stderr, "Error downloading %s: %v\n",
					resp.Request().URL(),
					resp.Err())
			} else {
				fmt.Printf("Finished %s %d / %d bytes (%d%%)\n",
					resp.Filename(),
					resp.BytesComplete(),
					resp.Size(),
					int(100*resp.Progress()))
			}
		}
	}

	// print progress for incomplete downloads
	for _, resp := range responses {
		if resp != nil {
			fmt.Printf("Downloading %s %d / %d bytes (%d%%) - %.02fKBp/s ETA: %ds \033[K\n",
				resp.Filename(),
				resp.BytesComplete(),
				resp.Size(),
				int(100*resp.Progress()),
				resp.BytesPerSecond()/1024,
				int64(resp.ETA().Sub(time.Now()).Seconds()))
		}
	}
}
