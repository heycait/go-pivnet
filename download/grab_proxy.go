package download

import (
	"time"
	"github.com/cavaliercoder/grab"
	"net/url"
)

type ProxyRequest struct {
	Wrapped *grab.Request
}

func NewProxyRequest(request *grab.Request) *ProxyRequest {
	return &ProxyRequest{
		Wrapped: request,
	}
}

func (p ProxyRequest) URL() *url.URL {
	return p.Wrapped.URL()
}

type ProxyResponse struct {
	Wrapped *grab.Response
	Filename string
	Size int64
	Request *ProxyRequest
}

func NewProxyResponse(response *grab.Response) *ProxyResponse {
	return &ProxyResponse {
		Wrapped: response,
		Filename: response.Filename,
		Size: response.Size,
		Request: NewProxyRequest(response.Request),
	}
}

func (p ProxyResponse) Err() error {
	return p.Wrapped.Err()
}

func (p ProxyResponse) IsComplete() bool {
	return p.Wrapped.IsComplete()
}

func (p ProxyResponse) BytesPerSecond() float64 {
	return p.Wrapped.BytesPerSecond()
}

func (p ProxyResponse) BytesComplete() int64 {
	return p.Wrapped.BytesComplete()
}

func (p ProxyResponse) Progress() float64 {
	return p.Wrapped.Progress()
}

func (p ProxyResponse) Done() {
	close(p.Wrapped.Done)
}

func (p ProxyResponse) ETA() time.Time {
	return p.Wrapped.ETA()
}

type ChannelProxy struct {
	Wrapped <-chan *grab.Response
	Channel <-chan *ProxyResponse
}

func NewChannelProxy(responseChannel <-chan *grab.Response) *ChannelProxy {
	channel := make(chan *ProxyResponse)
	go func() {
		for {
			select {
			case response := <-responseChannel:
				if response == nil {
					close(channel)
					return
				} else {
					channel <- NewProxyResponse(response)
				}

			}
		}
	}()
	return &ChannelProxy {
		Wrapped: responseChannel,
		Channel: channel,
	}
}