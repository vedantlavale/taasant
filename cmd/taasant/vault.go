package main

import (
	"cmp"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/vedantlavale/taasant/s3store"
	"github.com/vedantlavale/taasant/tgstore"
)

// vault is every place files can go, and the key that locks them all.
type vault struct {
	key      []byte
	def      string
	backends map[string]backend
}

// open makes the backend a target describes. It touches no network.
func open(t target) (backend, error) {
	if err := t.check(); err != nil {
		return nil, err
	}
	switch t.Type {
	case "telegram":
		return tgstore.New(t.Token, t.ChatID), nil
	case "s3":
		return s3store.New(t.Endpoint, t.Region, t.Bucket, t.AccessKey, t.SecretKey), nil
	}
	return nil, fmt.Errorf("%q is not a kind of storage taasant knows", t.Type)
}

// newVault opens the backends of a checked config. One that is not complete
// is left out, and asking for it says so.
func newVault(c config) *vault {
	key, _ := hex.DecodeString(c.Key)
	v := &vault{key: key, def: c.Default, backends: map[string]backend{}}
	for name, t := range c.Backends {
		if b, err := open(t); err == nil {
			v.backends[name] = b
		}
	}
	return v
}

// get returns the backend called name, or the default one for "".
func (v *vault) get(name string) (backend, error) {
	name = cmp.Or(name, v.def)
	if b, found := v.backends[name]; found {
		return b, nil
	}
	known := slices.Sorted(maps.Keys(v.backends))
	return nil, fmt.Errorf("%q is not set up, the places you can use are: %s", name, strings.Join(known, ", "))
}

// where is the backend an entry was stored in. Entries from before there was
// more than one place have none written down, and were all stored in Telegram.
func (e entry) where() string { return cmp.Or(e.Backend, "telegram") }
