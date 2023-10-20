package jwks

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/rakutentech/jwk-go/jwk"
	"github.com/valyala/fasttemplate"
	"golang.org/x/sync/singleflight"
)

const (
	_defaultTTL                = 1 * time.Hour
	_defaultMaxIdleConnPerHost = 255
	_defaultTimeout            = 1 * time.Second
	_defaultRetries            = 2
)

var (
	// ErrInvalidURL returned when input url has invalid format.
	ErrInvalidURL = errors.New("jwks: invalid url value or format")
	// ErrInvalidNumRetries returned when number of retries is zero.
	ErrInvalidNumRetries = errors.New("jwks: invalid number of retries")
)

// Manager fetches and returns JWK from public source.
type Manager struct {
	url      *fasttemplate.Template
	cache    Cache
	client   *http.Client
	useCache bool
	retries  uint
	group    singleflight.Group
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: _defaultMaxIdleConnPerHost,
		},
		Timeout: _defaultTimeout,
	}
}

// JWK represents an unparsed JSON Web Key (JWK) in its wire format.
type JWK = jwk.JWK

// NewManager returns a new instance of Manager.
func NewManager(rawURL string, opts ...Option) (*Manager, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("endpoint must have http:// or https:// scheme, got: %s", rawURL)
	}
	urlTemplate := fasttemplate.New(rawURL, "{{", "}}")

	mng := &Manager{
		url: urlTemplate,

		cache:    NewTTLCache(_defaultTTL),
		client:   defaultHTTPClient(),
		useCache: true,
		retries:  _defaultRetries,
	}

	for _, opt := range opts {
		opt(mng)
	}

	if mng.retries == 0 {
		return nil, ErrInvalidNumRetries
	}

	return mng, nil
}
