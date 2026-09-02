package nextcloud

import (
	"fmt"
	"io"
	"os"
	"path"
	"sync"

	"github.com/studio-b12/gowebdav"
)

// FileInfo is an interface to extract Nextcloud-specific fields like ETag
// from gowebdav's os.FileInfo implementation.
type FileInfo interface {
	os.FileInfo
	ETag() string
	Path() string
}

// RemoteFile represents the extracted metadata from Nextcloud for a single file/folder.
type RemoteFile struct {
	Path  string
	ETag  string
	Size  int64
	IsDir bool
}

// FetchDirectoryTree recursively fetches the directory tree from Nextcloud using PROPFIND
// under the hood (via ReadDir) and extracts the ETag for each file.
func FetchDirectoryTree(client *gowebdav.Client, root string) (map[string]RemoteFile, error) {
	files := make(map[string]RemoteFile)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var fetchErr error
	var errOnce sync.Once

	// Limit concurrency to 20 parallel requests to avoid overwhelming the server
	sem := make(chan struct{}, 20)

	var fetch func(string, string)
	fetch = func(currentPath, relPath string) {
		defer wg.Done()

		sem <- struct{}{}
		defer func() { <-sem }()

		infos, err := client.ReadDir(currentPath)
		if err != nil {
			errOnce.Do(func() {
				fetchErr = fmt.Errorf("failed to read remote directory %s: %w", currentPath, err)
			})
			return
		}

		for _, info := range infos {
			var etag string
			
			if wdFile, ok := info.(FileInfo); ok {
				etag = wdFile.ETag()
			}

			filePath := path.Join(currentPath, info.Name())
			relFilePath := path.Join(relPath, info.Name())
			
			if relFilePath == "" || relFilePath == "." {
				continue
			}

			if info.IsDir() {
				wg.Add(1)
				go fetch(filePath, relFilePath)
				continue
			}

			mu.Lock()
			files[relFilePath] = RemoteFile{
				Path:  filePath,
				ETag:  etag,
				Size:  info.Size(),
				IsDir: false,
			}
			mu.Unlock()
		}
	}

	wg.Add(1)
	go fetch(root, "")
	wg.Wait()

	if fetchErr != nil {
		return nil, fetchErr
	}
	return files, nil
}

// AtomicUpload safely uploads a file using a .nextdoor-tmp extension, then renames it via WebDAV MOVE.
// This prevents interrupted transfers from resulting in corrupted files and invalid ETags on the server.
func AtomicUpload(client *gowebdav.Client, remotePath string, reader io.Reader, size int64) error {
	partPath := remotePath + ".nextdoor-tmp"

	// 1. Upload to the temporary .nextdoor-tmp file stream
	if err := client.WriteStreamWithLength(partPath, reader, size, 0644); err != nil {
		// Attempt cleanup on failure, ignore errors during cleanup
		_ = client.Remove(partPath)
		return fmt.Errorf("failed to stream upload to .nextdoor-tmp file for %s: %w", remotePath, err)
	}

	// 2. Rename (MOVE) .nextdoor-tmp to final destination atomically
	// The gowebdav Rename method issues a WebDAV MOVE command with Overwrite: T
	if err := client.Rename(partPath, remotePath, true); err != nil {
		return fmt.Errorf("failed to execute atomic MOVE for %s: %w", remotePath, err)
	}

	return nil
}
