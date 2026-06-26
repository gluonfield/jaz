package telegram

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	bundledClientID   = ""
	bundledClientHash = ""
)

type ClientCredentials struct {
	APIID   int
	APIHash string
}

func Credentials() (ClientCredentials, bool, error) {
	return credentialsFrom(bundledClientID, bundledClientHash)
}

func credentialsFrom(id, hash string) (ClientCredentials, bool, error) {
	id = strings.TrimSpace(id)
	hash = strings.TrimSpace(hash)
	if id == "" && hash == "" {
		return ClientCredentials{}, false, nil
	}
	if id == "" || hash == "" {
		return ClientCredentials{}, false, errors.New("bundled Telegram client id and hash must both be set")
	}
	parsed, err := strconv.Atoi(id)
	if err != nil || parsed <= 0 {
		return ClientCredentials{}, false, fmt.Errorf("bundled Telegram client id is invalid")
	}
	return ClientCredentials{APIID: parsed, APIHash: hash}, true, nil
}
