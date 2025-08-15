package modelscope

// File represents a file or directory from the ModelScope API.
type File struct {
	Name          string `json:"Name"`
	Path          string `json:"Path"`
	Type          string `json:"Type"` // "file" or "tree"
	Size          int64  `json:"Size"`
	CommittedDate int64  `json:"CommittedDate"`
}

// FileListResponse is the top-level structure for the file list API response.
type FileListResponse struct {
	Data struct {
		Files []File `json:"Files"`
	} `json:"Data"`
	Success   bool   `json:"Success"`
	Message   string `json:"Message"`
	RequestId string `json:"RequestId"`
}
