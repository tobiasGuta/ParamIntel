package httpraw

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func Parse(raw []byte, defaultScheme string) (model.RequestTemplate, error) {
	if defaultScheme == "" {
		defaultScheme = "https"
	}
	r := bufio.NewReader(bytes.NewReader(raw))
	requestLine, err := r.ReadString('\n')
	if err != nil {
		return model.RequestTemplate{}, fmt.Errorf("read request line: %w", err)
	}
	parts := strings.Fields(strings.TrimSpace(requestLine))
	if len(parts) < 2 {
		return model.RequestTemplate{}, fmt.Errorf("invalid HTTP request line")
	}
	method, target := parts[0], parts[1]
	headers := make(http.Header)
	for {
		line, err := r.ReadString('\n')
		if err != nil && len(line) == 0 {
			return model.RequestTemplate{}, fmt.Errorf("read headers: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			return model.RequestTemplate{}, fmt.Errorf("invalid header line %q", line)
		}
		headers.Add(strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]))
	}
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(r)

	var finalURL string
	if u, err := url.Parse(target); err == nil && u.IsAbs() {
		finalURL = u.String()
	} else {
		host := headers.Get("Host")
		if host == "" {
			return model.RequestTemplate{}, fmt.Errorf("missing Host header for relative request target")
		}
		finalURL = defaultScheme + "://" + host + target
	}
	return model.RequestTemplate{Method: method, URL: finalURL, Headers: headers, Body: body.Bytes()}, nil
}
