package NetClient

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// ServerOrigin binds a saved bearer token to a scheme, host and effective port.
func ServerOrigin(address string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("服务地址必须是有效的 HTTP(S) 地址，不能包含凭据")
	}
	p := u.Port()
	if p == "" {
		if u.Scheme == "https" {
			p = "443"
		} else {
			p = "80"
		}
	}
	return u.Scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), p), nil
}
func credentialRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("重定向次数过多")
	}
	source, err := ServerOrigin(via[0].URL.String())
	target, te := ServerOrigin(req.URL.String())
	if err != nil || te != nil || source != target {
		return http.ErrUseLastResponse
	}
	return nil
}
