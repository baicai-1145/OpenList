package modelscope

import (
	stdpath "path"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

// Helper for transforming ModelScope API file objects into OpenList model objects.
func fileToObj(file File) model.Obj {
	isDir := file.Type == "tree"
	return &model.Object{
		ID:       file.Path,
		Name:     file.Name,
		Size:     file.Size,
		Modified: time.Time{},
		IsFolder: isDir,
		Path:     file.Path,
	}
}

func filesToObjs(files []File) []model.Obj {
	objects := make([]model.Obj, 0, len(files))
	for _, file := range files {
		object := &model.Object{
			Name:     file.Name,
			Path:     file.Path,
			Size:     file.Size,
			Modified: time.Unix(file.CommittedDate, 0),
			IsFolder: file.Type == "tree",
		}
		objects = append(objects, object)
	}
	return objects
}

func getRelativePath(obj model.Obj) string {
	if obj == nil || obj.GetPath() == "/" {
		return ""
	}
	return stdpath.Clean(obj.GetPath())
}
