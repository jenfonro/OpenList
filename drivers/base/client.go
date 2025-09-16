package base

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/go-resty/resty/v2"
)

var (
	NoRedirectClient *resty.Client
	RestyClient      *resty.Client
	HttpClient       *http.Client
)
var UserAgent = "Mozilla/5.0 (Macintosh; Apple macOS 15_5) AppleWebKit/537.36 (KHTML, like Gecko) Safari/537.36 Chrome/138.0.0.0"
var DefaultTimeout = time.Second * 30

func InitClient() {
	NoRedirectClient = resty.New().SetRedirectPolicy(
		resty.RedirectPolicyFunc(func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}),
	).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: conf.Conf.TlsInsecureSkipVerify})
	NoRedirectClient.SetHeader("user-agent", UserAgent)

	RestyClient = NewRestyClient()
	// 设置全局客户端的 Transport，使其可以基于请求头/Context 按请求动态代�?
	if hc := RestyClient.GetClient(); hc != nil {
		transport := &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				// 优先从请求头读取代理地址，其次从 Context 读取
				if addr := req.Header.Get("X-Driver-Proxy-Addr"); addr != "" {
					return url.Parse(addr)
				}
				if v := req.Context().Value("X-Driver-Proxy-Addr"); v != nil {
					if s, ok := v.(string); ok && s != "" {
						return url.Parse(s)
					}
				}
				return nil, nil
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: conf.Conf.TlsInsecureSkipVerify},
		}
		RestyClient.SetTransport(transport)
	}
	HttpClient = &http.Client{
		Timeout: time.Hour * 48,
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				if addr := req.Header.Get("X-Driver-Proxy-Addr"); addr != "" {
					return url.Parse(addr)
				}
				if v := req.Context().Value("X-Driver-Proxy-Addr"); v != nil {
					if s, ok := v.(string); ok && s != "" {
						return url.Parse(s)
					}
				}
				return http.ProxyFromEnvironment(req) // Fallback to environment proxy
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: conf.Conf.TlsInsecureSkipVerify},
		},
	}
}

func NewRestyClient() *resty.Client {
	client := resty.New().
		SetHeader("user-agent", UserAgent).
		SetRetryCount(3).
		SetRetryResetReaders(true).
		SetTimeout(DefaultTimeout).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: conf.Conf.TlsInsecureSkipVerify})
	return client
}

// RWithProxy returns a new request from the shared RestyClient and
// attaches per-request proxy via header when provided.
func RWithProxy(driverProxyAddr string) *resty.Request {
	req := RestyClient.R()
	if driverProxyAddr != "" {
		req.SetHeader("X-Driver-Proxy-Addr", driverProxyAddr)
	}
	return req
}

// NoRedirectRWithProxy returns a new request from the shared NoRedirectClient
// and attaches per-request proxy via header when provided.
func NoRedirectRWithProxy(driverProxyAddr string) *resty.Request {
	req := NoRedirectClient.R()
	if driverProxyAddr != "" {
		req.SetHeader("X-Driver-Proxy-Addr", driverProxyAddr)
	}
	return req
}
