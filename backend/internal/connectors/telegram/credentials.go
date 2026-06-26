package telegram

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	EnvAppID   = "JAZ_BUNDLED_TELEGRAM_APP_ID"
	EnvAppHash = "JAZ_BUNDLED_TELEGRAM_APP_HASH"
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
	credentials, ok, err := ParseCredentials(bundledClientID, bundledClientHash)
	if ok || err != nil {
		return credentials, ok, err
	}
	return ParseCredentials(os.Getenv(EnvAppID), os.Getenv(EnvAppHash))
}

func ParseCredentials(id, hash string) (ClientCredentials, bool, error) {
	id = strings.TrimSpace(id)
	hash = strings.TrimSpace(hash)
	if id == "" && hash == "" {
		return ClientCredentials{}, false, nil
	}
	if id == "" || hash == "" {
		return ClientCredentials{}, false, errors.New("Telegram client id and hash must both be set")
	}
	parsed, err := strconv.Atoi(id)
	if err != nil || parsed <= 0 {
		return ClientCredentials{}, false, fmt.Errorf("Telegram client id is invalid")
	}
	return ClientCredentials{APIID: parsed, APIHash: hash}, true, nil
}
