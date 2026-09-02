package main

import (
	"fmt"
	"log"
	"os"

	"github.com/pelletier/go-toml/v2"
	"github.com/studio-b12/gowebdav"
)

type Config struct {
	Webdav struct {
		URL      string `toml:"url"`
		User     string `toml:"user"`
		Password string `toml:"password"`
	} `toml:"webdav"`
}

func main() {
	// 1. Read config.toml
	configFile, err := os.ReadFile("config.toml")
	if err != nil {
		log.Fatalf("Error reading config.toml: %v\nMake sure to create it or run from the correct directory.", err)
	}

	var cfg Config
	err = toml.Unmarshal(configFile, &cfg)
	if err != nil {
		log.Fatalf("Error parsing config.toml: %v", err)
	}

	// 2. Initialize the WebDAV client using config
	client := gowebdav.NewClient(cfg.Webdav.URL, cfg.Webdav.User, cfg.Webdav.Password)

	// Optional: If you are testing on a local server without a valid SSL certificate
	// client.SetTransport(gowebdav.InsecureTransport())

	// 3. Connect and List Files in the root directory
	fmt.Println("Connecting to Nextcloud and fetching files...")
	files, err := client.ReadDir("/")
	if err != nil {
		log.Fatalf("Failed to read directory: %v", err)
	}

	fmt.Println("Files in Root Directory:")
	for _, file := range files {
		if file.IsDir() {
			fmt.Printf("[DIR]  %s\n", file.Name())
		} else {
			fmt.Printf("[FILE] %s (Size: %d bytes)\n", file.Name(), file.Size())
		}
	}
}
