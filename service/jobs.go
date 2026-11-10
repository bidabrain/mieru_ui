package service

import (
	"log"
	"time"

	"mieru-ui/database"
)

// StartJobs runs background maintenance: disables expired users and re-applies the config.
func StartJobs(store *database.Store) {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for range t.C {
			expireUsers(store)
		}
	}()
}

func expireUsers(store *database.Store) {
	now := time.Now().UnixMilli()
	changed := 0
	_ = store.Update(func(d *database.Data) error {
		for i := range d.Users {
			u := &d.Users[i]
			if u.Enabled && u.ExpireAt > 0 && u.ExpireAt <= now {
				u.Enabled = false
				changed++
			}
		}
		return nil
	})
	if changed == 0 {
		return
	}
	log.Printf("disabled %d expired user(s), applying config", changed)
	if _, err := Apply(store); err != nil {
		log.Printf("apply after expiry failed: %v", err)
	}
}
