package nextcloud

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jooapa/nextdoor/internal/config"
	"github.com/studio-b12/gowebdav"
)

// NewClient initializes a new WebDAV client using the provided configuration.
func NewClient(cfg *config.Config) *gowebdav.Client {
	client := gowebdav.NewClient(cfg.Webdav.URL, cfg.Webdav.User, cfg.Webdav.Password)
	
	// Optimize HTTP transport for high concurrency and speed
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	client.SetTransport(transport)
	
	return client
}

// ListRootFiles connects to the WebDAV server and lists files in the root directory.
func ListRootFiles(client *gowebdav.Client) error {
	fmt.Println("Connecting to Nextcloud and fetching files...")
	
	files, err := client.ReadDir("/")
	if err != nil {
		return fmt.Errorf("failed to read root directory: %w", err)
	}

	fmt.Println("Files in Root Directory:")
	for _, file := range files {
		if file.IsDir() {
			fmt.Printf("[DIR]  %s\n", file.Name())
		} else {
			fmt.Printf("[FILE] %s (Size: %d bytes)\n", file.Name(), file.Size())
		}
	}

	return nil
}
