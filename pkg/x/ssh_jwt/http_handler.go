package ssh_jwt

import (
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/ssh"
	"net/http"
	"strings"
	"time"
)

type Handler struct {
	AuthorizedKeys []string
	CookieName     string
	HeaderName     string
	SigningMethod  jwt.SigningMethod
	Handler        http.Handler
	StrictPaths    []string
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cn := h.CookieName
	if cn == "" {
		cn = "AuthToken"
	}
	hn := h.HeaderName
	if hn == "" {
		hn = "AuthToken"
	}
	authKeys := make([]string, 0, len(h.AuthorizedKeys))
	for _, ak := range h.AuthorizedKeys {
		sak, err := stripPublicKeyComment(ak)
		if err == nil {
			authKeys = append(authKeys, sak)
		}
	}
	q := r.URL.Query()
	tokReq := q.Get(cn)
	if tokReq != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     cn,
			Value:    tokReq,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteStrictMode,
		})
		u := r.URL
		q.Del(hn)
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
		return
	}
	authorized := false
	c, err := r.Cookie(cn)
	if err == nil && c != nil && h.SigningMethod != nil {
		smSSH, ok := h.SigningMethod.(*signingMethodSSHAgent)
		if ok {
			publicKey, expirationTime, err := smSSH.AuthorizeToken(c.Value)
			if err == nil && expirationTime.After(time.Now()) {
				spk, err := stripPublicKeyComment(publicKey)
				if err == nil {
					for _, k := range authKeys {
						if k == spk {
							authorized = true
							break
						}
					}
				}
			}
		}
	}
	if !authorized {
		for _, sp := range h.StrictPaths {
			if strings.HasPrefix(r.URL.Path, sp) {
				w.WriteHeader(401)
				_, _ = w.Write([]byte("Unauthorized"))
				return
			}
		}
		r.Header.Set("unauthorized", "true")
	}
	h.Handler.ServeHTTP(w, r)
}

func stripPublicKeyComment(key string) (string, error) {
	parsedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
	if err != nil {
		return "", err
	}
	return string(ssh.MarshalAuthorizedKey(parsedKey)), nil
}
