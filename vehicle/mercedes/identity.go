package mercedes

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	"github.com/andig/evcc/util"
	"github.com/andig/evcc/util/request"
	"golang.org/x/oauth2"
)

const redirectURI = "localhost:34972"

type Identity struct {
	AuthConfig *oauth2.Config
	token      *oauth2.Token
}

func NewIdentity(id, secret string) *Identity {
	return &Identity{
		AuthConfig: &oauth2.Config{
			ClientID:     id,
			ClientSecret: secret,
			Endpoint: oauth2.Endpoint{
				AuthURL:   "https://id.mercedes-benz.com/as/authorization.oauth2",
				TokenURL:  "https://id.mercedes-benz.com/as/token.oauth2",
				AuthStyle: oauth2.AuthStyleInParams,
			},
			// Scopes: []string{"scope=mb:vehicle:status:general","mb:user:pool:reader","offline_access"},
			Scopes: []string{"offline_access"},
		},
	}
}

func state() string {
	var b [9]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// urlOpen opens the specified URL in the default browser of the user.
func urlOpen(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

func (v *Identity) Token() *oauth2.Token {
	return v.token
}

func (v *Identity) Login(log *util.Logger) error {
	state := state()
	uri := v.AuthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "login consent"),
	)

	ln, err := net.Listen("tcp", redirectURI)
	if err != nil {
		return err
	}

	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(
		context.WithValue(context.Background(), oauth2.HTTPClient, request.NewHelper(log).Client),
		60*time.Second,
	)
	defer cancel()

	srv := &http.Server{Handler: v.redirectHandler(ctx, state, done)}

	defer srv.Close()
	go srv.Serve(ln)

	if err := urlOpen(uri); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return errors.New("login timeout")
	case <-done:
		if v.token == nil {
			return errors.New("login failed")
		}
	}

	return nil
}

func (v *Identity) redirectHandler(ctx context.Context, state string, done chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer close(done)

		data, err := url.ParseQuery(r.URL.RawQuery)
		if error, ok := data["error"]; ok {
			fmt.Fprintf(w, "error: %s: %s\n", error, data["error_description"])
			return
		}

		states, ok := data["state"]
		if !ok || len(states) != 1 || states[0] != state {
			fmt.Fprintln(w, "invalid response:", data)
			return
		}

		codes, ok := data["code"]
		if !ok || len(codes) != 1 {
			fmt.Fprintln(w, "invalid response:", data)
			return
		}

		token, err := v.AuthConfig.Exchange(ctx, codes[0])
		if err != nil {
			fmt.Fprintln(w, "token error:", err)
			return
		}

		v.token = token

		fmt.Fprintln(w, "Folgende Fahrzeugkonfiguration kann in die evcc.yaml Konfigurationsdatei übernommen werden")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  tokens:")
		fmt.Fprintln(w, "    access:", token.AccessToken)
		fmt.Fprintln(w, "    refresh:", token.RefreshToken)
	}
}
