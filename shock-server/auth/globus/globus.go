// Package globus implements Globus Online Nexus authentication
package globus

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"github.com/MG-RAST/Shock/shock-server/auth/basic"
	"github.com/MG-RAST/Shock/shock-server/conf"
	e "github.com/MG-RAST/Shock/shock-server/errors"
	"github.com/MG-RAST/Shock/shock-server/logger"
	"github.com/MG-RAST/Shock/shock-server/user"
	"io/ioutil"
	"net/http"
	"strings"
)

// Token response struct
type token struct {
	AccessToken     string      `json:"access_token"`
	AccessTokenHash string      `json:"access_token_hash"`
	ClientId        string      `json:"client_id"`
	ExpiresIn       int         `json:"expires_in"`
	Expiry          int         `json:"expiry"`
	IssuedOn        int         `json:"issued_on"`
	Lifetime        int         `json:"lifetime"`
	Scopes          interface{} `json:"scopes"`
	TokenId         string      `json:"token_id"`
	TokeType        string      `json:"token_type"`
	UserName        string      `json:"user_name"`
}

func authHeaderType(header string) string {
	tmp := strings.Split(header, " ")
	if len(tmp) > 1 {
		return strings.ToLower(tmp[0])
	}
	return ""
}

// Auth takes the request authorization header and returns
// user
func Auth(header string) (usr *user.User, err error) {
	switch authHeaderType(header) {
	case "globus-goauthtoken", "oauth":
		return fetchProfile(strings.Split(header, " ")[1])
	case "basic":
		if username, password, err := basic.DecodeHeader(header); err == nil {
			if t, err := fetchToken(username, password); err == nil {
				return fetchProfile(t.AccessToken)
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	default:
		return nil, errors.New("Invalid authentication header.")
	}
}

// fetchToken takes username and password and then retrieves user token
func fetchToken(u string, p string) (t *token, err error) {
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	req, err := http.NewRequest("GET", conf.AUTH_GLOBUS_TOKEN_URL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(u, p)
	if resp, err := client.Do(req); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusCreated {
			if body, err := ioutil.ReadAll(resp.Body); err == nil {
				if err = json.Unmarshal(body, &t); err != nil {
					return nil, err
				}
			}
		} else {
			return nil, errors.New("Authentication failed: Unexpected response status: " + resp.Status)
		}
	} else {
		return nil, err
	}
	return
}

// fetchProfile validiates token by using it to fetch user profile
func fetchProfile(t string) (u *user.User, err error) {
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	//logger.Error("got here")
	req, err := http.NewRequest("GET", conf.AUTH_GLOBUS_PROFILE_URL+"/"+clientId(t), nil)
	//logger.Error("URL: " + conf.AUTH_GLOBUS_PROFILE_URL+"/"+clientId(t))
	if err != nil {
		return nil, err
	}
	//req.Header.Add("Authorization", t)
	req.Header.Add("Authorization", "Globus-Goauthtoken "+t)
	if resp, err := client.Do(req); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if body, err := ioutil.ReadAll(resp.Body); err == nil {
				u = &user.User{}
				if err = json.Unmarshal(body, &u); err != nil {
					return nil, err
				} else {
					if u.Username == "" {
						return nil, errors.New(e.InvalidAuth)
					}
					if err = u.SetMongoInfo(); err != nil {
						return nil, err
					}
				}
			}
		} else if resp.StatusCode == http.StatusForbidden {
			return nil, errors.New(e.InvalidAuth)
		} else {
			err_str := "Authentication failed: Unexpected response status: " + resp.Status
			logger.Error(err_str)
			return nil, errors.New(err_str)
		}
	} else {
		return nil, err
	}
	return
}

func clientId(t string) string {
	for _, part := range strings.Split(t, "|") {
		if kv := strings.Split(part, "="); kv[0] == "client_id" {
			return kv[1]
		}
	}
	//if we get here then we have a new style token and need to make a call to look up the
	//ID instead of parsing the string
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	req, err := http.NewRequest("GET", conf.AUTH_GLOBUS_TOKEN_URL, nil)
        //logger.Error("URL: " + conf.AUTH_GLOBUS_TOKEN_URL)

	if err != nil {
		logger.Error("Failed contact with auth server")
		return ""
	}
	req.Header.Add("X-Globus-Goauthtoken", t)
	if resp, err := client.Do(req); err == nil {
		//logger.Error("resp: " + resp.Status)
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
			if body, err := ioutil.ReadAll(resp.Body); err == nil {
				var dat map[string]interface{}
				if err = json.Unmarshal(body, &dat); err != nil {
					logger.Error("Unable to parse response from auth server")
					return ""
				} else {
					return dat["client_id"].(string)
				}
			}
		} else if resp.StatusCode == http.StatusForbidden {
			logger.Error("Auth request rejected as Forbidden")
			return ""
		} else {
			err_str := "Authentication failed in clientID: Unexpected response status: " + resp.Status
			logger.Error(err_str)
			return ""
		}
	} else {
		logger.Error("Undefined auth error 1")
		return ""
	}
	logger.Error("Undefined auth error 2")
	return ""
}
