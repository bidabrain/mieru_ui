// mieru-ui: a web panel for the mieru proxy server (mita).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

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
      -password string           admin password
      -port int                  panel port
      -basepath string           panel URL path, e.g. /secret/
      -listen string             listen IP ("" = all)
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
	show := fs.Bool("show", false, "show settings")
	_ = fs.Parse(args)

	store := openStore()
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
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	if *show || (*username == "" && *password == "" && *port == 0 && *basePath == "" && *listen == "\x00") {
		s := store.Snapshot().Settings
		fmt.Printf("username: %s\nport:     %d\nbasePath: %s\nlisten:   %q\ndata:     %s\n",
			s.Username, s.Port, web.NormalizeBasePath(s.BasePath), s.ListenIP, config.DBPath())
		return
	}
	fmt.Println("settings updated; restart the panel (systemctl restart mieru-ui) to take effect")
}
