package download_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf/go-pivnet/download"
	"github.com/pivotal-cf/go-pivnet/download/fakes"
	"time"
	"bytes"
	"os"
	"fmt"
)

var _ = Describe("ProcessDownload", func() {
	var (
		result download.ErrorDownload
		tickerChannel chan time.Time
		downloadResponseChannel chan download.IProxyResponse
		response download.IProxyResponse
	)

	JustBeforeEach(func() {
		tickerChannel = make(chan time.Time)
		downloadResponseChannel = make(chan download.IProxyResponse)

		waitForResult := make(chan bool)
		go func() {
			result = download.ProcessDownload(downloadResponseChannel, tickerChannel, true)
			close(waitForResult)
		}()

		downloadResponseChannel <- response
		close(downloadResponseChannel)

		<-waitForResult
	})

	Context("when everything is downloaded", func() {
		BeforeEach(func() {
			response = &fakes.ProxyResponse{
				IsCompleteStub: func() bool {
					return true
				},
			}
		})

		It("there are no errors", func() {
			Expect(result.Error).NotTo(HaveOccurred())
		})

		It("cannot retry", func() {
			Expect(result.ShouldRetry).To(BeFalse())
		})
	})

	Context( "When there is an error in the download", func() {
		BeforeEach(func() {
			response = &fakes.ProxyResponse{
				ErrStub: func() error {
					return fmt.Errorf("Error")
				},
			}
		})

		It("there are errors", func() {
			Expect(result.Error).To(HaveOccurred())
			Expect(result.Error).To(MatchError("Error"))
		})

		It("cannot retry", func() {
			Expect(result.ShouldRetry).To(BeFalse())
		})
	})
})
var _ = Describe("FindStalledDownloads", func() {
	Context("When nothing is stalled and downloads are finished", func() {
		It("returns empty", func() {
			response1 := fakes.ProxyResponse{
				IsCompleteStub: func() bool {
					return true
				},
			}
			response2 := fakes.ProxyResponse{
				IsCompleteStub: func() bool {
					return true
				},
			}
			stalledDownloads := download.FindStalledDownloads([]download.IProxyResponse{&response1, &response2})
			Expect(stalledDownloads).To(BeEmpty())
		})
	})

	Context("when downloads are still active", func() {
		It("returns empty", func() {
			response1 := fakes.ProxyResponse{
				IsCompleteStub: func() bool {
					return false
				},
				BytesPerSecondStub: func() float64 {
					return float64(10000)
				},
			}
			response2 := fakes.ProxyResponse{
				IsCompleteStub: func() bool {
					return true
				},
			}
			stalledDownloads := download.FindStalledDownloads([]download.IProxyResponse{&response1, &response2})
			Expect(stalledDownloads).To(BeEmpty())
		})
	})

	Context("when there is only stalled downloads", func() {
		It("returns the stalled downloads", func() {
			response1 := fakes.ProxyResponse{
				IsCompleteStub: func() bool {
					return false
				},
				BytesPerSecondStub: func() float64 {
				 	return float64(0)
				},
				FilenameStub: func() string {
					return "stalled"
				},
			}
			response2 := fakes.ProxyResponse{
				IsCompleteStub: func() bool {
					return true
				},
			}

			stalledDownloads := download.FindStalledDownloads([]download.IProxyResponse{&response1, &response2})
			Expect(len(stalledDownloads)).To(Equal(1))
			Expect(stalledDownloads[0].Filename()).To(Equal("stalled"))
		})
	})
})
var _ = Describe("CheckDownloadStatus", func() {
	Context("Preparing to download", func() {
		It("Prints the prepare to download message", func() {
			buf := new(bytes.Buffer)
			download.CheckDownloadStatus(buf, time.Now(), []download.IProxyResponse{}, true, 0)
			Expect(buf.String()).To(Equal("preparing to download"))
		})
	})

	Context("When there is a stalled download", func() {
		It("Increments the download counter", func() {
			counter := 0
			responses := []download.IProxyResponse {&fakes.ProxyResponse{
				IsCompleteStub: func() bool {
					return false
				},
				BytesPerSecondStub: func() float64 {
					return 0
				},
			}}

			counter = download.CheckDownloadStatus(os.Stdout, time.Now().AddDate(0,0,-1), responses, true, counter)
			Expect(counter).To(Equal(1))
		})

		Context("Stalled download reached the retry limit", func() {
			It("Cancels the download ", func() {
				counter := 9
				fakeProxyReponse := &fakes.ProxyResponse{
					IsCompleteStub: func() bool {
						return false
					},
					BytesPerSecondStub: func() float64 {
						return 0
					},
				}
				responses := []download.IProxyResponse {fakeProxyReponse}

				download.CheckDownloadStatus(os.Stdout, time.Now().AddDate(0,0,-1), responses, true, counter)

				Expect(fakeProxyReponse.CancelCallCount()).To(Equal(1))
				Expect(fakeProxyReponse.SetDidTimeoutCallCount()).To(Equal(1))
			})
		})
	})

	Context("When there are no stalled downloads", func() {
		It("Resets the counter", func() {
			counter := 7
			responses := []download.IProxyResponse {&fakes.ProxyResponse{
				IsCompleteStub: func() bool {
					return false
				},
				BytesPerSecondStub: func() float64 {
					return 100000
				},
			}}

			counter = download.CheckDownloadStatus(os.Stdout, time.Now().AddDate(0,0,-1), responses, true, counter)
			Expect(counter).To(Equal(0))
		})
	})
})
var _ = Describe("MapToErrorDownload", func() {
	Context("When there are no errors", func() {
		It("Should have a nil error", func() {
			downloadResponses := []download.IProxyResponse{
				&fakes.ProxyResponse{},
			}

			err := download.MapToErrorDownload(downloadResponses)
			Expect(err.Error).To(BeNil())
		})
	})

	Context("When there are errors", func() {

		It("Should set the error", func() {
			downloadResponses := []download.IProxyResponse{
				&fakes.ProxyResponse{
					ErrStub: func() error {
						return fmt.Errorf("Oh no")
					},
				},
			}

			err := download.MapToErrorDownload(downloadResponses)
			Expect(err.Error).NotTo(BeNil())
		})

		Context("when response has a timeout set", func() {
			It("Should retry be true", func() {
				downloadResponses := []download.IProxyResponse{
					&fakes.ProxyResponse{
						ErrStub: func() error {
							return fmt.Errorf("Oh no")
						},
						DidTimeoutStub: func() bool {
							return true
						},
					},
				}

				err := download.MapToErrorDownload(downloadResponses)
				Expect(err.ShouldRetry).To(Equal(true))
			})
		})
	})
})
