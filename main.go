// mieru-ui: a web panel for the mieru proxy server (mita).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"

	"mieru-ui/config"
	"mieru-ui/database"
	"mieru-ui/service"
	"mieru-ui/web"
)

func usage() {
	fmt.Fprintf(os.Stderr, `mieru-ui %s - web panel for mieru (mita)

Usage:
  mieru-ui [run]                 start the panel
  mieru-ui setting [flags]       change panel settings
      -username string           admin username
      -password string           admin password (at least 8 characters)
      -port int                  panel port
      -basepath string           panel URL path, e.g. /secret/
      -listen string             listen IP ("" = all)
      -mita string               path or name of the mita binary
      -logout-all                invalidate every login session
      -show                      print current settings
  mieru-ui version               print version

Environment:
  MIERU_UI_DATA                  data directory (default %s)
`, config.Version, config.DataDir())
}

func main() {
	log.SetFlags(log.LstdFlags)
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "run":
		run()
	case "setting":
		setting(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println(config.Version)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
}

func openStore() *database.Store {
	store, err := database.Open(config.DBPath())
	if err != nil {
		log.Fatalf("open database %s: %v", config.DBPath(), err)
	}
	return store
}

func run() {
	store := openStore()
	d := store.Snapshot()
	if store.InitialPassword != "" {
		log.Printf("created new panel database; random admin credentials were written to %s", database.InitialCredentialsPath())
	}
	if database.CheckLogin(d.Settings, "admin", "admin") {
		log.Println("WARNING: panel is using the default credentials admin/admin, change them immediately")
	}
	// Re-apply on boot so mita matches the panel (e.g. users that expired while we were down).
	go func() {
		if (service.Mita{Bin: d.Settings.MitaBin}).Installed() {
			if _, err := service.Apply(store); err != nil {
				log.Printf("initial apply failed: %v", err)
			}
		} else {
			log.Printf("mita binary %q not found; install mieru server first", d.Settings.MitaBin)
		}
	}()
	service.StartJobs(store)
	if err := web.New(store).Run(); err != nil {
		log.Fatal(err)
	}
}

func setting(args []string) {
	fs := flag.NewFlagSet("setting", flag.ExitOnError)
	username := fs.String("username", "", "admin username")
	password := fs.String("password", "", "admin password")
	port := fs.Int("port", 0, "panel port")
	basePath := fs.String("basepath", "", "panel base path")
	listen := fs.String("listen", "\x00", "listen IP")
	mita := fs.String("mita", "", "mita binary")
	logoutAll := fs.Bool("logout-all", false, "invalidate all sessions")
	show := fs.Bool("show", false, "show settings")
	_ = fs.Parse(args)

	if *password != "" && len(*password) < 8 {
		log.Fatal("password must be at least 8 characters")
	}
	if *mita != "" {
		if _, err := exec.LookPath(*mita); err != nil {
			log.Fatalf("mita binary %q not found or not executable: %v", *mita, err)
		}
	}

	store := openStore()
	changed := *username != "" || *password != "" || *port != 0 || *basePath != "" ||
		*listen != "\x00" || *mita != "" || *logoutAll
	err := store.Update(func(d *database.Data) error {
		s := &d.Settings
		if *username != "" {
			s.Username = *username
		}
		if *password != "" {
			if err := database.SetPassword(s, *password); err != nil {
				return err
			}
		}
		if *port > 0 && *port <= 65535 {
			s.Port = *port
		}
		if *basePath != "" {
			s.BasePath = web.NormalizeBasePath(*basePath)
		}
		if *listen != "\x00" {
			s.ListenIP = *listen
		}
		if *mita != "" {
			s.MitaBin = *mita
		}
		if *logoutAll {
			s.SessionSecret = database.RandomHex(32)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	if *password != "" {
		database.RemoveInitialCredentials()
	} else if store.InitialPassword != "" {
		fmt.Printf("created new database; initial credentials: %s\n", database.InitialCredentialsPath())
	}
	if *show || !changed {
		s := store.Snapshot().Settings
		fmt.Printf("username: %s\nport:     %d\nbasePath: %s\nlisten:   %q\nmita:     %s\ndata:     %s\n",
			s.Username, s.Port, web.NormalizeBasePath(s.BasePath), s.ListenIP, s.MitaBin, config.DBPath())
		return
	}
	fmt.Println("settings updated; restart the panel (systemctl restart mieru-ui) to take effect")
}
