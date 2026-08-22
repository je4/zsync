package client

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/je4/zsync/v2/info"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"gopkg.in/resty.v1"
)

type Client struct {
	baseUrl    *url.URL
	apiKey     string
	ServerId   string
	client     *resty.Client
	Logger     zLogger.ZLogger
	CurrentKey *model.ApiKey
}

func NewClient(endpoint string, apiKey string, logger zLogger.ZLogger) (*Client, error) {
	burl, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse url %s", endpoint)
	}

	c := &Client{
		baseUrl: burl,
		apiKey:  apiKey,
		Logger:  logger,
	}

	if err := c.Init(); err != nil {
		return nil, errors.Wrap(err, "cannot init client")
	}

	return c, nil
}

func (c *Client) Init() (err error) {
	c.client = resty.New()
	c.client.SetHostURL(c.baseUrl.String())
	c.client.SetHeader("User-Agent", info.GetUserAgent())
	if c.apiKey != "" {
		c.client.SetAuthToken(c.apiKey)
		c.client.SetHeader("Zotero-API-Key", c.apiKey)
	}
	c.client.SetHeader("Zotero-API-Version", "3")
	c.client.SetContentLength(true)
	c.client.SetRedirectPolicy(resty.FlexibleRedirectPolicy(3))
	if c.apiKey != "" {
		c.CurrentKey, err = c.getCurrentKey()
		if err != nil {
			if c.Logger != nil {
				c.Logger.Warn().Msgf("Failed to retrieve current key (%v), falling back to server id detection", err)
			}
			_, _ = c.DetectServerId()
			if c.CurrentKey == nil {
				c.CurrentKey = &model.ApiKey{
					UserId:   0,
					Username: "",
				}
			}
			err = nil
		}
	} else {
		_, _ = c.DetectServerId()
		if c.CurrentKey == nil {
			c.CurrentKey = &model.ApiKey{
				UserId:   0,
				Username: "",
			}
		}
	}

	return
}

func (c *Client) GetBaseUrl() *url.URL {
	return c.baseUrl
}

func (c *Client) GetApiKey() string {
	return c.apiKey
}

func (c *Client) SetApiKey(apiKey string) {
	c.apiKey = apiKey
	if c.client != nil {
		if apiKey != "" {
			c.client.SetAuthToken(apiKey)
			c.client.SetHeader("Zotero-API-Key", apiKey)
		} else {
			c.client.Header.Del("Zotero-API-Key")
		}
	}
}

func (c *Client) SetServerId(serverId string) {
	c.ServerId = serverId
	if c.client != nil && serverId != "" {
		c.client.SetHeader("Zotero-Server-ID", serverId)
	}
}

func (c *Client) GetServerId() string {
	return c.ServerId
}

func (c *Client) GetCurrentKey() *model.ApiKey {
	return c.CurrentKey
}

func (c *Client) SetCurrentKey(key *model.ApiKey) {
	c.CurrentKey = key
}

func (c *Client) GetResty() *resty.Client {
	return c.client
}

func (c *Client) DetectServerId() (string, error) {
	resp, err := c.client.R().Get("/")
	if err != nil || resp.Header().Get("Zotero-Server-ID") == "" {
		resp, err = c.client.R().Get("")
	}
	if err != nil {
		return "", errors.Wrap(err, "cannot detect server id")
	}
	serverId := resp.Header().Get("Zotero-Server-ID")
	if serverId == "" {
		serverId = resp.Header().Get("zotero-server-id")
	}
	if serverId != "" {
		c.SetServerId(serverId)
	}
	return serverId, nil
}

// AuthorizeLocal requests authorization from a locally running Zotero desktop instance
// via POST /local/authorize or POST /keys with a default 2-minute timeout.
//
// NOTE: Zotero Cloud API keys (created on zotero.org/settings/keys) are validated only
// by the central Zotero Cloud servers and are NOT recognized by the local embedded HTTP server.
// To perform write operations against the local Zotero instance (localhost:23119), a local
// authorization handshake must be initiated, which triggers an interactive permission popup
// in the Zotero Desktop application and returns a local API token.
func (c *Client) AuthorizeLocal(appName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return c.AuthorizeLocalContext(ctx, appName)
}

// AuthorizeLocalContext requests authorization from a locally running Zotero desktop instance
// with a custom context for cancellation/timeout control.
func (c *Client) AuthorizeLocalContext(ctx context.Context, appName string) (string, error) {
	if appName == "" {
		appName = "ZSync"
	}
	if c.ServerId == "" {
		_, _ = c.DetectServerId()
	}

	authEndpoints := []string{"/local/authorize", "/keys"}
	var lastErr error

	for _, endpoint := range authEndpoints {
		if c.Logger != nil {
			c.Logger.Info().Msgf("local auth call: POST %s (app: %s, serverId: %s)", endpoint, appName, c.ServerId)
		}

		req := c.client.R().
			SetContext(ctx).
			SetHeader("Accept", "application/json").
			SetHeader("Content-Type", "application/json").
			SetBody(map[string]any{
				"appName":     appName,
				"application": appName,
				"library":     "1",
				"files":       "1",
				"notes":       "1",
				"write":       "1",
			})
		if c.ServerId != "" {
			req.SetHeader("Zotero-Server-ID", c.ServerId)
		}

		resp, err := req.Post(endpoint)
		if err != nil {
			if c.Logger != nil {
				c.Logger.Warn().Msgf("local auth request failed at %s: %v", endpoint, err)
			}
			lastErr = errors.Wrapf(err, "failed to post to %s", endpoint)
			continue
		}
		if c.Logger != nil {
			c.Logger.Debug().Msgf("local auth response status for %s: %d", endpoint, resp.StatusCode())
		}
		if resp.StatusCode() == 404 {
			lastErr = errors.Errorf("local authorization endpoint %s returned 404: %s", endpoint, resp.String())
			continue
		}
		if resp.StatusCode() != 200 {
			if c.Logger != nil {
				c.Logger.Warn().Msgf("local auth rejected with HTTP %d at %s: %s", resp.StatusCode(), endpoint, resp.String())
			}
			return "", errors.Errorf("local authorization failed at %s with status %d: %s", endpoint, resp.StatusCode(), resp.String())
		}

		var authResp model.LocalAuthResponse
		if err := json.Unmarshal(resp.Body(), &authResp); err != nil {
			return "", errors.Wrapf(err, "cannot unmarshal local auth response from %s: %s", endpoint, string(resp.Body()))
		}

		if authResp.Key != "" {
			c.SetApiKey(authResp.Key)
			if c.CurrentKey == nil {
				c.CurrentKey = &model.ApiKey{
					UserId:   0,
					Username: "",
				}
			}
		}

		return authResp.Key, nil
	}

	return "", lastErr
}

func (c *Client) getCurrentKey() (*model.ApiKey, error) {
	endpoint := "/keys/current"
	if c.Logger != nil {
		c.Logger.Info().Msgf("rest call: %s", endpoint)
	}

	resp, err := c.client.R().
		SetHeader("Accept", "application/json").
		Get(endpoint)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	if resp.StatusCode() != 200 {
		return nil, errors.Errorf("failed to get current key from %s: status %d (%s)", endpoint, resp.StatusCode(), resp.String())
	}
	rawBody := resp.Body()
	key := &model.ApiKey{}
	if err := json.Unmarshal(rawBody, key); err != nil {
		return nil, errors.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	return key, nil
}

/*
*
Clients accessing the Zotero API should be prepared to handle two forms of rate limiting: backoff requests and hard limiting.
If the API servers are overloaded, the API may include a Backoff: <seconds> HTTP header in responses, indicating that the client should perform the minimum number of requests necessary to maintain data consistency and then refrain from making further requests for the number of seconds indicated. Backoff can be included in any response, including successful ones.
If a client has made too many requests within a given time period, the API may return 429 Too Many Requests with a Retry-After: <seconds> header. Clients receiving a 429 should wait the number of seconds indicated in the header before retrying the request.
Retry-After can also be included with 503 Service Unavailable responses when the server is undergoing maintenance.
*/
func (c *Client) CheckRetry(header http.Header) bool {
	if header == nil {
		return false
	}
	retryAfterStr := strings.TrimSpace(header.Get("Retry-After"))
	if retryAfterStr == "" {
		return false
	}
	var retryAfter int64
	if val, err := strconv.ParseInt(retryAfterStr, 10, 64); err == nil {
		retryAfter = val
	} else if t, err := http.ParseTime(retryAfterStr); err == nil {
		diff := time.Until(t)
		if diff > 0 {
			retryAfter = int64(diff / time.Second)
			if diff%time.Second > 0 {
				retryAfter++
			}
		} else {
			retryAfter = 0
		}
	}

	if retryAfter > 0 {
		if c.Logger != nil {
			c.Logger.Info().Msgf("Sleeping %v seconds (RetryAfter)", retryAfter)
		}
		time.Sleep(time.Duration(retryAfter) * time.Second)
		return true
	}
	return false
}

func (c *Client) CheckBackoff(header http.Header) bool {
	if header == nil {
		return false
	}
	backoffStr := strings.TrimSpace(header.Get("Backoff"))
	if backoffStr == "" {
		return false
	}
	var backoff int64
	if val, err := strconv.ParseInt(backoffStr, 10, 64); err == nil {
		backoff = val
	}
	if backoff > 0 {
		if c.Logger != nil {
			c.Logger.Info().Msgf("Sleeping %v seconds (Backoff)", backoff)
		}
		time.Sleep(time.Duration(backoff) * time.Second)
		return true
	}
	return false
}
