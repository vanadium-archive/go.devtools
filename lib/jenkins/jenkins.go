package jenkins

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
)

func New(host string) *Jenkins {
	return &Jenkins{
		host: host,
	}
}

type Jenkins struct {
	host string
}

// Invoke invokes the Jenkins API using the given suffix, values and
// HTTP method.
func (j *Jenkins) Invoke(method, suffix string, values url.Values) (*http.Response, error) {
	apiURL, err := url.Parse(j.host)
	if err != nil {
		return nil, fmt.Errorf("Parse(%q) failed: %v", j.host, err)
	}
	apiURL.Path = fmt.Sprintf("%s/%s", suffix)
	apiURL.RawQuery = values.Encode()
	var body io.Reader
	url, body := apiURL.String(), nil
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	return res, nil
}
