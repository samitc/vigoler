package vigoler

type downloader interface {
	// GetInputSize return the size of the input in KB.
	GetInputSize(url string) (*Async, error)
	GetInputSizeHeaders(url string, headers map[string]string) (*Async, error)
	Download(url, output string) (*Async, error)
	DownloadHeaders(url string, headers map[string]string, output string) (*Async, error)
}
