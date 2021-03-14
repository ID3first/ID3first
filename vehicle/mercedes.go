package vehicle

import (
	"errors"
	"strings"
	"time"

	"github.com/andig/evcc/api"
	"github.com/andig/evcc/util"
	"github.com/andig/evcc/vehicle/mercedes"
	"golang.org/x/oauth2"
)

// Mercedes is an api.Vehicle implementation for Mercedes cars
type Mercedes struct {
	*embed
	*mercedes.Provider
}

func init() {
	registry.Add("mercedes", NewMercedesFromConfig)
}

// NewMercedesFromConfig creates a new Mercedes vehicle
func NewMercedesFromConfig(other map[string]interface{}) (api.Vehicle, error) {
	cc := struct {
		Title                  string
		Capacity               int64
		ClientID, ClientSecret string
		Tokens                 Tokens
		VIN                    string
		Cache                  time.Duration
	}{
		Cache: interval,
	}

	if err := util.DecodeOther(other, &cc); err != nil {
		return nil, err
	}

	if cc.ClientID == "" && cc.Tokens.Access == "" {
		return nil, errors.New("missing credentials")
	}

	var options []mercedes.ClientOption
	if cc.Tokens.Access != "" {
		options = append(options, mercedes.WithToken(&oauth2.Token{
			AccessToken:  cc.Tokens.Access,
			RefreshToken: cc.Tokens.Refresh,
			Expiry:       time.Now(),
		}))
	}

	log := util.NewLogger("mercedes")

	identity, err := mercedes.NewIdentity(log, cc.ClientID, cc.ClientSecret, options...)
	if err != nil {
		return nil, err
	}

	api := mercedes.NewAPI(log, identity)

	v := &Mercedes{
		embed:    &embed{cc.Title, cc.Capacity},
		Provider: mercedes.NewProvider(api, strings.ToUpper(cc.VIN), cc.Cache),
	}

	return v, nil
}
