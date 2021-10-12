package soju

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/machinebox/graphql"
)

type srhtAuthIRCConn struct {
	ircConn
	auth *SrhtAuth
}

type SrhtAuth struct {
	Username string
}

func checkSrhtCookie(cookie *http.Cookie) (*SrhtAuth, error) {
	h := make(http.Header)
	h.Set("Cookie", cookie.String())
	return checkSrhtAuth(h)
}

func checkSrhtToken(token string) (*SrhtAuth, error) {
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+token)
	return checkSrhtAuth(h)
}

func checkSrhtAuth(h http.Header) (*SrhtAuth, error) {
	endpoint := "https://meta.sr.ht"
	if v, ok := os.LookupEnv("SRHT_ENDPOINT"); ok {
		endpoint = v
	}

	client := graphql.NewClient(endpoint + "/query")

	req := graphql.NewRequest(`
		query {
			me {
				username
			}
		}
	`)

	for k, vs := range h {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}

	var respData struct {
		Me struct {
			Username string
		}
	}
	if err := client.Run(context.Background(), req, &respData); err != nil {
		return nil, err
	}

	return &SrhtAuth{Username: respData.Me.Username}, nil
}

func getOrCreateSrhtUser(srv *Server, auth *SrhtAuth) (*user, error) {
	u := srv.getUser(auth.Username)
	if u != nil {
		return u, nil
	}

	if os.Getenv("SRHT_NO_ALLOWLIST") != "1" {
		return nil, fmt.Errorf("user missing from allow-list")
	}

	record := User{Username: auth.Username}
	return srv.createUser(&record)
}
