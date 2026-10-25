package release

import "strings"

const (
	// ChecksumsName is the release asset that lists the SHA-256 of every archive.
	ChecksumsName = "checksums.txt"

	archivePrefix = "canary_"
	downloadBase  = "https://github.com/m10rten/go-canary-cli/releases/download"
)

// ArchiveName returns the release archive name for a tag, using Go's OS and
// architecture names. Windows archives are zip files; all others are tar.gz.
func ArchiveName(tag, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return archivePrefix + strings.TrimPrefix(tag, "v") + "_" + goos + "_" + goarch + ext
}

// DownloadURL returns the download URL of a release asset for a v-prefixed tag.
func DownloadURL(tag, asset string) string {
	return downloadBase + "/" + tag + "/" + asset
}
