package download

import (
	"fmt"
	"github.com/pivotal-cf/go-pivnet/logger"
	"io"
	"net/http"
	"os"
	"github.com/shirou/gopsutil/disk"
	"github.com/cavaliercoder/grab"
	"time"
	"strings"
)

//go:generate counterfeiter -o ./fakes/ranger.go --fake-name Ranger . ranger
type ranger interface {
	BuildRange(contentLength int64) ([]Range, error)
}

//go:generate counterfeiter -o ./fakes/http_client.go --fake-name HTTPClient . httpClient
type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

//go:generate counterfeiter -o ./fakes/download_client.go --fake-name DownloadClient . downloadClient
type downloadClient interface {
	DoBatch (int,...*grab.Request) <-chan *grab.Response
}

type downloadLinkFetcher interface {
	NewDownloadLink() (string, error)
}

type Client struct {
	HTTPClient httpClient
	DownloadClient downloadClient
	Ranger     ranger
	Logger     logger.Logger
}

func GetFileChunkNames(location string, ranges []Range) []string {
	var list []string
	for _, r := range ranges {
		fileName := fmt.Sprintf("%s_%d", location, r.Lower)
		list = append(list, fileName)
	}
	return list
}

func CleanupFileChunks(fileChunkNames []string) error {
	for _, fileChunkName := range fileChunkNames {
		err := os.Remove(fileChunkName)
		if err != nil {
			return err
		}
	}
	return nil
}

func CombineFileChunks(fileWriter *os.File, fileChunkNames []string) error {
	fileInfo, err := fileWriter.Stat()
	if err != nil {
		return fmt.Errorf("failed to read information from output file: %s", err)
	}
	file, err := os.OpenFile(fileWriter.Name(), os.O_RDWR, fileInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to open file for writing: %s", err)
	}

	for _, fileChunkName := range fileChunkNames {
		fileChunk, err := os.Open(fileChunkName)
		if err != nil {
			return err
		}

		_, err = io.Copy(file, fileChunk)
		errClose := fileChunk.Close()
		if err != nil {
			return err
		}
		if errClose != nil {
			return errClose
		}
	}
	err = file.Close()
	if err != nil {
		return err
	}

	return nil
}

func (c Client) Get(
	location *os.File,
	downloadLinkFetcher downloadLinkFetcher,
	progressWriter io.Writer,
) error {
	contentURL, err := downloadLinkFetcher.NewDownloadLink()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("HEAD", contentURL, nil)
	if err != nil {
		return fmt.Errorf("failed to construct HEAD request: %s", err)
	}

	req.Header.Add("Referer","https://go-pivnet.network.pivotal.io")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make HEAD request: %s", err)
	}

	contentURL = resp.Request.URL.String()

	ranges, err := c.Ranger.BuildRange(resp.ContentLength)
	if err != nil {
		return fmt.Errorf("failed to construct range: %s", err)
	}

	diskStats, err := disk.Usage(location.Name())
	if err != nil {
		return fmt.Errorf("failed to get disk free space: %s", err)
	}

	if diskStats.Free < uint64(resp.ContentLength) {
		return fmt.Errorf("file is too big to fit on this drive")
	}

	fileNameChunks := GetFileChunkNames(location.Name(), ranges)
	requests, err := GetRequests(contentURL, fileNameChunks, ranges)

	if err != nil {
		return fmt.Errorf("could not create request: %s", err)
	}

	responseChannel := c.DownloadClient.DoBatch(-1, requests...)
	err = waitForComplete(responseChannel, len(requests))

	if err != nil {
		return fmt.Errorf("download failed: %s", err)
	}


	c.Logger.Debug(fmt.Sprintf("assembling chunks"))
	if err := CombineFileChunks(location, fileNameChunks); err != nil {
		return fmt.Errorf("failed to combine file chunks: %s", err)
	}

	c.Logger.Debug(fmt.Sprintf("cleaning up chunks"))
	if err := CleanupFileChunks(fileNameChunks); err != nil {
		return fmt.Errorf("failed to cleanup file chunks: %s", err)
	}

	return nil
}

func GetRequests(contentURL string, fileNameChunks []string, ranges []Range) ([]*grab.Request, error) {
	var requests []*grab.Request

	for i, r := range ranges {
		request, err := grab.NewRequest(fileNameChunks[i], contentURL)
		if err != nil {
			return nil, err
		}
		request.HTTPRequest.Header = r.HTTPHeader
		request.HTTPRequest.Header.Add("Referer", "https://go-pivnet.network.pivotal.io")
		requests = append(requests, request)
	}
	return requests, nil
}

func waitForComplete(respch <-chan *grab.Response, responseSize int) error{
	const timeoutDuration = 5 * time.Second
	stalledDownloadChannel := make(chan(*grab.Response))
	responses := make([]*grab.Response, 0, responseSize)
	t := time.NewTicker(500 * time.Millisecond)
	stalledDownloadTimer := time.AfterFunc(timeoutDuration, func() {
		for _, response := range responses {
			if response != nil && !response.IsComplete() && response.BytesPerSecond() == 0 {
				stalledDownloadChannel <- response
			}
		}
	})

	defer t.Stop()
	defer stalledDownloadTimer.Stop()

	for {
		select {
		case resp := <-respch:
			if resp != nil {
				// a new response has been received and has started downloading
				responses = append(responses, resp)
			} else {
				// channel is closed - all downloads are complete
				var errstrings []string
				for _, response := range responses {
					if response != nil && response.Err() != nil {
						errstrings = append(errstrings, fmt.Sprintf("Error for %v: %v", response.Filename, response.Err().Error()))
					}
				}
				if len(errstrings) > 0 {
					return fmt.Errorf(strings.Join(errstrings, "\n"))
				} else {
					return nil
				}
			}

		case <-t.C:
			// update UI every 200ms
			updateUI(responses)
			for _, response := range responses {
				if response != nil && response.BytesPerSecond() > 0 {
					stalledDownloadTimer.Reset(timeoutDuration)
				} else {
					//everything is ok
				}
			}

		case stalledRequest := <-stalledDownloadChannel:
			close(stalledRequest.Done)
			return fmt.Errorf("a download timed out for chunk: %s", stalledRequest.Filename)
		}
	}
}

func updateUI(responses []*grab.Response) {
	fmt.Println("***************\n\n\n\n\n********************")
	// print newly completed downloads
	for i, resp := range responses {
		if resp != nil && resp.IsComplete() {
			if resp.Err() != nil {
				fmt.Fprintf(os.Stderr, "Error downloading %s: %v\n",
					resp.Request.URL(),
					resp.Err())
			} else {
				fmt.Printf("Finished %s %d / %d bytes (%d%%)\n",
					resp.Filename,
					resp.BytesComplete(),
					resp.Size,
					int(100*resp.Progress()))
			}
			responses[i] = nil
		}
	}

	// print progress for incomplete downloads
	for _, resp := range responses {
		if resp != nil {
			fmt.Printf("Downloading %s %d / %d bytes (%d%%) - %.02fKBp/s ETA: %ds \033[K\n",
				resp.Filename,
				resp.BytesComplete(),
				resp.Size,
				int(100*resp.Progress()),
				resp.BytesPerSecond()/1024,
				int64(resp.ETA().Sub(time.Now()).Seconds()))
		}
	}
}
