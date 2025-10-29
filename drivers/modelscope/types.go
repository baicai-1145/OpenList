package modelscope

// File represents a file or directory returned by the ModelScope API.
type File struct {
	Name          string `json:"Name"`
	Path          string `json:"Path"`
	Type          string `json:"Type"`
	Size          int64  `json:"Size"`
	CommittedDate int64  `json:"CommittedDate"`
}

// FileListResponse describes the response structure for the file listing API.
type FileListResponse struct {
	Data struct {
		Files []File `json:"Files"`
	} `json:"Data"`
	Success   bool   `json:"Success"`
	Code      int    `json:"Code"`
	Message   string `json:"Message"`
	RequestID string `json:"RequestId"`
}
