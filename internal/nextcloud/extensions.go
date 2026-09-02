package nextcloud

import (
	"fmt"
	"io"
	"os"
	"path"

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
	err := fetchTree(client, root, "", files)
	if err != nil {
		return nil, err
	}
	return files, nil
}

func fetchTree(client *gowebdav.Client, currentPath, relPath string, files map[string]RemoteFile) error {
	// ReadDir uses a PROPFIND request with Depth: 1
	infos, err := client.ReadDir(currentPath)
	if err != nil {
		return fmt.Errorf("failed to read remote directory %s: %w", currentPath, err)
	}

	for _, info := range infos {
		var etag string
		
		// Type assert to our interface to extract the ETag
		if wdFile, ok := info.(FileInfo); ok {
			etag = wdFile.ETag()
		}

		filePath := path.Join(currentPath, info.Name())
		relFilePath := path.Join(relPath, info.Name())
		
		// Clean paths to ensure consistent map keys
		if relFilePath == "" || relFilePath == "." {
			continue // Avoid mapping the root to an empty key
		}

		// Recursively fetch subdirectories and skip adding them to the files map
		if info.IsDir() {
			if err := fetchTree(client, filePath, relFilePath, files); err != nil {
				return err
			}
			continue
		}

		files[relFilePath] = RemoteFile{
			Path:  filePath,
			ETag:  etag,
			Size:  info.Size(),
			IsDir: info.IsDir(), // Now always false
		}
	}

	return nil
}

// AtomicUpload safely uploads a file using a .nextdoor-tmp extension, then renames it via WebDAV MOVE.
// This prevents interrupted transfers from resulting in corrupted files and invalid ETags on the server.
func AtomicUpload(client *gowebdav.Client, remotePath string, reader io.Reader) error {
	partPath := remotePath + ".nextdoor-tmp"

	// 1. Upload to the temporary .nextdoor-tmp file stream
	if err := client.WriteStream(partPath, reader, 0644); err != nil {
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
